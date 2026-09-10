# 17. The iteration contracts

- **Status:** Proposed — and deliberately **not a decision**. This ADR states
  five problems precisely, records the evidence, sketches directions, and
  develops three rival proposals. It decides nothing, except problem 4, which is
  settled in place. A second ADR, or a revision of this one, should choose.
- **Date:** 2026-09-08
- **Evidence:** `experiments/iteration/` (`RESULTS.md`), with context from
  `experiments/vectorcost/`, `sliceadapter/`, `linkedlist/` and `sizedcollect/`.
- **Relates to:** ADR `0006` (`Elems`/`Elems2`, and the reason they are two
  interfaces), `0008` (the contract layers, and the deferred ordered tier),
  `0004` (bulk insert), `0010` (`LinkedList`, whose `All` yields pairs),
  `0013` (views, and the deferred "should `Range` return a view"), `0015` and
  `0016` (`Vector` and slice views, where problem 1 keeps surfacing).

## Context

Iteration is the one thing every container here does, and five problems have
accumulated around it. Each was found while doing something else, each was
deferred, and they are related closely enough that fixing one in isolation is
likely to make another worse.

## Problem 1: a type has one `All`, so `Elems` and `Elems2` are exclusive

```go
type Elems[T any] interface  { Len() int; All() iter.Seq[T] }
type Elems2[K, V any] interface { Len() int; All() iter.Seq2[K, V] }
```

A type cannot satisfy both. Verified as a compile error:

```
Slice[string] does not implement Elems2[int, string] (wrong type for method All)
        have All() iter.Seq[string]
        want All() iter.Seq2[int, string]
```

ADR `0006` records the cause — "Go has no higher-kinded types: `iter.Seq` and
`iter.Seq2` cannot be unified" — and treats it as a fact to live with. It has
since cost four separate things:

- **`Vector` must pick one.** It can be seen as values, as index/value pairs, or
  as bare indexes. It gets one, and the others need different names or nothing.
  `AllIndexed` exists **solely because `All` was taken**.
- **A slice adapter could not serve index/value pairs** (ADR `0016`), which
  removed one of the last reasons to have one.
- **`LinkedList` (ADR `0010`) spends its `All` on cursor/value pairs**, so it
  satisfies `Elems2` and not `Elems`. The two sequence containers therefore
  **disagree about what `All` means**.
- **`ListView[P, NT]` cannot be built** (ADR `0016`) because of that
  disagreement: whichever shape it picks, one container's view stops mirroring
  its container.

### And the library contradicts the standard library

This is the part that was not noticed before. In the stdlib, **`All` always means
`iter.Seq2`**, without exception:

| stdlib | yields |
|---|---|
| `slices.All(s)` | `iter.Seq2[int, E]` |
| `slices.Backward(s)` | `iter.Seq2[int, E]` |
| `maps.All(m)` | `iter.Seq2[K, V]` |
| `slices.Values(s)` | `iter.Seq[E]` |
| `maps.Keys(m)` | `iter.Seq[K]` |
| `maps.Values(m)` | `iter.Seq[V]` |

Against this library:

| here | yields | matches the stdlib? |
|---|---|---|
| `HashDict.All`, `SortedDict.All`, `Map.All` | `iter.Seq2[K, V]` | yes |
| `HashSet.All`, `SortedSet.All` | `iter.Seq[T]` | **no** |
| `Vector.All` | `iter.Seq[T]` | **no** |
| `Vector.AllIndexed` | `iter.Seq2[int, T]` | this is what `All` means elsewhere |

So `All` is overloaded here in a way it is not overloaded in Go, and the
half that collides is the half that disagrees with the language's own vocabulary.
**Problem 1 may be a naming problem wearing a type-system problem's clothes.**

## Problem 2: bulk construction and insertion is ragged

| container | variadic constructor | `Collect` | `CollectSeq` | variadic add | `*All` | `*AllSeq` |
|---|---|---|---|---|---|---|
| `HashSet` | ✓ | **✗** | **✗** | ✓ | **✗** | **✗** |
| `SortedSet` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `HashDict` | n/a | ✓ | ✓ | n/a | ✓ | ✓ |
| `SortedDict` | n/a | ✓ | ✓ | n/a | ✓ | ✓ |
| `Map` | n/a | **✗** | **✗** | n/a | **✗** | **✗** |
| `Vector` | ✓ | ✓ | ✓ | **✗** | ✓ | ✓ |

Some of the gaps are principled and some are not:

- **`n/a` is genuine.** A dict cannot take a variadic of key/value pairs, so
  `NewHashDict()` taking nothing is not an inconsistency with
  `NewHashSet(vs ...T)`.
- **`Map`'s row is defensible.** ADR `0009` makes it a deliberately thin adapter.
- **`HashSet`'s row is not.** It has no `CollectHashSet`, no `AddAll` and no
  `AddAllSeq`, while `SortedSet` has all three. Nothing decided that; it is the
  oldest container and the conventions arrived later.
- **`Vector`'s missing variadic add is deliberate** (ADR `0015`, measured), but
  ADR `0015`'s own follow-up then found that a variadic *bulk* form is 2.1x
  faster than any alternative for appending a slice, and `AppendMany` was never
  added.

There is also a shape question hiding under the ✓s: **every operation exists
twice**, once taking `Elems` and once taking `iter.Seq`, because the first
carries a length and the second does not. That is eight functions and eight
methods to say four things.

## Problem 3: ordered containers iterate forwards only

`SortedSet`, `SortedDict` and `Vector` are ordered, and none of them can be read
in reverse. Neither can any view. There is no `Backward` anywhere in the package,
and `Range(lo, hi)` runs forwards only.

The stdlib has `slices.Backward`. A caller here has no equivalent, and — unlike
the shape problem — **cannot build one**.

## Problem 4: "key" and "value" are not defined, and the viewers disagree

This one is arguably **prior to problem 1** — it is numbered last because it was
found last.

There is a rule operating in the package, and it is nowhere written down:

> A **key** is the handle a container's lookup methods take. A **value** is what
> they give back.

Applied to what exists:

| container | lookup takes | yields | key | value |
|---|---|---|---|---|
| `HashDict[K, V]`, `Map[K, V]` | `K` | `V` | `K` | `V` |
| `SortedDict[K, V]` | `K` | `V` | `K` | `V` |
| `HashSet[T]` | `T` | `bool` | `T` | — |
| `SortedSet[T]` | `T` | `bool` | `T` | — |
| `Vector[T]`, a slice | `int` | `T` | `int` | `T` |
| `LinkedList[V]` (ADR `0010`) | a cursor | `V` | cursor | `V` |

That rule is coherent and answers the questions directly. **A `Vector`'s index is
its key.** **A set's element is its key, and a set has no value.**

### But the viewers implement four different answers

ADR `0012` gives keys a round trip — `ToKeyView` out, `FromKeyView(NK) (K, bool)`
back in — and values only an outbound conversion, because only a key travels
inward, for lookup. That is right. What is not right is which containers get
which:

| container | viewer requires | key converted? | round-trips? |
|---|---|---|---|
| `HashDict`, `Map` | `KeyViewer` + `ValueViewer` | yes | yes |
| `HashSet` | `KeyViewer` only | yes | yes |
| `SortedDict` | `ValueViewer` only | **no** | — |
| `Vector`, slice | `ValueViewer` only | n/a (`int`) | — |
| `SortedSet` | *no viewer at all* | **no** | — |

Four treatments for what the rule above says are three kinds of thing.

**`HashDict.K` and `SortedDict.K` are the same kind of thing** — caller-supplied
lookup keys — and one is convertible while the other is not. ADR `0012`
decision 3 justified that on **safety**: `cmp.Ordered` admits only immutable
value types, so there is nothing for a conversion to protect.

That argument is sound and incomplete. Conversion does two jobs:

- **Protection** — hide a mutable key behind a read-only type. Only mutable keys
  need it, and `cmp.Ordered` keys are not mutable. `0012` is right about this.
- **Abstraction** — stop a consumer naming the container's own types. ADR `0013`
  leaned on this heavily: "unordered consumers stop naming the implementation."

Ordered views get the first for free and the second not at all. A consumer of
`SortedDictView[K, NV]` names the container's raw `K`, and a consumer of
`SortedSetView[T]` names its raw `T` with no way to convert anything. **Nobody
decided that was acceptable; it fell out of a decision made about safety.**

### A third category the rule does not name

`HashDict`'s `K` is caller data. `Vector`'s `int` is not — it is a coordinate the
container assigned, meaningful only relative to that container. A
`LinkedListCursor` is the same kind of thing and more obviously so, being opaque
by construction (ADR `0010` decision 1).

So there are three categories, not two:

| category | example | converted? | round-trips? |
|---|---|---|---|
| **caller key** | `HashDict.K`, `HashSet.T` | yes — protection and abstraction | **yes**, it travels inward |
| **position** | `Vector`'s index, a cursor | no — already opaque or trivial | n/a |
| **value** | `HashDict.V`, `Vector`'s element | yes — outbound only | no |

Under this taxonomy every current treatment is explained except one: **ordered
containers put caller keys in the position column**, which is where the anomaly
lives.

### It decides problem 1

If a container is a bag of *(handle, value)* pairs, then the general contract is
`Elems2` and `Elems` is the degenerate case:

- a **dict** is key/value → `HoldsAll[K, V]`
- a **sequence** is position/value → `HoldsAll[int, T]`
- a **set** has keys and no values → `HoldsKeys[T]` is the honest shape

(Written with the `Holds*` names, which replace `Elems` and `Elems2` outright
under proposal A. ADR `0006` gave those interfaces one job — carrying a length —
and `SizeHint` takes it over.)

Which makes `Vector` an `Elems2[int, T]` whose values can also be walked alone —
exactly direction 1A below, arrived at from semantics rather than from matching
the stdlib. That two independent routes reach the same shape is the strongest
argument either of them has.

## Problem 5: there is no way to build one container from another's keys

Orthogonal to problem 2, which is about the bulk API being *ragged*. This one is
about it being *combinatorial*.

Putting a `HashDict`'s keys into a `Vector` has no spelling today. The
constructors take either an `Elems[T]` or an `iter.Seq[T]`, and a `HashDict` is
an `Elems2[K, V]`, so the caller writes the adapter by hand:

```go
// sketch -- today
v := containers.CollectVectorSeq(func(yield func(K) bool) {
	for k := range d.All() {
		if !yield(k) {
			return
		}
	}
})
```

That works and loses two things: the **length**, which `CollectVectorSeq` cannot
take, and the **value**, which is copied into every yield and immediately thrown
away.

### What it costs, measured

Building a `Vector` of a dict's 256 string keys:

| | | memory | allocs |
|---|---|---|---|
| **narrow (`int`) values** | | | |
| today, by hand | 2.002 µs | 9.30 KiB | 15 |
| keeping the length | 1.304 µs (**1.54x**) | 4.98 KiB | 9 |
| length **and** a native key walk | **950.0 ns (2.1x)** | 4.89 KiB | 6 |
| **wide (1 KiB) values** | | | |
| today, by hand | 6.080 µs | 9.30 KiB | 15 |
| keeping the length | 5.830 µs (1.04x) | 4.98 KiB | 9 |
| length **and** a native key walk | **970.9 ns (6.3x)** | 4.89 KiB | 6 |

The fully-adapted form is **flat in value width** — 950 ns against 971 ns —
because it never touches a value. Both partial forms scale with it.

### The naive fix, sized

Add constructors that read the other shapes: `CollectVectorFromKeys`,
`CollectVectorFromValues`, and the `Seq` twin of each. Counting it honestly:
three single-element targets (`HashSet`, `SortedSet`, `Vector`), three source
shapes (an `Elems`, an `Elems2`'s keys, an `Elems2`'s values), and a sized and an
unsized form of each is **eighteen functions**, before `LinkedList` or any dict
target. It grows as containers × shapes × 2.

## Findings

From `experiments/iteration/`.

### Converting between shapes is free only when the discarded half is small

With `int` elements, deriving one shape from another looks free — `Values` from
an `All` costs nothing measurable, `Keys` likewise, inventing an index costs
6.5%, and none of them allocate.

**That does not generalise.** An `iter.Seq2` passes its value *by value* into
`yield`, so a consumer that drops it has already paid for the copy. At 256
elements, per element:

| element width | `Keys` native | `Keys` derived, value dropped | value actually consumed |
|---|---|---|---|
| 64 B | 1.31 ns | 3.31 ns (**2.5x**) | 3.30 ns |
| 1 KiB | 1.20 ns | 17.9 ns (**14.9x**) | 23.7 ns |
| 8 KiB | 1.19 ns | 72.8 ns (**61x**) | 102.4 ns |

A native key walk is **flat** at ~1.2 ns per element, because it never touches
the value. A derived one scales at memory bandwidth. And **dropping a value costs
70–80% of what using it would have** — a caller who wants only keys pays almost
the full price of the data they discard.

The `int` figures said otherwise because they were built from concrete,
inlinable functions: the compiler saw the value was unused and elided the copy.
Routing the same measurement through a **generic** interface — which is what this
library does, `Elems2[K, V]` being generic — stops the elision. **The elision is
real but fragile, and an API cannot rely on it.**

This is also why `maps.Keys` exists in the stdlib as its own function rather than
something derived from `maps.All`: `for k := range m` never materialises the
value, and no wrapper around `maps.All` can avoid doing so.

### Reversing is free from inside, and expensive from outside

| | | allocs |
|---|---|---|
| forward, native | 1.199 µs | 0 |
| **backward, native** | **1.202 µs** | **0** |
| **backward, derived from a forward iterator** | **4.055 µs** | **12, 24.6 KiB** |

An `iter.Seq` is a *push* iterator: it yields forwards and the caller cannot ask
for the previous element. Reversing one from outside means buffering all of it —
3.4x, and allocation linear in the sequence.

### What can be done from outside a container, and at what price

| | free function? | cost |
|---|---|---|
| drop a small key or value | yes | nothing |
| drop a **wide** value | yes | ~the cost of using it |
| invent an index | yes | ~6.5% |
| **reverse** | **no** | buffers the whole sequence |

A container knows its backing, so it can walk keys without materialising values
and walk backwards without buffering. Nothing outside it can do either.

**Both problems 1 and 3 therefore want methods on containers, for different
reasons.** Problem 3 because a free function cannot do the job at all; problem 1
because a free function can, but pays for data it throws away as soon as values
are wide. The earlier reading of this experiment — that shape was purely an
ergonomics question — was wrong, and the direction sketches below are graded
against the corrected finding.

## A note on the examples below

Each direction shows what the **common case** looks like: a caller who just wants
to walk keys, values, or pairs. The running example is a dict whose values are
wide, a set, and a vector:

```go
d := containers.NewHashDict[string, User]()  // User is large
s := containers.NewHashSet[string]()
v := containers.NewVector[Item]()
```

One thing to get straight first, because it changes how problem 1 reads:
**iterating keys is already expressible today.** Ranging an `iter.Seq2` with one
variable is legal Go:

```go
for k := range d.All() { }   // compiles today, and copies every User
```

So problem 1 is not that a caller cannot walk keys. It is that doing so costs
14.9x a native walk at 1 KiB values, and that `Vector` cannot satisfy
`Elems2[int, T]` for construction. The directions below should be read against
that: the iteration examples are mostly about *cost* and *churn*, not about
expressiveness.

## Directions for problem 1

### 1A. Follow the stdlib: `All` means `Seq2`, `Values`/`Keys` mean `Seq`

```go
// sketch
type Elems[T any] interface     { Len() int; Values() iter.Seq[T] }
type Elems2[K, V any] interface { Len() int; All() iter.Seq2[K, V] }
```

`Vector` then has both, and satisfies both contracts. `AllIndexed` becomes
`All` and stops being a workaround. `LinkedList` keeps `All` for cursor pairs and
gains `Values`. The two sequence containers stop disagreeing, which unblocks
`ListView[P, NT]`.

Against: it renames `All` on every set-side container, which is the most
disruptive option here, and it means `HashDict` would satisfy `Elems[V]` via
`Values` — collecting a dict into a set would silently take the values.

**Strengthened by the corrected finding.** A native `Keys` is flat at ~1.2 ns per
element regardless of value width, where a derived one is 14.9x that at 1 KiB.
Native per-shape methods are a performance feature, not only an ergonomic one —
which is exactly the reason the stdlib has `maps.Keys` rather than leaving
callers to wrap `maps.All`.

Iterating:

```go
for k, v := range d.All() { }     // pairs
for k := range d.Keys() { }       // keys -- never materialises a User
for val := range d.Values() { }   // values

for e := range s.Keys() { }       // a set's elements -- WAS s.All(); a set's
                                  // element IS its key, per problem 4
for i, e := range v.All() { }     // WAS v.AllIndexed()
for e := range v.Values() { }     // WAS v.All()
```

The first three read well and the last three are the cost: **every existing set
and vector iteration in every caller changes**, and the two that change are the
ones that already looked right. `s.All()` becoming `s.Keys()` is the single most
disruptive line in this ADR, and the least comfortable — "the keys of a set"
is correct under the taxonomy and still reads oddly. `s.Values()` would read
better and would contradict problem 4.

### 1B. Keep one `All` per container; add free functions for other shapes

```go
// sketch
func Values[K, V any](e Elems2[K, V]) iter.Seq[V]
func Keys[K, V any](e Elems2[K, V]) iter.Seq[K]
```

Cheapest change. **Weakened badly by the corrected finding**: free functions are
free only for narrow values, and a `Keys` derived from an `All` over 1 KiB values
costs 14.9x a native one. It also does not solve the problem — `Vector`'s `All`
is still `Seq[T]`, so `Vector` still cannot be an `Elems2`, and
`CollectHashDict(vector)` still cannot work.

Free functions remain useful as a *convenience* for the narrow cases. They are
not a substitute for a container offering the shape natively.

Iterating:

```go
for k, v := range d.All() { }                  // pairs, unchanged
for k := range containers.Keys(d) { }          // keys -- still copies every User
for val := range containers.Values(d) { }      // values

for e := range s.All() { }                     // unchanged
for e := range v.All() { }                     // unchanged
for i, e := range v.AllIndexed() { }           // unchanged
```

**No churn at all**, which is its real appeal. But note what the second line
costs, and note a wrinkle: if `Keys` returns an `Elems[K]` so it can also feed a
constructor (problem 5), it is an *interface* and no longer directly rangeable —
the caller writes `range containers.Keys(d).All()`. Making it rangeable means
returning a bare `iter.Seq[K]`, which drops the length, which is problem 5 again.
**One helper cannot be both** while `Elems` is an interface.

### 1C. Make `Elems` a value, not a contract

```go
// sketch
type Elems[T any] struct {
	Len int
	Seq iter.Seq[T]
}

func (v *Vector[T]) Values() Elems[T]
func (v *Vector[T]) Indexed() Elems2[int, T]
```

The shape stops being a property of the *type* and becomes a property of the
*call*, so the collision disappears entirely — a container can offer as many
shapes as it likes. It also folds problem 2's duplication away: an unknown length
is `Len: 0`, so `CollectX(Elems[T])` covers what `CollectX` and `CollectXSeq`
cover today, halving that surface.

Iterating:

```go
for k, v := range d.All().Seq { }              // pairs -- note the .Seq
for k := range d.Keys().Seq { }                // keys
for val := range d.Values().Seq { }            // values

for e := range s.Keys().Seq { }   // a set's element is its key
for i, e := range v.All().Seq { }
```

**The `.Seq` is a tax on the most common operation in the package**, and it is
paid on every iteration whether or not the length is wanted. A struct is not
rangeable; only its func-typed field is. (`for v := range e.Seq` does compile —
verified — so this works, it just reads worse than everything else here.)

The compensation is that the wrinkle 1B hits disappears: one helper can be both
rangeable and length-carrying, because the length rides alongside rather than
inside an interface. `containers.Keys(d)` feeds a constructor *and*
`range containers.Keys(d).Seq` walks it.

Against: it discards `Elems` as an interface, which ADRs `0006` and `0008` are
built on — containers would no longer *satisfy* anything, they would *produce*
something. That is a large conceptual change, and it removes the ability to write
a function generic over "any container" without naming a producing method.

### 1D. Keep both `All`s and accept the collision

The status quo, made explicit: `Elems` and `Elems2` stay exclusive, `AllIndexed`
stays, `ListView` stays unbuildable, and the disagreement with the stdlib stays.

Iterating, today:

```go
for k, v := range d.All() { }        // pairs
for k := range d.All() { }           // keys -- legal, and copies every User
for _, val := range d.All() { }      // values
for e := range s.All() { }
for e := range v.All() { }
for i, e := range v.AllIndexed() { } // and `for i := range v.AllIndexed()` for indexes
```

Every shape a caller might want is reachable. Two of the six lines silently copy
a value they discard, at 14.9x a native walk when the value is 1 KiB, and nothing
at the call site says so. **That is the honest statement of the status quo**, and
it is recorded so that doing nothing is a choice rather than a default.

## Directions for problem 2

### 2A. Fill the matrix

Give `HashSet` its `CollectHashSet`, `AddAll` and `AddAllSeq`; add
`Vector.AppendMany(es ...T)`; decide `Map` deliberately rather than by omission.
Smallest change, leaves the shape duplication in place.

**Iteration is untouched by all three of problem 2's directions** — they change
how a container is filled, not how it is read. What changes is the filling:

```go
containers.CollectHashSet(other)          // does not exist today
hs.AddAll(other)                          // does not exist today
v.AppendMany(names...)                    // does not exist today; 2.1x the alternatives
```

### 2B. Collapse `X` and `XSeq` into one

Follows from 1C: if a length is carried by the argument rather than by the
argument's type, one function covers both. Eight functions and eight methods
become four and four.

```go
containers.CollectVector(src)                     // src carries its own length
containers.CollectVector(containers.Seq(anyIter)) // or does not, at Len 0
```

Iteration is untouched, except that anything *returning* an `Elems` acquires
1C's `.Seq` tax.

### 2C. Free functions instead of methods

```go
// sketch
func InsertAll[T any](dst MutableSet[T], src Elems[T])
```

One implementation instead of one per container, and it cannot be forgotten on a
new container. Against: it reads worse than a method, and ADR `0002` records that
anything generic over containers has to be a free function anyway — so this may
be where it ends up regardless.

```go
containers.InsertAll(hs, other)   // instead of hs.AddAll(other)
```

Iteration untouched.

## Directions for problem 3

### 3A. `Backward` methods on the ordered containers

```go
// sketch
func (s *SortedSet[T]) Backward() iter.Seq[T]
func (d *SortedDict[K, V]) Backward() iter.Seq2[K, V]
func (v *Vector[T]) Backward() iter.Seq2[int, V]
```

Mirrors `slices.Backward`, free at every backing this library has, and it is the
only place the capability can come from.

Iterating:

```go
for e := range ss.All() { }              // forward, as today
for e := range ss.Backward() { }         // reverse
for k, val := range sd.Backward() { }
for i, e := range vec.Backward() { }

for k, val := range sd.Range(lo, hi) { }         // forward slice of the order
for k, val := range sd.RangeBackward(lo, hi) { } // ...and its reverse?
```

The first four read exactly like their forward twins, which is the appeal. The
last line is the open part: a reversed range needs a second method, and **adding
one per ordered operation does not scale** — `Range`, and then `Floor`, `Ceil`,
`Min` and `Max` already come in pairs by another name.

### 3B. Reversal as a view operation

```go
// sketch
func Reverse[NT any](v IndexedView[NT]) IndexedView[NT]
```

Free for an `IndexedView`, whose `At(i)` makes reversal a subtraction. Does not
work for `SetView` or `DictView`, which have no positional access — so it solves
reversal for sequences only, which may be enough.

Iterating:

```go
view := containers.ViewVectorIdentity(vec)
for e := range view.All() { }                          // forward
for e := range containers.Reverse(view).All() { }      // reverse

// A sorted set or dict cannot be reversed this way at all.
```

Note what that costs a caller who is not otherwise using views: reversing a
`Vector` now means constructing a view first, which is an allocation and a
concept they did not ask for.

### 3C. `Range` returns a view, and views reverse

ADR `0013` already carries "should `Range` return a view rather than an
iterator?" as a follow-up, and ADR `0008` carries a deferred ordered contract
tier. Combined with 3B, one answer covers all three: `Range` yields a view,
views can be reversed, and the ordered tier declares both.

Iterating:

```go
for k, val := range sd.All() { }                              // whole container
for k, val := range sd.Range(lo, hi).All() { }                // a slice of it
for k, val := range containers.Reverse(sd.Range(lo, hi)).All() { }  // reversed
```

Every ordered read composes from two pieces instead of needing its own method,
which is what stops the multiplication. The cost is visible in that third line:
it is the most to read of any option here, and `Range` returning a view is a
behaviour change for existing callers, who currently range it directly.

This is the most speculative direction and the only one that does not multiply
methods.

## Directions for problem 4

Problem 4 carries its proposal inline — the three-category taxonomy of caller
key, position and value — because it is a definition rather than a menu. The
choice it presents is narrower: **adopt that taxonomy and fix the one thing that
contradicts it**, which is ordered containers treating caller keys as positions,
or **keep the current four treatments** and record that "key" means different
things in different corners of the package.

The second is a real option. Converting ordered keys costs the `SortedDictView`
embedding that ADR `0016` depends on, and buys abstraction for key types that are
already immutable and mostly already named.

What a caller sees, adopting the taxonomy:

```go
// A sorted dict view could then hide its key type, as a hash dict view already can.
sv := containers.ViewSortedDict(sd, viewer)          // viewer converts K -> NK too
for nk, nval := range sv.All() { }                   // neither raw type named
```

and keeping the four treatments:

```go
sv := containers.ViewSortedDict(sd, viewer)          // viewer converts values only
for k, nval := range sv.All() { }                    // k is the container's raw K
```

The difference is one identifier in one line, which is a fair measure of how much
is at stake: this is a coherence question, not an ergonomics one.

### What problem 4 settles

All of it. The taxonomy is adopted — **caller key, position, value** — and the
three questions it left are answered.

**1. Ordered containers keep converting values only.** ADR `0012` decision 3
stands unchanged. Its safety argument is sound: `cmp.Ordered` admits only
immutable value types, so there is nothing for a key conversion to protect. The
abstraction it forgoes — a consumer of `SortedDictView[K, NV]` naming the raw
`K` — is not worth reopening ADR `0016`'s `SortedDictView` embedding `DictView`,
which only type-checks *because* ordered views convert values only.

So the one thing that contradicts the taxonomy stays, deliberately, and is
recorded as an exception rather than an oversight: **ordered containers treat
caller keys as positions.**

**2. Positions are never converted.** An index or a cursor is assigned by the
container, so a conversion would have nothing to protect and nothing to
abstract. This is what makes `CanViewVector` and `CanViewSlice` `ValueViewer`s
with no key half, and it is now a stated rule rather than an accident of how they
were written.

**3. "Key" keeps two scopes, documented rather than renamed.**

`HoldsKeys[int]` covers a `Vector`'s index; `KeyViewer`'s round trip does not.
The word spans caller keys and positions in one family and only caller keys in
the other. Three reasons not to fix that by renaming:

- **It cannot be misused.** Every view constructor names its own
  `CanView<Container>` interface, so the pairing is fixed at the signature:
  `ViewVector` asks for a `ValueViewer` and no caller can cause a key conversion
  on a `Vector`. This is a homonym, not a collision — unlike `All`, which was a
  collision that *blocked* designs. Nothing here is blocked.
- **Go's own vocabulary agrees.** `slices.All` returns `iter.Seq2[int, E]`,
  putting a slice index in the key position of a pair. Calling a `Vector`'s index
  a key for iteration follows the language rather than inventing anything.
- **Renaming costs more than it buys.** `Keys()` matches `maps.Keys`, which is
  half the point of problem 1. And splitting the source family into `HoldsKeys`
  and a `HoldsPositions` would mean two `HoldsAll` variants, which breaks the
  uniformity that lets `Backward()` return a single interface.

The scope is therefore stated, not renamed: **`HoldsKeys` means the handle a
container addresses elements by. `KeyViewer` means a key the caller supplies and
can hand back.** Positions are the first and not the second.

## Directions for problem 5

### 5A. Add the constructors

Eighteen or more functions, each trivial, each needing a doc comment and a test,
and each a thing to forget when a container is added. Recorded so that "just add
them" is a weighed option rather than the default — it is not obviously wrong,
only large.

```go
keys := containers.CollectVectorFromKeys(d)      // one of eighteen
for k := range d.All() { }                       // iteration unchanged, and still copies
```

**It does nothing for iteration** — it fixes construction only, so the caller who
wants to walk keys cheaply is no better off.

### 5B. Shape adapters that preserve the length

```go
// sketch
func Keys[K, V any](e Elems2[K, V]) Elems[K]
func Values[K, V any](e Elems2[K, V]) Elems[V]
```

`CollectVector(containers.Keys(d))` then works with the constructor that already
exists. Two free functions instead of eighteen, composing with every present and
future container, and they keep the length — worth **1.54x** and half the memory.

They cannot fix the wasted value copy on their own: deriving keys from an `All`
pays it, which is 6x at 1 KiB values.

```go
containers.CollectVector(containers.Keys(d))     // construction
for k := range containers.Keys(d).All() { }      // iteration -- see 1B's wrinkle
```

The second line is where the interface-versus-value question bites: `.All()` is
needed because `Elems` is an interface. Under 1C it would be `.Seq` and under 1A
the caller would just write `d.Keys()`.

### 5C. 5B, upgrading to a native walk when the source has one

```go
// sketch
func Keys[K, V any](e Elems2[K, V]) Elems[K] {
	if n, ok := e.(interface{ Keys() iter.Seq[K] }); ok {
		return sized[K]{e.Len(), n.Keys()}   // never touches a value
	}
	return sized[K]{e.Len(), dropValue(e.All())}
}
```

The `io.WriterTo` pattern: an optional interface, used when present. Measured
**2.1x at narrow values and 6.3x at wide ones**, and flat in value width.

It only pays off if containers offer `Keys()`/`Values()` natively — which is
direction 1A. **Problem 5's best answer is downstream of problem 1's**, and this
is the clearest evidence for 1A yet: the same method that fixes the naming
mismatch also removes a 6x cost here.

```go
containers.CollectVector(containers.Keys(d))     // 6.3x today's route at wide values
for k := range d.Keys() { }                      // and the caller who only wants to
                                                 // iterate skips the adapter entirely
```

### 5D. Source-side methods, and no adapters at all

If a container's `Keys()` returns an `Elems[K]` rather than a bare `iter.Seq[K]`,
the free functions are unnecessary:

```go
// sketch
containers.CollectVector(d.Keys())
containers.CollectHashSet(d.Values())
```

One constructor per container, no adapters, no explosion, and the length travels
with the shape. This is where 1A and 1C together lead, and it is the smallest
final surface of any option here.

```go
containers.CollectVector(d.Keys())               // construction
for k := range d.Keys() { }                      // iteration -- if Keys returns a
                                                 // bare iter.Seq
for k := range d.Keys().Seq { }                  // ...or this, if it returns a sized
                                                 // value carrying its length
```

**Those last two lines are the whole of decision-blocker 2**, written out. One of
them is what every caller types for the commonest operation in the package, and
the choice is between a `.Seq` on every iteration and a second method for the
sized form.

Against: it needs every container to grow shape methods that return a sized
thing, which is the largest change in this ADR, and it depends on whether `Elems`
stays an interface — decision-blocker 2.

### 5E. Element transformation is a different axis, and should not be conflated

A caller who wants `Vector[string]` from `HashDict[int, User]` needs a function,
not a shape:

```go
// sketch
func TransformSeq[A, B any](seq iter.Seq[A], f func(A) B) iter.Seq[B]
```

```go
for name := range containers.TransformSeq(d.Keys(), strings.ToUpper) { }
```

None of 5A–5D address that, and none should. Recorded so the two are not solved
together by accident — shape adaptation is mechanical and can preserve a length;
transformation is arbitrary and cannot in general.

## What this ADR does not do

It does not decide. Five things should be settled before it becomes a decision,
and three of them are already open elsewhere. They are listed in dependency
order, not importance order:

0. **What a key and a value are** (problem 4), which is logically prior to the
   rest: it decides whether `Vector` is an `Elems2[int, T]`, whether ordered
   containers should convert their keys, and whether a position is a key at all.
   Problem 5's answer is downstream of it too.

1. **Whether `All` should mean `Seq2`**, matching the stdlib. Everything in
   problem 1 follows from that answer — and the corrected finding means the
   answer also decides whether callers with wide values can walk keys cheaply,
   which is a performance question rather than a naming one.
2. **Whether `Elems` stays an interface.** ADR `0006` and `0008` assume it does.
3. **Whether `Range` returns a view** (ADR `0013`'s follow-up) and whether the
   ordered contract tier lands (ADR `0008`'s). Problem 3's shape depends on both.
4. **Whether ADR `0012` decision 3 should be revisited** — ordered containers
   converting values only. It is sound on safety and silent on abstraction, and
   problem 4 is where that shows. Note that ADR `0016` records this decision as
   load-bearing for a different reason: it is what lets `SortedDictView` embed
   `DictView`. Changing it reopens that too.

## Follow-ups absorbed into this ADR

These were recorded elsewhere and belong here now:

- ADR `0015`'s bulk-mutator consistency follow-up — problem 2.
- ADR `0016`'s "the iteration contracts want their own ADR" — this document.
- ADR `0013`'s view-iteration allocation erratum: iterating a view allocates
  three times per call, because `All` returns a closure through a dynamic call.
  It is a cost of the contract's *shape* and belongs in whatever replaces it.

## Proposals

The directions above are per-problem alternatives. A proposal combines them into
one coherent design and says which directions it takes. Proposals are numbered
by letter; more may be added before this ADR decides. There are four: **A** a
sealed `Collector` primitive, **B** a `Source` value, **C** no primitive at all,
and **D** materialising to slices.

The directions use the vocabulary of the time — `Elems`, `CollectX`,
`CollectXSeq` — because they were written against the package as it stands.
Proposal A renames or removes several of those, and says so where it does. When
the two disagree, the proposal is the later word.

### Proposal A: a `Collector` primitive

Every bulk operation — construction, insertion and removal — takes any number of
`Collector`s. A `Collector` abstracts away *where a sequence of entries comes
from* — a container's keys, its values, its pairs, a bare iterator, or a literal
list — so the consumer never learns which.

```go
type Collector[T any] interface {
	SizeHint() (int, bool)
	AsSeq() iter.Seq[T]
	sealedCollector()
}

type Collector2[K, V any] interface {
	SizeHint() (int, bool)
	AsSeq2() iter.Seq2[K, V]
	sealedCollector()
}
```

Two methods each, and the two are symmetric. `SizeHint` reports whether it knows,
so "empty" and "unknown" stop being the same answer, and a constructor forwards
the source's hint rather than calling a length. `AsSeq`/`AsSeq2` are the only way
to read the entries.

**There is deliberately no `AsSlice`.** An earlier draft had one, so that a
consumer whose source was already a slice could take a memmove instead of an
iterator. It was removed as premature: it is worth **1.9x** to a consumer that
*retains* what it collects and **nothing measurable** to one that does not — and
it cost an aliasing hazard, an ownership vocabulary (`Items` versus a
no-copy variant), and a rule about when a returned slice may be kept.

The option stays open at no cost. `Collector` is sealed, so **adding a method to
it later is not a breaking change** — nothing outside this package implements
one, which ADR `0013` established when it chose sealed view interfaces. If a
measured call site ever wants the 1.9x, `AsSlice` can be reintroduced then,
against evidence rather than in anticipation.

It is also the *designated* place for that optimisation. `AppendMany` was the
alternative — a variadic bulk method buying the same win on one container — and
is rejected below for exactly that reason: `AsSlice` buys it for every consumer
of every collector, and a per-container method buys it once. **If the slice path
is ever wanted, it goes here.**

**Sources.** A container advertises which shapes it can produce:

```go
type HoldsKeys[T any] interface   { Keys() iter.Seq[T] }
type HoldsValues[T any] interface { Values() iter.Seq[T] }

// A pair source must also produce each half natively.
type HoldsAll[K, V any] interface {
	HoldsKeys[K]
	HoldsValues[V]
	All() iter.Seq2[K, V]
}

// The size, when there is one, is an optional upgrade.
type CanLen interface{ Len() int }
```

**A source says nothing about size.** `HoldsKeys` is a single method, which is
about as low as an interface bar goes — a caller's own type needs one method to
be a source. The size arrives by type assertion inside the collector
constructors:

```go
func KeysOf[T any](h HoldsKeys[T]) Collector[T] {
	n, ok := 0, false
	if l, is := h.(CanLen); is {
		n, ok = l.Len(), true
	}
	return collector[T]{n, ok, h.Keys()}
}
```

Three things fall out, and they are why this beats putting `SizeHint` on the
sources:

- **Absence is expressed by absence.** A key-bounded sub-range writes nothing at
  all, where a mandatory `SizeHint` made it write
  `func (r keyRange) SizeHint() (int, bool) { return 0, false }` — boilerplate
  declaring a negative.
- **`Len()` is the spelling containers already use.** No parallel vocabulary, and
  nothing in the public contract that exists only to serve the constructors.
- **The embedding gets simpler.** With `SizeHint` on both halves, `HoldsAll`
  embedded two interfaces declaring the same method and relied on Go permitting
  that. Now their method sets are disjoint.

The risk is that `Len()` becomes load-bearing implicitly: a type that has one for
its own reasons will have it used as a hint. That is safe here for the reason ADR
`0006` already gives — **a size hint that disagrees with what the iterator yields
produces a worse allocation, never a wrong result** — and `Len()` meaning a cheap
element count is a convention Go applies everywhere. It wants a doc line on the
constructors, not a guard.

Verified: a container reports `(3, true)`, a key-bounded sub-range with no `Len`
method reports `(0, false)`, a reversed container reports `(3, true)`, and all
three collect correctly.

The embedding is not just tidiness. It means anything that can produce pairs
must *offer* a key-only and value-only walk, which is what stops `KeysOf(dict)`
falling back to deriving and paying 14.9x at wide values. A source that genuinely
has only pairs — a zip of two iterators, say — goes through `AllFrom(seq)` and
never claims to be a `HoldsAll`.

**The interface requires the method, not that it be cheap**, and that distinction
is worth stating because the 14.9x rests on it. This satisfies `HoldsAll` and
pays the full cost:

```go
func (d *lazyDict) Keys() iter.Seq[string] {
	return func(y func(string) bool) {
		for k := range d.All() { if !y(k) { return } }   // copies every value
	}
}
```

Nothing catches that. It is a convention — a `Keys()` walks keys without
materialising values — and unlike `Len()`'s cheapness, which Go establishes
everywhere, this one is local to this package and has no precedent to lean on.
It wants stating on `HoldsKeys` and `HoldsValues`, and it is the thing to check
when a container is added.

**A set is a `HoldsKeys`, not a `HoldsValues`**, per problem 4's taxonomy: a
set's element is its key, and a set has no value. So `KeysOf(mySet)` is the
spelling and `ValuesOf(mySet)` does not compile.

**Constructors.** Eight, covering every source:

```go
func KeysOf[T any](h HoldsKeys[T]) Collector[T]
func ValuesOf[T any](h HoldsValues[T]) Collector[T]
func AllOf[K, V any](h HoldsAll[K, V]) Collector2[K, V]
func ItemsFrom[T any](seq iter.Seq[T]) Collector[T]
func AllFrom[K, V any](seq iter.Seq2[K, V]) Collector2[K, V]
func Items[T any](vs ...T) Collector[T]

// For a caller who has an iterator and knows how long it is.
func ItemsFromSized[T any](n int, seq iter.Seq[T]) Collector[T]
func AllFromSized[K, V any](n int, seq iter.Seq2[K, V]) Collector2[K, V]
```

What each reports for size: `KeysOf`, `ValuesOf` and `AllOf` forward whatever the
source's `CanLen` says, or `(0, false)` when it has none. `Items` knows its own
length, so `(len(vs), true)`. `ItemsFrom` and `AllFrom` report `(0, false)`, and
the `Sized` pair report `(n, true)`.

The `Sized` pair exists because `Collector` is sealed: a caller reading *n*
records, or holding a length from anywhere else, otherwise has no way to say so
and no way to implement a collector that could. **A wrong `n` is safe only
downward**: an under-estimate degrades to the unsized cost, but an over-estimate
retains the memory it asked for — 16x too large measures 3x *worse* than passing
no hint at all. These constructors want a doc line saying **prefer an
under-estimate**, not that a wrong `n` is harmless; see the erratum below.

**Consumers.** Per container: one constructor, one bulk insert and one bulk
delete — not one of each per source shape:

```go
func NewVector[T any](cs ...Collector[T]) *Vector[T]
func NewHashSet[T comparable](cs ...Collector[T]) *HashSet[T]
func NewHashDict[K comparable, V any](cs ...Collector2[K, V]) *HashDict[K, V]

func (v *Vector[T]) AppendAll(cs ...Collector[T])
func (s *HashSet[T]) AddAll(cs ...Collector[T])
func (d *HashDict[K, V]) SetAll(cs ...Collector2[K, V])

// Map stops being empty here: it is a collector target as well as a source.
// It gets the bulk methods but no constructor; see below.
func (m Map[K, V]) SetAll(cs ...Collector2[K, V])
func (m Map[K, V]) DeleteAll(cs ...Collector[K])

// Removal is by KEY, so it takes Collector, never Collector2.
func (s *HashSet[T]) Delete(k T)
func (s *HashSet[T]) DeleteAll(cs ...Collector[T])
func (d *HashDict[K, V]) Delete(k K)
func (d *HashDict[K, V]) DeleteAll(cs ...Collector[K])

// Ordered containers add two more; see the two sections below.
func (d *SortedDict[K, V]) Backward() HoldsAll[K, V]
func (d *SortedDict[K, V]) Range(lo, hi K) RangeAll[K, V]
```

### One constructor per container, variadic in collectors

`New<Container>` takes any number of collectors and `Collect<Container>` goes
away. Three functions per container become one:

| today | after |
|---|---|
| `NewVector[T]()` | `NewVector[T]()` |
| `NewVector(vs ...T)` | `NewVector(Items(vs...))` |
| `CollectVector(src Elems[T])` | `NewVector(ValuesOf(src))` |
| `CollectVectorSeq(seq)` | `NewVector(ItemsFrom(seq))` |

**Thirteen functions become five.** Counted honestly against what exists rather
than against a filled matrix: `SortedSet`, `HashDict`, `SortedDict` and `Vector`
have all three today, `HashSet` has only `NewHashSet` (problem 2's ragged row),
and `Map` has none. The eighteen a full 6x3 matrix would hold was never reached —
which is the point of problem 2 — so the saving is thirteen down to five, and
`Map` stays at zero for the reason given below. It also removes a
redundancy the old pair had grown: `NewVector(1, 2, 3)` and
`CollectVector(Items(1, 2, 3))` were two spellings of one thing.

**`Collect<Container>` disappears entirely**, which reaches outside this ADR:
`0004`, `0006`, `0009` and `0015` all name those functions. They are not wrong —
they describe the package as it was when each was written — but adopting proposal
A means the names in them stop resolving, so each wants a pointer rather than a
rewrite. The `Collect` prefix also echoed `slices.Collect` and `maps.Collect`,
and that echo is given up; `NewVector(KeysOf(d))` reads well enough to be worth
it.

Several collectors are **unioned in order**, which is the composition the split
form could not express:

```go
containers.NewHashSet(containers.KeysOf(a), containers.KeysOf(b))
containers.NewVector(containers.ValuesOf(x), containers.ItemsFrom(seq))
```

Three rules make that well-defined.

- **Order is significant, and later wins.** A `Vector` concatenates, a `HashSet`
  unions, and a dict takes the last value for a repeated key. That follows ADR
  `0004`, which fixed last-write-wins for bulk insertion — and its warning
  applies with more force here, because several collectors give more ways to
  produce a duplicate than one ever did. `slices.CompactFunc` keeps the **first**
  of each run, so a sorted-dict implementation that sorts and compacts gets this
  backwards, and the test must use **distinguishable values** to see it. That bug
  has occurred in this repository once already.
- **Size hints sum, and unknown ones are skipped.** `NewVector(KeysOf(d),
  ItemsFrom(seq))` presizes for the dict half rather than giving up because the
  other half declined. A partial total under-reports, which degrades to the
  unsized cost and never a wrong result. Poisoning the whole sum on one
  unknown would discard good information for nothing. **Summing can also
  over-report** — `NewHashSet(KeysOf(a), KeysOf(b))` on overlapping sources sums
  to 2n for a set that holds n — which is the one direction that is not free;
  bounded by the number of collectors, so mild, but see the erratum below.
- **Zero collectors needs an explicit type argument** — `NewVector[int]()`, since
  there is nothing to infer from. That is exactly today's behaviour for
  `NewVector[int]()`, and it is rarely the right spelling anyway: ADR `0002`
  gives every container a usable zero value, so `var v containers.Vector[int]` is
  the idiomatic empty.

**What it costs**, measured: constructing through a collector is 2–4x the
variadic form — 99.5 ns against 23.8 ns for a three-element literal, 2.379 µs
against 1.119 µs for 1024 elements. The large-input half of that gap is
per-element iteration against a memmove, which is exactly what `AsSlice` is
reserved to recover; that reservation and this decision stay coherent, because
fixing it once on `Collector` fixes it for every constructor at once.

### Bulk removal, and `Remove` becoming `Delete`

Removal was missing from every earlier draft: a caller could bulk-*add* from any
source and could only remove one element at a time. `Collector` makes the
symmetric operation nearly free, so it is added.

```go
func (s *HashSet[T]) Delete(k T)
func (s *HashSet[T]) DeleteAll(cs ...Collector[T])
func (d *HashDict[K, V]) Delete(k K)
func (d *HashDict[K, V]) DeleteAll(cs ...Collector[K])
func (m Map[K, V]) DeleteAll(cs ...Collector[K])   // Delete(k K) it already has
```

**`HashSet.Remove` and `SortedSet.Remove` become `Delete`.** That is problem 4's
taxonomy applied: a set's element *is* its key, and the operation that removes a
key is called `Delete` everywhere else in the package. One name for one idea.

**`DeleteAll` takes `Collector[K]`, never `Collector2`.** You set pairs and you
delete keys, which makes the signature identical across sets and dicts — again
because a set's element is its key. It is the clearest payoff the taxonomy has
produced.

Four rules, and the third is the one that matters:

- **Order is irrelevant.** Deletion is idempotent and commutative, so unlike
  `New` and `AddAll`, the sequence in which collectors are drained cannot be
  observed. Deleting a key that is not present is a no-op, exactly as `Delete`
  is.
- **Size hints are ignored** by the hash containers, which allocate nothing to
  delete. A sorted container may still want one — bulk deletion from a sorted
  slice is the mirror of ADR `0004`'s bulk insertion, and a count would let it
  compact in one pass rather than shifting per key. Left to the implementation.
- **`DeleteAll` drains its collectors before deleting anything.** This is not an
  optimisation, it is required for correctness. The obvious spelling of "empty
  this container" is `d.DeleteAll(KeysOf(d))`, and deleting from a container
  while walking it is exactly what ADR `0008`'s `MutableDict` forbids. Streaming
  it silently corrupts a sorted container:

  ```
  streaming    d.DeleteAll(KeysOf(d)) -> [b]   (wanted [])
  materialised same call              -> []
  ```

  Not an error, not a panic — a wrong answer, and only when the source happens to
  be the receiver. `AddAll` has no equivalent hazard, because appending does not
  shift existing elements while deletion does. The cost is one allocation of the
  keys, which is noise against deletion that is already O(n) per key on a sorted
  slice. **The abstraction is what makes this easy to write**, so the abstraction
  pays for it.
- **Zero collectors still touches the receiver**, per the eager-dereference rule
  above: `d.DeleteAll()` on a nil receiver panics. **`Map` is the exception**, and
  it is the pre-existing one: it is a defined map type with value receivers, so
  there is nothing to dereference. On a nil `Map`, `DeleteAll` is a no-op and
  `SetAll` panics only once it actually writes — which is precisely what
  `delete(m, k)` and `m[k] = v` do on a nil builtin map. `Map.Set` and
  `Map.Delete` already behave this way; the bulk forms inherit it rather than
  introducing it, and ADR `0009`'s reason for `Map` existing is that builtin
  semantics show through.

`Vector` gets neither. It has no removal today, which is the oversight ADR `0015`
records rather than something this proposal should settle.

**`Map` gains `SetAll`.** ADR `0009` keeps it deliberately thin — an adapter, not
a default — but "thin" was priced when a bulk insert meant two methods and an
`Elems2`. One method taking collectors is cheap enough that leaving `Map` as the
only dict that cannot be filled in bulk is harder to justify than adding it.

Which forces the rest of `Map`'s row to be decided too, rather than left to the
omission that produced the ragged matrix in problem 2:

- **`DeleteAll` follows `SetAll` directly.** The same argument transfers intact —
  `Map` would otherwise be the only dict that can be filled in bulk but not
  emptied in bulk — and `Map` already spells the single-key form `Delete(k K)`.
  It is the container the sets' `Remove`→`Delete` rename aligns *toward*, not
  away from, so `Map` ends up with the same removal pair as every other dict.
- **`NewMap` is refused**, and this is the one place ADR `0009`'s thinness still
  does real work. `Map` exists so that builtin syntax applies to it:
  `Map[string]int{"a": 1}` and `make(Map[string]int, n)` already construct one.
  A `NewMap` would compete with a composite literal rather than enable anything,
  and it is the only constructor in the package that would. **This is a real
  exception to "one constructor per container", and it is recorded as such** so
  that a later reader finds a decision rather than a gap.

So `Map` ends at `Set`/`SetAll`/`Delete`/`DeleteAll` — every bulk *method* the
other dicts have, and no constructor.

**Bulk methods are variadic in collectors**, matching `New`: `AddAll`, `SetAll`,
`AppendAll` and `DeleteAll` all take `...Collector`. A single-collector form
would have been the only place in the proposal that was not.

### What happens to the existing mutators

The consumer list above shows the bulk methods and is silent on the rest, which
leaves the obvious question: does a `Collector` subsume `Append`, or sit beside
it? Measured, and the answer differs by arity. (Removal follows the same split,
and is covered in the section above.)

| | | allocs |
|---|---|---|
| `v.Append(1)` | **1.229 ns** | 0 |
| `v.AppendAll(Items(1))` | **60.60 ns** | 144 B |
| `v.AppendMany(src...)` — 1024 ints | **2.860 µs** | — |
| `v.AppendAll(Items(src...))` | 4.196 µs | — |

**Single-element mutators survive.** A `Collector` for one element is **49x** the
direct call and allocates 144 bytes to carry it, so `Append(e T)`, `Set(k, v)`
and their kin stay.

**But the two variadic ones stop being variadic.** `HashSet.Add(vs ...T)` and
`SortedSet.Add(vs ...T)` become `Add(v T)`, with `AddAll(cs ...Collector[T])`
covering the bulk case. That is the whole footprint of the change: they are the
only variadic mutators in the package, and the same two are the only `Remove`s,
so both this and the `Delete` rename land on the same pair.

The sets' variadic-ness was never a considered position. `Vector.Append` was made
single deliberately, on measurement, by ADR `0015`; `Set(k, v)` on the dicts
could never be variadic, because key/value pairs cannot be spread. So the sets
are the outliers rather than the proposal, and after this **no method in the
package is variadic in elements** — only in collectors.

**The `*Seq` twins are subsumed.** `AppendAllSeq(seq)` becomes
`AppendAll(ItemsFrom(seq))`, and likewise `AddAllSeq` and `SetAllSeq`. That is
direction 2B, and it halves that half of the surface.

**`AppendAll` changes its argument** from `Elems[T]` to `Collector[T]`, which is
the whole point.

**`AppendMany` is not added**, which answers ADR `0015`'s follow-up in the
negative.

That follow-up measured a variadic bulk form at 2.1x the alternatives for
appending a plain slice. Against a `Collector` it is **1.47x**, because a
collector carries a size hint where the bare iterator it was compared against did
not — so most of the original gap was the missing presize, and the residual is
memmove against per-element iteration.

1.47x is real, and it is **the same 1.47x `AsSlice` would have recovered**.
Those are two answers to one question, and they differ in reach: a variadic
method buys it on one container, and `AsSlice` buys it for every consumer of
every collector at once. So the slice optimisation, if it is ever wanted, goes on
`Collector` — one place, all callers — rather than being spread a method at a
time across the containers.

That leaves `Vector` with two mutators, `Append` and `AppendAll`, and leaves the
package with no variadic bulk forms at all.

**Caller code:**

```go
containers.NewVector(containers.KeysOf(d))            // problem 5, solved
containers.NewHashSet(containers.ValuesOf(d))
containers.NewHashDict(containers.AllOf(v))           // a vector, keyed by index
containers.NewHashSet(containers.KeysOf(a), containers.KeysOf(b))  // unioned
v.AppendAll(containers.Items("a", "b", "c"))
v.AppendAll(containers.ItemsFrom(slices.Values(names)))
```

**Type inference works**, which is the thing that would have sunk it. `KeysOf(d)`
and `ValuesOf(d)` on a dict that has both methods each resolve to the right one
with no type arguments, and a mismatch is a compile error naming the method:

```
Collector[int] does not implement Collector[string] (wrong type for method AsSeq)
        have AsSeq() iter.Seq[int]
```

**What it takes from the directions.** 1A, for the native `Keys`/`Values` that
`HoldsKeys` and `HoldsValues` require — without them `KeysOf` cannot avoid the
discarded-value copy. 2B, since a `Collector` carries the length and the
`X`/`XSeq` split collapses. 5B, of which the collector constructors are a
generalisation. 5A and 5D become unnecessary.

5C's optional-interface upgrade survives in a different place: `KeysOf` no longer
needs it, because `HoldsAll` requires a native `Keys()` outright, but `CanLen`
uses exactly that pattern for the size.

### Reverse, as a source rather than a method per operation

A container that can be read backwards has one method, returning **a source**
rather than an iterator:

```go
func (d *SortedDict[K, V]) Backward() HoldsAll[K, V]   // matches slices.Backward
func (v *Vector[T]) Backward() HoldsAll[int, T]
func (s *SortedSet[T]) Backward() HoldsKeys[T]
```

**No new interface is needed.** Because `HoldsAll` embeds the other two, a
`HoldsAll[K, V]` is already a `HoldsKeys[K]` and a `HoldsValues[V]`, so the
static return type keeps every shape reachable.

The reuse is the point: a reverse source plugs into everything a forward one
does, so reverse composes with the whole `Collector` machinery rather than
needing its own vocabulary.

```go
for k, v := range sd.Backward().All() { }          // iterate in reverse
for _, v := range sd.Backward().All() { }          // reverse values, discarding
                                                    // a narrow key -- free
containers.NewVector(containers.KeysOf(sd.Backward()))  // collect in reverse
containers.NewHashSet(containers.ValuesOf(v.Backward()))
```

This only works because of the embedding. An earlier draft declared
`HoldsAll` without it, and the static return type then hid the concrete type's
other methods:

```
in call to KeysOf, type HoldsAll[string, int] of d.Backward()
does not match HoldsKeys[T] (cannot infer T)
```

Verified with the embedding in place: inference reaches through it, and
`KeysOf(d.Backward())` yields `[c b a]` while `ValuesOf(d.Backward())` yields
`[3 2 1]`. (An earlier draft put a size method on both halves, so the embedding
declared it twice and relied on Go permitting that. With the size moved to
`CanLen`, the two halves have disjoint method sets and the question does not
arise.) The reverse source is one word — a
pointer to the container it reverses — so **`d.Backward()` boxes with zero
allocations.**

The returned source deliberately has no `Backward()` of its own, so a double
reverse is a compile error rather than a no-op.

### Reversibility as a named capability, and what `Range` returns

`Backward()` alone is a method, not a type, so reversibility cannot appear in a
signature. Two interfaces give it a name:

```go
type RangeKeys[T any] interface {
	HoldsKeys[T]
	Backward() HoldsKeys[T]
}

type RangeAll[K, V any] interface {
	HoldsAll[K, V]
	Backward() HoldsAll[K, V]
}
```

`SortedSet` satisfies `RangeKeys[T]`; `SortedDict` satisfies `RangeAll[K, V]`;
`Vector` satisfies `RangeAll[int, T]`. Generic code can now demand
reversibility:

```go
func lastKey[K, V any](r RangeAll[K, V]) (K, bool) {
	for k := range r.Backward().Keys() { return k, true }
	var zero K
	return zero, false
}
```

**And `Range` returns one of these rather than a sequence.**

```go
func (d *SortedDict[K, V]) Range(lo, hi K) RangeAll[K, V]
func (s *SortedSet[T]) Range(lo, hi T) RangeKeys[T]
```

That is what the names are for: a `RangeAll` is *what `Range` gives you*, and a
whole container is simply the widest one. **This closes the sub-range reverse
gap**, which was proposal A's last open item, and it closes it by composition
rather than by adding a method per ordered operation:

```go
sub := sd.Range(lo, hi)
for k, v := range sub.All() { }                       // forward over the range
for k, v := range sub.Backward().All() { }            // reversed over the range
containers.NewVector(containers.KeysOf(sub.Backward()))
lastKey(sub)                                          // generic code takes it too
```

Verified: a sub-range of `[a b c d e]` over `[1, 4)` yields `[b c d]` forward and
`[d c b]` backward, `KeysOf(sub.Backward())` collects `[d c b]`, and a generic
function over `RangeAll` accepts a sub-range as readily as a container. It
reports **no** size hint, per the consequence below — an early draft measured a
hint of 3 here, from an index-bounded range that has since been rejected.

Three consequences worth stating:

- **A sub-range holds its bounds as keys, not indices, and simply has no `Len`.**
  That is what the optional `CanLen` buys. Measured over 4096 keys:
  constructing an index-bounded range costs **48.5 ns** for two binary searches,
  a key-bounded one **3.4 ns**. More importantly the index form is *wrong* once
  the container changes — inserting a key inside the range makes it miss an
  element, and inserting one before it slides the whole window:

  ```
  before insert:        byIndex=[b d]   byKey=[b d]
  after inserting "c":  byIndex=[b c]   byKey=[b c d]
  after inserting "a":  byIndex=[a b]   byKey=[a b c d]
  ```

  A key-bounded range re-derives its bounds per operation, so it always reflects
  the container as it stands. Verified: after the container grows, the same
  sub-range yields the current contents of its interval.

  **`Vector` gets no `Range`, and the reason is not the one it appears to be.**
  An index-bounded range over the shipped `Vector` *is* stable: its whole surface
  is `Len`, `At`, `Set`, `Append` and the bulk appends, none of which shifts an
  existing index. Verified — after an `Append` a sub-range still shows the same
  elements, and after a `Set` it shows the update, exactly as every other view in
  this package does.

  That stability is an artifact of an incomplete surface rather than a property
  of sequences. `PopBack`, `Insert` or `Remove` are all plausible additions, and
  the day one lands every outstanding index-bounded range silently starts
  pointing at the wrong elements — no compile error, no failing test. **A
  guarantee that depends on a method not existing yet is not a guarantee**, so
  `Vector.Range` is left out until there is something stable for it to hold.

  The cost is that a sub-range of a `Vector` has no spelling: `At(i)` in a loop
  works and gives up bounds-check elimination. Recorded as a follow-up rather
  than solved.
- **A sub-range cannot be sub-ranged**, and generic code over a `RangeAll` cannot
  narrow one either. `RangeAll` has no `Range` of its own, so
  `sd.Range(a, b).Range(c, d)` is a compile error.

  An earlier draft justified that as "the same shape as `Backward()` returning a
  source with no `Backward()`". It is not the same shape: reversing a reverse is
  *identity*, so forbidding it costs nothing, while narrowing a narrow is
  meaningful — `Range("a","z").Range("m","p")` is just `Range("m","p")`.

  Nor is it a language limit. A recursive declaration compiles:

  ```go
  type RangeAll[K, V any] interface {
      HoldsAll[K, V]
      Backward() HoldsAll[K, V]
      Range(lo, hi K) RangeAll[K, V]   // legal; K need not be constrained here
  }
  ```

  It is left out for now to keep the interface minimal, and recorded as something
  to revisit rather than as something settled.
- **Boxing costs differ.** A container into a `RangeAll` is **0 allocations**,
  being a pointer; a sub-range is **1**, being a pointer plus two indices and so
  not pointer-shaped. Today's `Range` returns a closure, which allocates too, so
  this is a wash rather than a regression.

There is deliberately **no `RangeValues`**. Sets are `HoldsKeys` and dicts and
sequences are `HoldsAll`, so nothing in the taxonomy is values-only — the
asymmetry follows from problem 4 rather than being arbitrary.

This also answers ADR `0013`'s deferred "should `Range` return a view?". It
returns a *source*, which is enough: a source offers iteration and nothing else,
so there is no mutation to deny and no seal to need.

**Surface.** Eight collector constructors, one `New` and one bulk method per container,
plus `Backward` on the ordered ones and `Range` on the sorted ones — roughly
thirty declarations for the whole proposal, of which about twenty are what
direction 5A's eighteen functions were trying to do. 5A covered a third of the
source-shape × target combinations and nothing else; this covers all of them and
adds reverse and sub-ranges on top.

**Rules settled while reviewing the proposal:**

- **`KeysOf` requires a native `Keys()`; it does not fall back to deriving from
  `All()`.** A container that wants to be a key source provides the method. This
  makes proposal A **depend on direction 1A** rather than merely preferring it,
  and the dependency is deliberate: the fallback would silently cost 14.9x at
  wide values.
- **A collector is single-pass unless its source says otherwise, and this is
  documented on `Collector`.** One over a container can be walked repeatedly; one
  over `ItemsFrom(seq)` cannot, and nothing in the interface distinguishes them —
  a second `AsSeq()` pass over a spent iterator yields **nothing, silently**.
  That sits badly with a package that has otherwise paid for loud failure, and
  there is no way to catch it in the type system, so it is stated instead: *a
  consumer reads a collector once.*
- **A nil `Collector` panics when used, and nothing checks for it.** `var c
  Collector[int]` is a nil interface, so `NewVector(c)` panics on first use.
  Same rule as ADR `0013` decision 8 for views, restated here so it is not
  rediscovered.
- **Bulk methods dereference their receiver eagerly, and constructors dereference
  the collector eagerly.** ADR `0002`'s rule, which this proposal is unusually
  exposed to: `AppendAll`, `AddAll` and `SetAll` all take an argument that can
  legitimately be empty, and the natural implementation — range the collector,
  insert — never touches the receiver when it is. Verified: a nil `*Vector` with
  an empty collector does **not** panic that way, and does once the receiver is
  read on a path that always runs. The rule has been broken three times in this
  package; a new method family taking a possibly-empty argument is exactly where
  it breaks again.
- **Sealing costs callers nothing.** A caller's own type needs one method —
  `Values()` — to satisfy `HoldsValues[T]` and work with `ValuesOf`; a `Len()`
  alongside it is picked up as a size hint. Sealing only prevents implementing
  `Collector`, which nothing needs to do.
- **There is one way to read a collector, so there is nothing for a helper to
  hide.** An earlier draft gave `Collector` an `AsSlice` alongside `AsSeq`, which
  raised the question of whether the package should provide a helper to branch
  between them. Removing `AsSlice` removes the question: every consumer writes
  one loop over `AsSeq`.

  The measurements that informed that draft are kept, because they bound what was
  given up. A hand-written branch beat an `AsSeq`-only consumer by ~15% against a
  map-backed target and ~2x against a slice-backed one; a callback helper cost
  4–8% over hand-writing it. Those are the numbers to re-read if `AsSlice` is
  ever reintroduced.
- **`Elems` and `Elems2` are removed**, not renamed or kept as aliases.
  `HoldsValues[T]` is `Elems[T]` with `All` renamed and the length dropped;
  `HoldsAll[K, V]` is `Elems2[K, V]` likewise. ADR `0006` says their purpose *is*
  carrying a length, and that job moves to `CanLen`. The library has no external
  users, so there is nothing to keep them for.

### Proposal A, as it stands

**One sentence.** Every bulk operation — construction, insertion and removal —
takes collectors, which abstract away where a sequence of entries comes from;
containers advertise what shapes they can produce; and a container that reads
backwards returns a reversed source rather than an iterator.

**The surface**, complete:

```go
// Sources -- what a container advertises.
type HoldsKeys[T any] interface   { Keys() iter.Seq[T] }
type HoldsValues[T any] interface { Values() iter.Seq[T] }
type HoldsAll[K, V any] interface { HoldsKeys[K]; HoldsValues[V]; All() iter.Seq2[K, V] }
type CanLen interface             { Len() int }   // optional; found by assertion

// Sources that also read backwards. Range returns one; a container is the widest.
type RangeKeys[T any] interface   { HoldsKeys[T]; Backward() HoldsKeys[T] }
type RangeAll[K, V any] interface { HoldsAll[K, V]; Backward() HoldsAll[K, V] }

// The primitive.
type Collector[T any] interface {
	SizeHint() (int, bool)
	AsSeq() iter.Seq[T]
	sealedCollector()
}
type Collector2[K, V any] interface {
	SizeHint() (int, bool)
	AsSeq2() iter.Seq2[K, V]
	sealedCollector()
}

// Eight constructors.
func KeysOf[T any](h HoldsKeys[T]) Collector[T]
func ValuesOf[T any](h HoldsValues[T]) Collector[T]
func AllOf[K, V any](h HoldsAll[K, V]) Collector2[K, V]
func ItemsFrom[T any](seq iter.Seq[T]) Collector[T]
func AllFrom[K, V any](seq iter.Seq2[K, V]) Collector2[K, V]
func Items[T any](vs ...T) Collector[T]
func ItemsFromSized[T any](n int, seq iter.Seq[T]) Collector[T]
func AllFromSized[K, V any](n int, seq iter.Seq2[K, V]) Collector2[K, V]

// Per container: one constructor, one bulk insert, one bulk delete. Shown for a
// set and a dict; every container follows the same pattern.
func NewHashSet[T comparable](cs ...Collector[T]) *HashSet[T]
func (s *HashSet[T]) Add(v T)                       // no longer variadic
func (s *HashSet[T]) AddAll(cs ...Collector[T])
func (s *HashSet[T]) Delete(k T)                    // was Remove(vs ...T)
func (s *HashSet[T]) DeleteAll(cs ...Collector[T])

func NewHashDict[K comparable, V any](cs ...Collector2[K, V]) *HashDict[K, V]
func (d *HashDict[K, V]) Set(k K, v V)
func (d *HashDict[K, V]) SetAll(cs ...Collector2[K, V])   // Map gains this too
func (d *HashDict[K, V]) Delete(k K)
func (d *HashDict[K, V]) DeleteAll(cs ...Collector[K])    // by KEY, not pairs; Map too

// Ordered containers add two; Vector has no removal.
func (d *SortedDict[K, V]) Backward() HoldsAll[K, V]
func (d *SortedDict[K, V]) Range(lo, hi K) RangeAll[K, V]
```

**Settled**, in summary. Each row's reasoning is in the sections above; this
table is an index, not a second statement of the rules.

| | |
|---|---|
| `HoldsAll` embeds `HoldsKeys` and `HoldsValues` | so pair sources must offer cheap half-walks, and `Backward` needs no interface of its own |
| a set is a `HoldsKeys` | its element is its key; `ValuesOf(set)` does not compile |
| there is **no** `AsSlice` | premature: 1.9x to a retaining consumer, nothing to others, against an aliasing hazard and an ownership vocabulary. Sealed, so it can be added later without breaking anyone |
| `KeysOf` requires a native `Keys()` | no silent fallback to deriving, which would cost 14.9x at wide values |
| no consumer-side helper | there is one way to read a collector, so there is nothing to hide |
| bulk methods deref the receiver eagerly; constructors deref the collector eagerly | ADR `0002`'s rule, and a possibly-empty argument is exactly where it breaks |
| a collector is single-pass unless its source says otherwise | a second `AsSeq()` over a spent iterator yields nothing, silently; stated because the type system cannot catch it |
| a nil `Collector` panics on use, and nothing checks | as ADR `0013` decision 8 for views |
| `HoldsAll` requires the half-walks but cannot require them to be *cheap* | a conforming `Keys()` may derive from `All()`; the 14.9x rests on a convention, so it is stated on the interfaces |
| `SizeHint() (int, bool)` on `Collector`; nothing about size on the sources | "empty" and "unknown" stop being the same answer, and the sources stay clean |
| the size is an optional `CanLen`, found by assertion | absence is expressed by absence, and `Len()` is the spelling containers already have |
| a sub-range holds **key** bounds and declines a hint | 3.4 ns to construct against 48.5 ns, and correct when the container changes |
| reverse is a source, named `Backward` | matching `slices.Backward`; composes with every `Collector` constructor |
| a reverse source has no `Backward` | double reverse is a compile error |
| sealing costs callers nothing | a caller's type with `Len` and `Values` is a `HoldsValues` already |
| `RangeKeys` / `RangeAll` name reversibility | so it can appear in a signature; `Range` returns one, and a container is the widest one |
| a sub-range cannot be sub-ranged | `RangeAll` has no `Range`, as a reverse source has no `Backward` |
| there is no `RangeValues` | nothing in the taxonomy is values-only |
| `New<Container>(cs ...Collector[T])` replaces `New`, `Collect` and `CollectSeq` | thirteen existing functions become five, and several sources compose |
| several collectors union in order, later wins | ADR `0004`'s rule; `slices.CompactFunc` keeps the first, so a sorted implementation must be tested with distinguishable values |
| size hints sum, unknown ones skipped | under-reporting degrades to the unsized cost; over-reporting on overlapping sources is the one unsafe direction, and is bounded by the collector count |
| `Remove` becomes `Delete` on sets; `DeleteAll(cs ...Collector[K])` everywhere | a set's element is its key, so removal takes keys and the signature is identical across sets and dicts |
| `DeleteAll` drains its collectors before deleting | `d.DeleteAll(KeysOf(d))` otherwise corrupts a sorted container silently |
| every bulk method is variadic in collectors | uniform with `New`; nothing in the proposal takes exactly one |
| `HashSet.Add` and `SortedSet.Add` stop being variadic | the only variadic mutators in the package, and the only ones that never had a measured reason |
| `Map` gains `SetAll` and `DeleteAll` | one method taking collectors is cheap enough that being the only dict that cannot be filled or emptied in bulk is not worth defending |
| `Map` gets **no** `NewMap` | the one real exception to one-constructor-per-container: a composite literal and `make` already construct a `Map`, which is the point of the type |
| **`Elems` and `Elems2` are removed**, not renamed or aliased | the `Holds*` family replaces them, `CanLen` takes over the length job, and the library has no external users to break |

**Verified, not assumed.** Inference resolves `KeysOf(d)` and `ValuesOf(d)` on a
dict that has both, with no type arguments, and a mismatch is a compile error
naming the method. Inference also reaches *through* the embedding, which the
un-embedded form failed to do. `d.Backward()` boxes with zero allocations, and
`KeysOf(d.Backward())` yields `[c b a]`.

Three later checks matter as much. A key-bounded sub-range stays correct when the
container grows underneath it, where an index-bounded one silently misses
elements. A `CanLen` assertion finds a container's length and finds nothing on a
sub-range that has none. And the eager-dereference rule is confirmed in both
directions — a nil receiver with an empty collector panics under the guarded
implementation and **not** under the natural one.

**Which problems it closes.** Problem 1, by requiring native per-shape methods.
Problem 3, for whole containers and for sub-ranges. Problem 5, entirely —
`NewVector(KeysOf(d))` is the case that had no spelling.

Problem 2 most thoroughly of all, and by more than the `X`/`XSeq` collapse it
was originally scoped at: `New` and `Collect` fold into one variadic
constructor, `HashSet` gains the bulk methods it never had, `Map` gains
`SetAll` and `DeleteAll`, the two variadic mutators stop being variadic, and bulk *removal* —
which the problem statement did not think to ask for — exists for the first
time.

**Nothing blocks this proposal.** The last blocking item — reverse over a
sub-range — is closed by `Range` returning a `RangeAll` rather than an
`iter.Seq2`, which also answers ADR `0013`'s deferred "should `Range` return a
view?" in the negative: it returns a source, and a source has nothing to deny.

Two things are deliberately deferred, neither of which changes anything decided
above:

- **`RangeAll` declares no `Range`**, so a sub-range cannot be narrowed and
  generic code over one cannot narrow it either. The recursive declaration
  compiles; it is left out to keep the interface minimal, and is the likeliest
  thing to be added later.
- **`Vector` has no `Range`.** Index bounds over the shipped `Vector` are stable
  today, but only because nothing in its surface shifts an index — a `PopBack`
  or `Remove` would end that silently. Deferred until there is something stable
  to hold, which leaves a `Vector` sub-range with no spelling short of `At(i)` in
  a loop.

**Problem 4 is settled** and no longer gates this proposal — the taxonomy is
adopted, ordered containers keep values-only conversion as a recorded exception,
positions are never converted, and "key" keeps two documented scopes.

**The cost to callers**, stated plainly. This is the largest change proposed in
this repository, and it lands on every file that uses the package.

*Iteration* — every set and vector call site:

```go
s.All()  ->  s.Keys()          // a set's element is its key
v.All()  ->  v.Values()
v.AllIndexed()  ->  v.All()    // All now means pairs, as it does in the stdlib
```

*Construction* — every `Collect` call site, and every variadic one:

```go
CollectVector(src)      ->  NewVector(ValuesOf(src))
CollectVectorSeq(seq)   ->  NewVector(ItemsFrom(seq))
NewHashSet(1, 2, 3)     ->  NewHashSet(Items(1, 2, 3))
```

*Mutation* — the sets, and every `*Seq` twin:

```go
s.Add(1, 2, 3)          ->  s.AddAll(Items(1, 2, 3))
s.Remove(x)             ->  s.Delete(x)
d.SetAllSeq(seq)        ->  d.SetAll(ItemsFrom(seq))
```

Nothing here is subtle and none of it is a silent behaviour change — every line
above fails to compile until it is updated, which is the one mercy. But it is a
whole-package break, and it is worth being honest that the proposal's benefits
are mostly *coherence* rather than capability: problem 5's cross-container
construction and problem 3's reverse iteration are genuinely new, and the rest is
the same operations spelled consistently.

### What proposals B and C are testing

Proposal A above is complete and internally consistent. What follows does not
amend it — it puts two rivals beside it so that adopting A is a choice rather
than a default, and so the one measurement that would separate them is on the
record.

Proposal A settled into one shape early and was then refined for many rounds. It
is worth naming what it never justified: **A's entire apparatus exists to move a
length.** The sealed `Collector`, `SizeHint`, the `CanLen` assertion, and the
`ItemsFromSized`/`AllFromSized` pair are all there so a consumer learns how big
its input is. ADR `0006` says a missing hint costs an allocation and never a
wrong result — which tells you the failure is benign, not that it is cheap.

Proposals B and C attack that from opposite sides. B keeps the length and gives
up something else to carry it more cheaply; C gives up the plumbing and makes the
caller supply the length by hand.

**There is a trilemma here, and it is a language fact rather than a taste.**
Three things are wanted from whatever a container's `Keys()` returns:

1. it can be ranged over directly — `for k := range d.Keys()`
2. a length travels with it
3. it needs no wrapper at the call site — `NewVector(d.Keys())`, not
   `NewVector(KeysOf(d))`

**No type gets all three**, because range-over-func requires the operand's core
type to be a function, and a function type cannot carry a field:

```
cannot range over (dict{}).SizedKeys() (value of struct type Sized[string])
```

| | ranges directly | length travels | no wrapper |
|---|---|---|---|
| **A** — `iter.Seq` plus a sealed `Collector` | yes | yes | **no**: `KeysOf(d)` |
| **B** — a `Source[T]` struct the container returns | **no**: `.Seq` | yes | yes |
| **C** — bare `iter.Seq`, no primitive | yes | **no** | yes |

A fourth corner was checked and is worse than all three. A *defined function
type* — `type Source[T any] func(yield func(T) bool)` — does range directly and
does carry methods, both verified. But it is a named type with a named
counterpart, so it is not assignable to an `iter.Seq[T]` parameter:

```
type Source[string] of dict{}.Keys() does not match inferred type iter.Seq[string]
```

That severs the package from `slices.Collect`, `slices.Sorted`, `maps.Keys` and
every other stdlib function taking an `iter.Seq`, in exchange for method syntax.
**Rejected**, and recorded here so it is not rediscovered: the stdlib iterator
vocabulary is worth more than dot-chaining.

### Proposal B: `Source[T]`, a sized value the container hands you

One concrete struct replaces A's six interfaces. A container returns it directly,
so there is no wrapper to spell.

```go
type Source[T any] struct {
	Seq  iter.Seq[T]     // forward; never nil
	Back iter.Seq[T]     // backward; nil when the source has no order
	N    int             // valid only when Known
	Known bool
}

type Source2[K, V any] struct {
	Seq   iter.Seq2[K, V]
	Back  iter.Seq2[K, V]
	N     int
	Known bool
}
```

```go
func (d *HashDict[K, V]) Keys() Source[K]
func (d *HashDict[K, V]) Values() Source[V]
func (d *HashDict[K, V]) All() Source2[K, V]
func (s *HashSet[T]) Keys() Source[T]
func (v *Vector[T]) Values() Source[T]

func NewVector[T any](srcs ...Source[T]) *Vector[T]
func (v *Vector[T]) AppendAll(srcs ...Source[T])
func (s *HashSet[T]) AddAll(srcs ...Source[T])
func (s *HashSet[T]) DeleteAll(srcs ...Source[T])

// Two free functions instead of eight constructors.
func Items[T any](vs ...T) Source[T]
func From[T any](seq iter.Seq[T]) Source[T]
```

**Caller code:**

```go
containers.NewVector(d.Keys())                  // problem 5, with no wrapper
containers.NewHashSet(a.Keys(), b.Keys())       // unioned, still variadic
v.AppendAll(containers.Items("a", "b"))
for k := range d.Keys().Seq { ... }             // the tax, paid here
```

**What it collapses.** `HoldsKeys`, `HoldsValues`, `HoldsAll`, `CanLen`,
`RangeKeys`, `RangeAll`, `Collector` and `Collector2` — eight named types in A —
become `Source` and `Source2`. Reversibility stops being a *type* and becomes a
**nil-able field**: `Backward` is `Source{Seq: s.Back, Back: s.Seq, ...}`, a
struct copy with two fields swapped, no allocation and no boxing. `Range` returns
a `Source2` whose `Back` is populated, which is all `RangeAll` ever meant.

**What it costs, and it is not small.**

- **`for k := range d.Keys()` stops compiling.** It becomes `range d.Keys().Seq`.
  This lands on the single most common operation in the package, and it is the
  reason B is not obviously better than A despite being much smaller.
- **Capability leaves the type system.** A function that requires a reversible
  source cannot say so in its signature; it takes a `Source[T]` and checks
  `if s.Back == nil`. A's `RangeAll` is a compile-time promise, and B downgrades
  it to a runtime one. This is the real axis between A and B: **declared
  capability versus discovered capability.**
- **The struct is public and its fields are public**, so its layout is the API.
  A's `Collector` is sealed and can gain methods without breaking anyone;
  `Source` cannot gain a field without changing every composite literal. A caller
  who writes `Source[int]{Seq: s}` pins the shape forever. Mitigable by
  unexporting the fields and adding accessors — which reintroduces method calls
  and costs B its simplicity.
- **A nil `Seq` is a panic with no seal to prevent it.** `var s Source[int]` is a
  valid value whose `Seq` is nil, and `range` over a nil `iter.Seq` panics. A's
  sealed interface makes the equivalent unconstructible.

**What it likely wins**, unmeasured: `Source` is a value of four words passed
directly, where A's `Collector` boxes into an interface — measured at 144 B for a
single-element collector. B should be cheaper per bulk call. **This wants
measuring before B is taken seriously**, and it is the one number that could
justify paying the `.Seq` tax.

### Proposal C: no primitive, and the caller sizes it

C is the minimal proposal, and it exists because this ADR's own accounting admits
that A's benefits are "mostly coherence rather than capability". C asks what is
left if only the capability is bought.

Containers expose the stdlib's vocabulary and nothing else:

```go
func (d *HashDict[K, V]) Keys() iter.Seq[K]
func (d *HashDict[K, V]) Values() iter.Seq[V]
func (d *HashDict[K, V]) All() iter.Seq2[K, V]
func (d *SortedDict[K, V]) Backward() iter.Seq2[K, V]
func (d *SortedDict[K, V]) Range(lo, hi K) iter.Seq2[K, V]
func (d *SortedDict[K, V]) RangeBackward(lo, hi K) iter.Seq2[K, V]

// The only addition: presizing, made explicit.
func (v *Vector[T]) Grow(n int)
func (s *HashSet[T]) Grow(n int)
```

**There are no bulk methods and no bulk constructors.** `AddAll`, `SetAll`,
`AppendAll`, `DeleteAll`, `Collect<Container>` and the `*Seq` twins are all
deleted rather than unified. Bulk anything is a range loop:

```go
var v containers.Vector[string]
v.Grow(d.Len())
for k := range d.Keys() {
	v.Append(k)
}
```

**Problem 5 dissolves rather than being solved.** "Put a dict's keys into a
vector" needed a new spelling only because the operation was a *constructor*,
and constructors multiply with the cross-product of source shapes and
destinations. As a loop it is the same three lines for every pair of containers
that will ever exist, including ones outside this package.

**Problem 2 dissolves the same way.** The ragged matrix — `HashSet` missing two
functions, `Map` missing three — stops being ragged when the matrix is empty.
There is no consistency to maintain across containers because there is nothing to
be consistent about.

**Problem 1 is fixed directly** by the `Keys`/`Values`/`All` rename, which every
proposal shares; it never needed a primitive.

**Problem 3 is fixed by naming the four cases.** `Backward` and `RangeBackward`
are two more methods on two containers, against A's `RangeKeys`/`RangeAll`
machinery. Combinatorially worse if a fifth ordered container arrives, and
completely clear until then.

**The size hint becomes the caller's job, and this is C's central bet.** The
measurement below shows presizing is worth **2.5x to 3.8x** — far too much to
discard. C does not discard it; it moves it from a plumbed value to an explicit
call. The caller writes `v.Grow(d.Len())` on the line above the loop, where it is
visible, and where a caller who knows something the container does not — that the
loop will `continue` past most of the input, say — can size it correctly, which
no automatic hint can.

**What it costs:**

- **The hint can be forgotten, and forgetting is silent.** A caller who omits
  `Grow` pays the full 3.8x and nothing tells them. A's plumbing cannot be
  forgotten, and that is the strongest argument against C.
- **Three lines instead of one**, at every bulk site.
- **No signature can say "fillable".** A function that populates a caller's
  container takes... nothing. There is no type for it, so the caller does the
  filling and passes the result.
- **Multi-source union is two loops**, which is fine, and gets the presize wrong
  unless the caller sums the lengths by hand.
- **The `for` loop is not free either.** A range loop over a container's
  `iter.Seq` costs the same yield indirection every proposal pays, so C is not
  faster than A per element — it is only smaller.

**What it wins:** the entire `Collector` surface, the `Holds*` family, the eight
constructor functions, the sealing question, the nil-collector question, the
reusability question, the drain-before-delete correctness hazard — all of it
stops existing, because none of it is built. `DeleteAll(KeysOf(d))` cannot
silently corrupt a sorted container when there is no `DeleteAll`; the caller
writing the loop has to materialise the keys themselves, and will see why.

### What the measurement says about all three

`experiments/iteration` finding 9 prices the thing A is built to carry:

| build, 1024 elements | unsized | sized | |
|---|---|---|---|
| slice | 3.481 µs, 12 allocs, 24.6 KiB | 1.282 µs, 1 alloc, 8 KiB | **2.7x** |
| map | 32.94 µs, 22 allocs, 72.7 KiB | 8.941 µs, 6 allocs, 36.1 KiB | **3.7x** |

At 8 elements a slice still gains 3.5x and **a map gains nothing at all**, the
runtime's small-map path allocating identically either way.

**This vindicates A's central complexity.** A 2.7x-3.7x factor on every bulk
build is not something to hand to callers and hope they remember, which is the
strongest argument against C and the reason A's plumbing earns its keep.

**It also produces an erratum against A.** Proposal A states, of
`ItemsFromSized` and of the summing rule, that "a wrong *n* costs an allocation
and never a wrong result, per ADR `0006`". Measured, that holds only when the
hint is **too small**: hinting 16 for 1024 elements costs 3.425 µs against 3.481
µs unsized, so the hint is merely wasted. Hinting **16x too large costs 10.58 µs
and retains 128 KiB** — 8.3x the correct build, and 3x worse than passing no hint
at all. An over-estimate does not trade an allocation; it trades transient
allocation for retained memory.

Two places in A over-estimate by construction:

- **`ItemsFromSized(n, seq)` takes *n* from the caller**, who may be guessing.
  A's framing invites a generous guess as the safe choice. It is the unsafe one.
- **The summing rule over-reports on overlapping sources.**
  `NewHashSet(KeysOf(a), KeysOf(b))` where `a` and `b` hold the same keys sums to
  2n for a set that will hold n. Bounded at the number of collectors, so it is
  mild — but it is over-estimation on the one path A added specifically to
  compose sources, and A currently documents only the under-reporting direction
  as benign.

Neither sinks A. Both want the doc line on the `Sized` constructors to say
**"prefer an under-estimate"** rather than "a wrong n is harmless", and the
summing rule to say it may over-allocate on overlapping sources.

### Where the three stand

They are not variants; they disagree about what an abstraction is for.

| | A | B | C | D |
|---|---|---|---|---|
| named types added | 8 | 2 | 0 | **1** |
| free functions added | 8 | 2 | 0 | **0** |
| shape methods per dict | 3 | 3 | 3 | **6** |
| `for k := range d.Keys()` | works | **`.Seq`** | works | works |
| size hint | automatic | automatic | **caller** | **intrinsic** |
| reversibility | in the type system | a nil-able field | a named method | `slices.Reverse` |
| streaming / unbounded sources | yes | yes | yes | **no** |
| cost of forgetting | cannot | cannot | **2.7x-3.7x, silent** | cannot |
| measured against A | baseline | -2 allocs, nil at scale | not built | within 16% on narrow elements, **77% slower on wide**, a third of the allocations |

**A** buys the size hint with a wrapper at every call site. **B** buys it with the
range syntax, and collapses eight types into two by demoting capability from
compile time to run time. **C** buys nothing and builds nothing, betting that a
loop and an explicit `Grow` are better than any abstraction over them.

The question this ADR now has to answer is narrow and stated: **is
`NewVector(KeysOf(d))` worth eight named types and eight functions, when
`NewVector(d.Keys())` costs one struct and the range syntax,
`v.Grow(d.Len())` plus a loop costs nothing at all, and
`NewVector(d.KeySlice()...)` is both smaller and faster than all of them provided
the constructor may keep the slice?**

B has now been measured — see the section below. Its case rested on a value being
cheaper than a boxed interface; that is true, and it turns out to buy about 40 ns
and two allocations per bulk call, which evaporates at scale and is partly
refunded by B's own eager reverse half.

### Proposal B, measured against proposal A

B's whole case was that a value is cheaper than a box, and it was the one thing
about B that had not been measured. It is now — `experiments/iteration`,
`abcost.go`, finding 10. **The claim is true, and B spends the winnings on its
own design.**

Constructing a source and nothing else — A's `KeysOf(d)` against B's `d.Keys()`:

| | time | bytes | allocs |
|---|---|---|---|
| unordered, A | 26.22 ns | 40 B | 2 |
| unordered, B | **10.87 ns** | **16 B** | **1** |
| ordered, A | 29.71 ns | **72 B** | 2 |
| ordered, B | 29.22 ns | 96 B | 2 |

**On an unordered container B wins outright**, 2.4x and half the allocations,
which is precisely the value-versus-interface gap it predicted.

**On an ordered container the win is gone and B uses more memory than A.** That
is B's design showing through, not noise. Reversibility in B is a **field**, so
an ordered container builds the reverse closure **eagerly on every call, whether
or not anything ever reads it**. In A reversibility is a **type**, so `Backward`
and `Range` build the reverse half only when asked for it.

Priced directly, against an escaping sink so the unused half cannot be elided:

| | time | bytes | allocs |
|---|---|---|---|
| B, reverse half populated | 43.58 ns | 144 B | 3 |
| B, forward only | **28.86 ns** | **96 B** | 2 |
| A, boxed equivalent | 41.91 ns | 120 B | 3 |

**Carrying the reverse half costs more than the box it avoids.** Forward-only, B
beats A comfortably; reversible, B is slightly slower than A and uses 20% more
memory.

End to end, the picture is smaller than either number suggests:

| build a vector from one container's keys | A | B | |
|---|---|---|---|
| 8 keys, unordered | 132.3 ns, 6 allocs | 103.0 ns, 4 allocs | 1.28x |
| 8 keys, ordered | 116.3 ns, 6 allocs | 74.52 ns, 4 allocs | **1.56x** |
| 1024 keys, unordered | 7.969 µs, 6 allocs | 7.977 µs, 4 allocs | **dead heat** |
| 1024 keys, ordered | 3.278 µs, 6 allocs | 2.990 µs, 4 allocs | 1.10x |
| three sources unioned | 1.711 µs, 14 allocs | 1.557 µs, 8 allocs | 1.10x |
| a literal list | 100.1 ns, 7 allocs | 57.28 ns, 4 allocs | **1.75x** |

**B saves two allocations per bulk call at every size**, and wins 1.3x-1.75x
where the source is cheap. The win **evaporates as the source grows** — at 1024
keys of an unordered container the two are indistinguishable, because the map
walk dominates everything the abstraction does.

**What this decides.** B's advantage is a constant, and bulk construction is the
operation where constants matter least — it is called once per collection built,
against a walk that is linear in the collection. Two allocations and ~40 ns are
real, and they are not obviously worth:

- `for k := range d.Keys()` no longer compiling,
- reversibility moving from a compile-time promise to a runtime `if s.Back == nil`,
- and a public struct whose field layout is the API.

**So B is measured and does not clear the bar it set for itself.** The honest
summary is that A and B cost about the same, and A buys a type system with the
difference. B remains on the record because its *collapse* — eight named types
into two — is a real simplification that a future reader may weigh differently,
and because it produced the finding that eager reversibility is a design cost, which
is a fact about the problem rather than about B.

**One thing B teaches A.** B's reverse half is expensive because it is built
eagerly. A should check it is not doing the same: `Range(lo, hi)` returning a
`RangeAll` must not construct the backward iterator until `Backward()` is called
on it. Nothing in A's write-up says either way, and the cheap version is the one
where `RangeAll` holds the bounds and builds a direction on demand.

### Proposal D: materialise to a slice

C's appeal was raw simplicity, bought by giving up the size hint — which
measured at 2.7x-3.7x and is too much to give up. D keeps C's simplicity and
gets the size back, by making the currency a **slice** rather than an iterator.
A slice knows its own length, so the length stops being something the API has to
carry.

```go
type KeyValue[K, V any] struct {
	Key   K
	Value V
}
```

Containers gain slice-materialising methods alongside the iterators they already
have:

```go
// HashDict -- iteration unchanged, bulk added
func (d *HashDict[K, V]) Keys() iter.Seq[K]
func (d *HashDict[K, V]) Values() iter.Seq[V]
func (d *HashDict[K, V]) All() iter.Seq2[K, V]
func (d *HashDict[K, V]) KeySlice() []K
func (d *HashDict[K, V]) ValueSlice() []V
func (d *HashDict[K, V]) AllSlice() []KeyValue[K, V]

// A set's element is its key, so a set gets one.
func (s *HashSet[T]) Keys() iter.Seq[T]
func (s *HashSet[T]) KeySlice() []T

// A vector is keyed by position.
func (v *Vector[T]) Values() iter.Seq[T]
func (v *Vector[T]) All() iter.Seq2[int, T]
func (v *Vector[T]) ValueSlice() []T
func (v *Vector[T]) AllSlice() []KeyValue[int, T]
```

And every bulk operation is variadic in the element type:

```go
func NewVector[T any](vs ...T) *Vector[T]
func NewHashSet[T comparable](vs ...T) *HashSet[T]
func NewHashDict[K comparable, V any](kvs ...KeyValue[K, V]) *HashDict[K, V]

func (v *Vector[T]) AppendAll(vs ...T)
func (s *HashSet[T]) AddAll(vs ...T)
func (s *HashSet[T]) DeleteAll(ks ...T)
func (d *HashDict[K, V]) SetAll(kvs ...KeyValue[K, V])
func (d *HashDict[K, V]) DeleteAll(ks ...K)

// Map is a bulk target too, and still gets no constructor.
func (m Map[K, V]) SetAll(kvs ...KeyValue[K, V])
func (m Map[K, V]) DeleteAll(ks ...K)
```

**Bulk operations are variadic in the element type, not in the slice type.** A
slice reaches them by spreading — `NewVector(d.KeySlice()...)` — which keeps one
spelling for a literal list and for a materialised source, and leaves unioning to
the caller (`slices.Concat(a.KeySlice(), b.KeySlice())...`) rather than building
composition into every signature. Two consequences fall out, and both are
favourable:

- **A spread costs nothing.** Verified rather than read off the spec: passing
  `src...` costs **0.9363 ns against 0.9256 ns** for a plain `[]T` parameter, at
  **zero allocations either way**, and the callee receives the identical backing
  array. The variadic form is free relative to taking a slice, so the choice
  between them is about spelling, not cost.
- **One spelling covers both cases.** `NewVector(1, 2, 3)` and
  `NewVector(d.KeySlice()...)` are the same function, where a `[]T` parameter
  would have forced either `NewVector([]int{1, 2, 3})` or a second entry point.

**A note for the deferred optimisation below.** A spread carries the source's
*capacity*, not just its length: spreading `src[:2]` from a 1024-element array
hands the callee **len 2, cap 1024** — measured. That is harmless while
constructors copy, which is what D proposes. It is a trap for any future
constructor that adopts, which would write into `src[2]` on its first append and
must therefore store `slices.Clip(vs)`. `bytes.NewBuffer` has the identical
hazard.

**The contract is the whole design.** A `*Slice` result is a **full, independent
copy**: nothing the container does afterwards is visible through it, and nothing
done to it is visible in the container. That single rule is what the rest falls
out of.

**Caller code:**

```go
containers.NewVector(d.KeySlice()...)          // problem 5, no wrapper, no primitive
containers.NewHashSet(v.ValueSlice()...)
containers.NewVector(1, 2, 3)                  // one spelling for literals too
v.AppendAll(other.ValueSlice()...)
d.DeleteAll(d.KeySlice()...)                  // safe by construction
ks := d.KeySlice(); slices.Reverse(ks)        // problem 3, no Backward needed
slices.Sort(ks)                               // the whole slices package applies

// Unioning is the caller's, not every signature's.
containers.NewHashSet(slices.Concat(a.KeySlice(), b.KeySlice())...)
```

**What the contract buys, beyond simplicity:**

- **The size stops being plumbed.** `SizeHint`, `CanLen`, `ItemsFromSized`,
  `AllFromSized`, the hint-summing rule and its over-estimation erratum all
  cease to exist. A slice's length is exact, always known, and free.
- **The self-deletion hazard cannot happen.** A needed an explicit
  "`DeleteAll` drains its collectors before deleting" rule, without which
  `d.DeleteAll(KeysOf(d))` silently corrupts a sorted container. Under D the
  argument is already a copy, so the bug is unconstructible rather than
  documented.
- **Problem 1 is solved by the rename, not by the slices.** `Keys`/`Values`/`All`
  replacing a single overloaded `All` is what removes the collision; `Elems` and
  `Elems2` then have no job left. The `*Slice` names simply mirror that
  vocabulary rather than introducing a second one.
- **Problem 3 is covered for the common case.** Reversal is `slices.Reverse` on
  a copy the caller owns — no `Backward`, no `RangeKeys`, no `RangeAll`. Reading
  a large container backwards *without* materialising it is not covered, and is
  deferred with the rest of the reverse machinery.
- **The stdlib comes with it.** Sorting, compaction, filtering, chunking,
  `slices.Concat` for unioning sources: all of it applies without this package
  providing anything.

**Count against A:** one named type against eight, zero free functions against
eight. The cost is on the other side of the ledger: three extra methods on every
container and every view, since the slices are an addition to the iterators
rather than a replacement for them.

**Measured against proposal A** (`experiments/iteration`, `dslice.go`, finding
11), at 1024 elements. The result is sharper than "it depends":

| | A | D, copying | D, adopting |
|---|---|---|---|
| map keys → vector | 8.097 µs, 6 allocs, 18.1 KiB | 8.766 µs, 2 allocs, **36.0 KiB** | **6.991 µs**, 1 alloc, 18.0 KiB |
| slice → vector | 3.305 µs, 6 allocs, 18.1 KiB | 3.835 µs, 2 allocs, **36.0 KiB** | **1.949 µs**, 1 alloc, 18.0 KiB |
| → map-backed set | 14.99 µs, 11 allocs | **14.17 µs**, 7 allocs | — |
| 1 KiB values ×256 | 36.10 µs, 6 allocs, 256 KiB | **64.02 µs**, 2 allocs, **512 KiB** | **28.47 µs**, 1 alloc, 256 KiB |

**The copy is not the problem; doing it twice is.** A constructor that copies
the slice it was handed materialises once and copies again into the destination,
and that version **loses to A nearly everywhere while doubling peak memory** —
1.8x slower at 1 KiB values, where the payload dominates. A constructor that
adopts the slice does the work once and **beats A everywhere**, 1.16x to 1.70x,
at one allocation against six.

**D proposes the copying column**, for the reasons in the ownership section
below. The adopting column is kept because it is the measurement a later ADR
would act on, not because D claims it.

**So ownership is D's central question — and D answers it by declining it.** The `*Slice`
contract already guarantees `d.KeySlice()` is a private copy, so adopting it is
safe. What the constructor cannot know is whether the slice it received came from
a `*Slice` method or from the caller's own live data, where adopting would
silently alias. Three ways out:

- **Constructors copy.** Safe, obvious, and gives up the *adopting* win — which
  the accounting below shows costs little on narrow elements and a great deal on
  wide ones.
- **Constructors adopt, and it is documented.** `NewVector(d.KeySlice()...)` is
  optimal, and a caller passing a slice they intend to keep must write
  `NewVector(slices.Clone(mine)...)`. Fast, and a genuine footgun: Go's convention
  is that a callee does *not* take ownership, so this violates an expectation
  rather than an explicit rule.
- **Two spellings**, one copying and one adopting. Recreates exactly the
  `X`/`XSeq` twinning that problem 2 exists to remove.

**D takes the first.** `New*` constructors — and every bulk mutator — copy their
input and never retain it. The rest of this section records why, what it costs,
and what it leaves open.

**This is the same question A already answered.** A deleted `AsSlice` partly to
avoid "an ownership vocabulary and a rule about when a returned slice may be
kept". D faces that question from the other side, and — unlike A, which could simply
delete the accessor — cannot avoid *asking* it, because slices are D's entire
surface. It can still answer it in the negative, which is what it does.
Worth noting the symmetry: A's rejected `AsSlice` optimisation was worth 1.9x to
a consumer that retains what it collects, and D-adopting measures 1.70x on the
comparable case. These are two spellings of the same win.

#### Ownership: what Go actually does

Ownership transfer has a settled shape in the standard library. D ends up
going the other way, but the evidence is what makes that a decision rather than
an oversight — and it is what a later ADR would build on. All of the following
was read out of this toolchain's own source rather than recalled.

**The default is explicit non-retention.** `io` states it four times, and it is
the expectation every Go programmer brings to a `[]T` parameter:

```
// Implementations must not retain p.
        -- io/io.go:85, 98, 229, 248
```

**Ownership transfer is nevertheless real precedent.** Six public occurrences
outside `runtime`, `cmd` and `internal`:

| function | location | wording |
|---|---|---|
| `bytes.NewBuffer(buf)` | `bytes/buffer.go:482` | "takes ownership of buf, and the caller should not use buf after this call" |
| `go/doc.New(pkg, …)` | `go/doc/doc.go:118` | "takes ownership of the AST pkg and may edit or overwrite it" |
| `go/doc.NewFromFiles` | `go/doc/doc.go:254` | "takes ownership of the AST files and may edit them" |
| `go/types.NewInterface` | `go/types/interface.go:35` | "takes ownership of the provided methods and may modify their types" |
| `go/types.NewInterfaceType` | `go/types/interface.go:48` | as above |
| `zip.Writer.CreateHeader(fh)` | `archive/zip/writer.go:267` | "takes ownership of fh and may mutate its fields. The caller must not modify fh after calling…" |

Three things follow, and they answer the naming question directly.

**1. None of them encode it in the name.** `NewBuffer`, `New`, `NewInterface`,
`CreateHeader` are all maximally plain. The only name-level marker for aliasing
anywhere in the standard library is `Unsafe`, confined to package `unsafe` and
generated trie code. `math/big` documents aliasing accessors the same way —
`Bits`/`SetBits`, "the result and abs share the same underlying array"
(`math/big/int.go:105, 117`) — with plain names.

**2. Go names the copy, not the alias.** `slices.Clone`, `bytes.Clone`,
`maps.Clone` and `strings.Clone` all exist; there is no `Alias` or `NoCopy`
counterpart in the public API. **This is the load-bearing point.** The vocabulary
for opting *out* of a transfer is already idiomatic — `NewVector(slices.Clone(mine)...)`
needs no vocabulary this package has to invent. Note this cuts both ways: it is
also why a *copying* default costs callers nothing to express, since a caller who
wants to hand over a throwaway simply has nothing to write.

**3. The doc wording is near-templated** — two clauses, declaring the transfer
and then stating the caller's concrete obligation:

```go
// NewVectorOwning creates a Vector using vs as its storage. It takes
// ownership of vs, and the caller should not use vs after this call.
// To keep using it, pass slices.Clone(mine).
```

That template is recorded for the ADR that adds such a constructor. **What D
itself ships is the opposite**, and needs one clause rather than two:

```go
// NewVector creates a Vector containing vs. The Vector copies vs and does
// not retain it; the caller remains free to modify the slice afterwards.
func NewVector[T any](vs ...T) *Vector[T]
```

**Read against D's actual decision, this evidence points the other way — and
that is worth being explicit about.** Six of six precedents keep the name plain
*because ownership transfer is that function's only mode*: there is one
`bytes.NewBuffer`, and it always adopts. D has chosen the opposite default, so
`New*` will already mean "copies" across the whole package. A later adopting
constructor therefore **cannot** be plain-named without making two `New*`
functions behave differently — the exact thing this decision exists to prevent.

So the precedent's real lesson for D is narrower than it first appears: **name
the mode that is not the default.** The doc template still applies verbatim to
whatever that function ends up called, and `slices.Clone` remains the caller's
escape hatch — but the naming conclusion inverts once the default does.

**One disanalogy, recorded so it is not discovered later.** In all six
precedents the argument is a purpose-built thing whose reason for existing is to
be handed over — a `Buffer`'s backing store, an AST, a `FileHeader`, a method
list. D's constructors take elements of the most common type in Go, where the
non-retention expectation is strongest and a spread argument is likeliest to be
live caller data. The mechanic is identical; the blast radius is not.
`bytes.NewBuffer` is a known footgun for exactly this reason, and it is one
function where D would be every constructor. The `...` at the call site narrows
this — a literal call transfers nothing — but it does not close it.

**D adopts nothing, for this iteration.** Every `New*` constructor copies its
input, matching the `io` default that a callee does not retain what it is
handed. The precedent above is not wasted — it is what a *later* ADR would cite
if the optimisation is wanted — but D as proposed does not spend it.

**Why the uniform non-owning rule wins here**, beyond the obvious virtue that
every constructor named `New*` then does the same thing:

- **Two hazards stop existing rather than being managed.** The capacity leak
  above — `src[:2]` arriving as len 2, cap 1024 — needs no `slices.Clip`, because
  nothing is retained to leak through. And `NewSortedSet(mine...)` can no longer
  reorder the caller's slice, because it sorts its own copy.
- **The per-container asymmetry disappears.** Adoption is a property of the
  backing store: it is worth the measured win for `Vector`, `SortedSet` and
  `SortedDict`; it is worth **nothing at all** for `HashSet`, `HashDict` and
  `Map`, which must insert element by element regardless and have no slice to
  adopt. A uniform owning rule would have asked callers for vigilance that half
  the containers could not repay. A uniform non-owning rule costs those three
  containers nothing.
- **The door stays genuinely open.** Adding a function is not a breaking change,
  so an adopting constructor can arrive later under its own name, against a
  measured call site rather than in anticipation — which is the same standard
  this ADR applied when it removed A's `AsSlice`.

**What it costs**, stated against the numbers already gathered. The shipped
figure for D is the *copying* column, so D against A becomes:

| | A | D, as proposed |
|---|---|---|
| map keys → vector | 8.097 µs, 6 allocs | 8.766 µs, 2 allocs — **8% slower** |
| slice → vector | 3.305 µs, 6 allocs | 3.835 µs, 2 allocs — **16% slower** |
| → map-backed set | 14.99 µs, 11 allocs | 14.17 µs, 7 allocs — **5% faster** |
| 1 KiB values ×256 | 36.10 µs, 256 KiB | 64.02 µs, 512 KiB — **77% slower** |

**D's performance case against A therefore mostly evaporates, and its
simplicity case does not.** On narrow elements the two are within noise of each
other in either direction; D still uses a third of the allocations. The one real
loss is **wide elements, where copying twice costs 77% and doubles peak
memory** — and that is exactly the call site a future adopting constructor would
be introduced to fix, with a measurement already waiting for it.

**Shape of the deferred optimisation**, so the later ADR starts from something:
an adopting constructor should take **`[]T`, not `...T`**. Adoption is
meaningless for a literal call, and the plain slice parameter makes the handover
explicit at the call site instead of hiding it behind a spread. Naming is that
ADR's problem; the six precedents above say it will be tempted to leave the name
plain, and this package should not, because `New*` will already mean "copies".

**What D costs, beyond ownership:**

- **Plain iteration is taxed.** Walking keys via `KeySlice()` costs 7.255 µs and
  18 KiB against 5.725 µs and **zero allocations** for `range d.Keys()`. So D
  **must keep the `iter.Seq` methods**; the slices are an addition, not a
  replacement. A dict ends with six shape methods rather than three, and D is
  simpler in *types* while being larger in *per-container surface*.
- **Peak memory doubles on any copying path**, which is worst exactly where it
  is least affordable: wide values, where D-copying holds 512 KiB to A's 256 KiB.
- **Reverse iteration is deferred.** `slices.Reverse` on an owned copy covers
  the common case, and revisiting proposal A's `Backward`/`RangeKeys`/`RangeAll`
  machinery is left to a later ADR should D be adopted — the one case it does not
  cover is reading a large container backwards without materialising it.
- **Laziness is gone.** `NewVector(KeysOf(huge))` under A streams; under D the
  whole source is materialised first, whatever the destination does with it. A
  consumer that stops early — a `Take(10)` over a million-element source — pays
  for all million.
- **Unbounded sources cannot be expressed at all.** A `Collector` over an
  infinite `iter.Seq` is legal under A and impossible under D.

#### What D settles beyond bulk construction

**Views gain the `*Slice` methods.** Without them a container cannot be built
from a view, and a view is the read-only boundary callers are meant to pass
around. Adding them is safe by the same contract that makes the whole proposal
work — a `*Slice` result is a copy, so it hands out nothing the view protects —
and ADR `0013` established that **adding a method to a sealed interface is not a
breaking change**, since nothing outside the package can implement one.

```go
type SetView[NT any] interface {
	Len() int
	Keys() iter.Seq[NT]
	KeySlice() []NT
	Has(NT) bool
	sealedView()
}

type DictView[NK, NV any] interface {
	Len() int
	Keys() iter.Seq[NK]
	Values() iter.Seq[NV]
	All() iter.Seq2[NK, NV]
	KeySlice() []NK
	ValueSlice() []NV
	AllSlice() []KeyValue[NK, NV]
	Get(NK) (NV, bool)
	sealedView()
}

type IndexedView[NT any] interface {
	Len() int
	Values() iter.Seq[NT]
	All() iter.Seq2[int, NT]
	ValueSlice() []NT
	AllSlice() []KeyValue[int, NT]
	At(int) NT
	sealedView()
}
```

The cost is real and should be stated: every one of the nine-plus unexported view
structs implements three more methods, and a view that carries a viewer must
apply the conversion per element while materialising. `NewVector(view.KeySlice()...)`
then works, which is the point.

**`Elems` and `Elems2` are removed**, as under proposal A and for a stronger
reason: D's constructors take `...T`, so nothing needs a source *interface* at
all. Their remaining job was carrying a length, and a slice carries its own.
`MutableSet` and `MutableDict` declare the read methods directly instead of
embedding them:

```go
type MutableSet[T any] interface {
	Len() int
	Keys() iter.Seq[T]
	KeySlice() []T
	Has(T) bool
	Add(T)
	Delete(T)
}
```

**`Keys`/`Values`/`All` keep the meanings problem 1 and problem 4 settled.**
`All` always yields pairs, `Keys` and `Values` each yield one half, a set is
key-only so `KeySlice` is its one materialiser, and a `Vector` is keyed by
position. The `*Slice` names mirror the iterator names exactly, so there is one
vocabulary rather than two.

**`Range` keeps returning an `iter.Seq`, and there is no `RangeSlice`.** A
sub-range is the one place materialising is plainly wrong: the caller asked for a
bounded window *because* the container is large, and handing back a copy of the
window defeats the request. A caller who wants the window as a slice writes
`slices.Collect(v.Range(lo, hi))` and has said so. This leaves ADR `0013`'s
deferred "should `Range` return a view?" exactly where it was, rather than
answering it the way proposal A did.

**Reverse iteration is deferred to a later ADR**, per the note above:
`slices.Reverse` on an owned copy covers the common case, and the machinery A
proposed can be revisited if the streaming-backwards case turns out to matter.

**The single-element mutators and the renames are inherited from A**, none of
which depended on the `Collector`:

- `Append(e T)`, `Set(k, v)`, `Add(v T)`, `Has`, `Get` and `Delete(k)` stay as
  single-element operations. A collector for one element measured 49x the direct
  call; a one-element variadic is cheaper than that but still not free, and the
  bulk form is right there.
- **`HashSet.Add` and `SortedSet.Add` stop being variadic**, leaving `AddAll` as
  the bulk spelling. After this **no method in the package is variadic except the
  bulk ones**, which is the same end state A reached by a different route.
- **`Remove` becomes `Delete`** on the sets. A set's element is its key, and the
  operation that removes a key is `Delete` everywhere else — including on `Map`,
  which already spells it that way.
- **`Map` gains `SetAll` and `DeleteAll` and still gets no `NewMap`**, for the
  reasons A gives: a composite literal and `make` already construct a `Map`,
  which is the point of the type.

**The `*Seq` twins are deleted, and `slices.Collect` is the bridge.**
`AppendAllSeq(seq)` becomes `AppendAll(slices.Collect(seq)...)`. This is the one
place D forces an allocation that a streaming API would not, and it is the
honest price of the slice being the currency.

**Duplicate resolution follows ADR `0004`: later wins.** Within a single call the
last occurrence of a repeated key is the one that survives, matching bulk
insertion as it already behaves. The trap A recorded applies unchanged and is
worth repeating because this repository has hit it once already:
**`slices.CompactFunc` keeps the *first* of each run**, so a sorted-container
implementation that sorts-and-compacts gets last-wins backwards, and the test
must use **distinguishable values** to catch it.

**Zero arguments still touch the receiver.** `v.AppendAll()`,
`d.DeleteAll()` and `NewVector[int]()` are all legal, and per ADR `0002`'s
eager-dereference rule the methods must dereference the receiver on a path that
always executes — a rule violated three times in this repository's history,
every time via a loop that could run zero times. `NewVector[int]()` needs the
explicit type argument, exactly as today.

**One reversal to record.** A rejected `AppendMany(es ...T)`, answering ADR
`0015`'s follow-up in the negative, on the grounds that the slice optimisation
belonged on `Collector` where it would serve every consumer. D has no
`Collector`, and `AppendAll(vs ...T)` *is* `AppendMany`. **D therefore answers
ADR `0015`'s follow-up in the positive**, and the 1.47x that measurement found is
kept rather than deferred.

**Where D lands.** It is the only proposal that gets C's simplicity without C's
sacrifice: one named type, no free functions, no sealing question, no
nil-collector question, no drain-before-delete hazard, and the size exact by
construction rather than plumbed. Its cost is a second set of methods per
container, doubled peak memory on every bulk build, and no streaming at all.

**It is also the strongest rival A has**, though on simplicity rather than on
speed. B was measured and did not clear its own bar. C gave up 2.7x-3.7x. D as
proposed is **within 16% of A on narrow elements in either direction, 77% slower
on wide ones, and uses a third of the allocations** — while replacing eight named
types and eight free functions with one struct and none. The performance case
it could have made is deferred, deliberately, to keep every `New*` meaning one
thing.
