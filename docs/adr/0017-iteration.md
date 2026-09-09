# 17. The iteration contracts

- **Status:** Proposed — and deliberately **not a decision**. This ADR states
  three problems precisely, records the evidence, and sketches directions. It
  decides nothing. A second ADR, or a revision of this one, should choose.
- **Date:** 2026-09-08
- **Evidence:** `experiments/iteration/` (`RESULTS.md`), with context from
  `experiments/vectorcost/`, `sliceadapter/`, `linkedlist/` and `sizedcollect/`.
- **Relates to:** ADR `0006` (`Elems`/`Elems2`, and the reason they are two
  interfaces), `0008` (the contract layers, and the deferred ordered tier),
  `0004` (bulk insert), `0010` (`LinkedList`, whose `All` yields pairs),
  `0013` (views, and the deferred "should `Range` return a view"), `0015` and
  `0016` (`Vector` and slice views, where problem 1 keeps surfacing).

## Context

Iteration is the one thing every container here does, and three problems have
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
by letter; more may be added before this ADR decides.

### Proposal A: a `Collector` primitive

Every bulk constructor and bulk mutator takes a `Collector`. A `Collector`
abstracts away *where a sequence of entries comes from* — a container's keys, its
values, its pairs, a bare iterator, or a literal list — so the consumer never
learns which.

```go
type Collector[T any] interface {
	SizeHint() int
	AsSlice() []T
	AsSeq() iter.Seq[T]
	sealedCollector()
}

type Collector2[K, V any] interface {
	SizeHint() int
	AsSeq2() iter.Seq2[K, V]
	sealedCollector()
}
```

`SizeHint` returns 0 when the size is not known, so the consumer never has to ask
whether a length exists. `AsSlice` returns nil when the entries were not a slice
to begin with, so a consumer can take a memmove fast path when one is available
and fall back otherwise. `AsSeq`/`AsSeq2` always work.

**Sources.** A container advertises which shapes it can produce:

```go
type HoldsKeys[T any] interface   { Len() int; Keys() iter.Seq[T] }
type HoldsValues[T any] interface { Len() int; Values() iter.Seq[T] }

// A pair source must also produce each half natively.
type HoldsAll[K, V any] interface {
	HoldsKeys[K]
	HoldsValues[V]
	All() iter.Seq2[K, V]
}
```

The embedding is not just tidiness. It means **anything that can produce pairs
must offer a cheap key-only and value-only walk**, which is what stops
`KeysOf(dict)` falling back to deriving and paying 14.9x at wide values. A source
that genuinely has only pairs — a zip of two iterators, say — goes through
`AllFrom(seq)` and never claims to be a `HoldsAll`.

**A set is a `HoldsKeys`, not a `HoldsValues`**, per problem 4's taxonomy: a
set's element is its key, and a set has no value. So `KeysOf(mySet)` is the
spelling and `ValuesOf(mySet)` does not compile.

**Constructors.** Six, covering every source:

```go
func KeysOf[T any](h HoldsKeys[T]) Collector[T]
func ValuesOf[T any](h HoldsValues[T]) Collector[T]
func AllOf[K, V any](h HoldsAll[K, V]) Collector2[K, V]
func ItemsFrom[T any](seq iter.Seq[T]) Collector[T]
func AllFrom[K, V any](seq iter.Seq2[K, V]) Collector2[K, V]
func Items[T any](vs ...T) Collector[T]
```

**Consumers.** One per container, not one per source shape:

```go
func CollectVector[T any](c Collector[T]) *Vector[T]
func CollectHashSet[T comparable](c Collector[T]) *HashSet[T]
func CollectHashDict[K comparable, V any](c Collector2[K, V]) *HashDict[K, V]

func (v *Vector[T]) AppendAll(c Collector[T])
func (s *HashSet[T]) AddAll(c Collector[T])
func (d *HashDict[K, V]) SetAll(c Collector2[K, V])
```

**Caller code:**

```go
containers.CollectVector(containers.KeysOf(d))        // problem 5, solved
containers.CollectHashSet(containers.ValuesOf(d))
containers.CollectHashDict(containers.AllOf(v))       // a vector, keyed by index
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
discarded-value copy. 2B, since `SizeHint` carries the length and the `X`/`XSeq`
split collapses. 5B and 5C, of which the collector constructors are a
generalisation. 5A and 5D become unnecessary.

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
containers.CollectVector(containers.KeysOf(sd.Backward()))  // collect in reverse
containers.CollectHashSet(containers.ValuesOf(v.Backward()))
```

This only works because of the embedding. An earlier draft declared
`HoldsAll` without it, and the static return type then hid the concrete type's
other methods:

```
in call to KeysOf, type HoldsAll[string, int] of d.Backward()
does not match HoldsKeys[T] (cannot infer T)
```

Verified with the embedding in place: Go permits the repeated `Len()`, inference
reaches through it, and `KeysOf(d.Backward())` yields `[c b a]` while
`ValuesOf(d.Backward())` yields `[3 2 1]`. The reverse source is one word — a
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
containers.CollectVector(containers.KeysOf(sub.Backward()))
lastKey(sub)                                          // generic code takes it too
```

Verified: a sub-range of `[a b c d e]` over `[1, 4)` yields `[b c d]` forward and
`[d c b]` backward, `KeysOf(sub.Backward())` collects `[d c b]` with a
`SizeHint` of 3, and a generic function over `RangeAll` accepts a sub-range as
readily as a container.

Three consequences worth stating:

- **A sub-range must know its own length**, because `RangeAll` embeds `HoldsAll`.
  Over a sorted slice that is two binary searches, which today's `Range` — a bare
  `iter.Seq2` — does not have to do.
- **A sub-range cannot be sub-ranged.** `RangeAll` has no `Range` of its own, so
  `sd.Range(a, b).Range(c, d)` is a compile error. Deliberate, and the same shape
  as `Backward()` returning a source with no `Backward()`.
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

**What proposal A does not cover.** Reverse over a **sub-range** is still
uncovered: `Range(lo, hi)` returns an `iter.Seq2`, not a source, so it cannot be
reversed by this mechanism. Covering it means `Range` returning a source or a
view, which is ADR `0013`'s deferred follow-up in another guise.

And proposal A **assumes problem 4's answer rather than settling it**: `HoldsKeys`, `HoldsValues` and `HoldsAll` encode the
caller-key/position/value taxonomy directly, so adopting proposal A commits to
it. Proposal A therefore cannot land before problem 4 is decided, though the
sharpest instance — whether a set is a `HoldsKeys` or a `HoldsValues` — is now
settled in favour of `HoldsKeys`.

**Surface.** Six constructors, one `Collect` and one bulk method per container:
about twenty declarations covering every source-shape × target combination,
against eighteen functions under 5A covering a third of them.

**Rules settled while reviewing the proposal:**

- **`AsSlice` returns non-nil only for a collector built from a caller-owned
  slice** — `Items(vs...)`, or `ItemsFrom` over one. It never exposes a
  container's backing array, which would hand out the interior ADR `0001` exists
  to protect and `Vector` was built to own.
- **`KeysOf` requires a native `Keys()`; it does not fall back to deriving from
  `All()`.** A container that wants to be a key source provides the method. This
  makes proposal A **depend on direction 1A** rather than merely preferring it,
  and the dependency is deliberate: the fallback would silently cost 14.9x at
  wide values.
- **`Collector2` has no `AsSlice`**, because there is no natural contiguous form
  for pairs. Stated so the asymmetry does not read as an oversight.
- **Reusability follows `iter.Seq`, and is not specified.** A collector over a
  container can be walked repeatedly; one over a single-use iterator cannot, and
  nothing in the interface distinguishes them. This inherits ADR `0006`'s
  existing non-guarantee and makes it more visible, so it wants documenting on
  `Collector` itself.
- **Sealing costs callers nothing.** A caller's own type with `Len()` and
  `Values()` satisfies `HoldsValues[T]` and works with `ValuesOf` directly;
  sealing only prevents implementing `Collector`, which nothing needs to do.
- **No consumer-side helper is provided.** A `Collector` carries both an
  `AsSlice` and an `AsSeq`, so a consumer *may* branch on which is available —
  but nothing in the package does it for them. `AsSeq` always works, and that
  guarantee is what makes the absence of a helper acceptable: a consumer with no
  opinion writes one loop and takes the hit.

  The hit is small where it lands and large where it matters, which is why this
  is not a shortcut. Against a **map-backed** target the branch is worth ~15%,
  since a map insert at ~13 ns dominates the iterator's ~1 ns. Against a
  **slice-backed** target it is worth ~2x, because there the fast path is a
  memmove rather than a cheaper loop. So the branch gets hand-written in the two
  or three places it is worth 2x, and skipped everywhere it is worth 15%.

  A callback helper was measured at 4–8% over a hand-written branch — cheap
  enough to be viable, and rejected anyway, because it earns its keep only in the
  cases that will be hand-written regardless. A helper that materialises a slice
  was rejected outright: it is the best option when the collector already has a
  slice and the worst when it does not.
- **`Elems` and `Elems2` may not survive.** `HoldsValues[T]` is `Elems[T]` with
  `All` renamed, and `HoldsAll[K, V]` is `Elems2[K, V]`. ADR `0006` says their
  purpose *is* carrying a length, and that purpose moves to `SizeHint`. Whether
  they are renamed, kept as aliases, or removed is part of adopting this.

### Proposal A, as it stands

**One sentence.** Every bulk constructor and bulk mutator takes a `Collector`,
which abstracts away where a sequence of entries comes from; containers advertise
what shapes they can produce; and a container that reads backwards returns a
reversed source rather than an iterator.

**The surface**, complete:

```go
// Sources -- what a container advertises.
type HoldsKeys[T any] interface   { Len() int; Keys() iter.Seq[T] }
type HoldsValues[T any] interface { Len() int; Values() iter.Seq[T] }
type HoldsAll[K, V any] interface { HoldsKeys[K]; HoldsValues[V]; All() iter.Seq2[K, V] }

// Sources that also read backwards. Range returns one; a container is the widest.
type RangeKeys[T any] interface   { HoldsKeys[T]; Backward() HoldsKeys[T] }
type RangeAll[K, V any] interface { HoldsAll[K, V]; Backward() HoldsAll[K, V] }

// The primitive.
type Collector[T any] interface {
	SizeHint() int
	AsSlice() []T
	AsSeq() iter.Seq[T]
	sealedCollector()
}
type Collector2[K, V any] interface {
	SizeHint() int
	AsSeq2() iter.Seq2[K, V]
	sealedCollector()
}

// Six constructors.
func KeysOf[T any](h HoldsKeys[T]) Collector[T]
func ValuesOf[T any](h HoldsValues[T]) Collector[T]
func AllOf[K, V any](h HoldsAll[K, V]) Collector2[K, V]
func ItemsFrom[T any](seq iter.Seq[T]) Collector[T]
func AllFrom[K, V any](seq iter.Seq2[K, V]) Collector2[K, V]
func Items[T any](vs ...T) Collector[T]

// One Collect and one bulk method per container, plus Backward where ordered.
func CollectVector[T any](c Collector[T]) *Vector[T]
func (v *Vector[T]) AppendAll(c Collector[T])
func (v *Vector[T]) Backward() HoldsAll[int, T]
func (d *SortedDict[K, V]) Range(lo, hi K) RangeAll[K, V]
```

**Settled.**

| | |
|---|---|
| `HoldsAll` embeds `HoldsKeys` and `HoldsValues` | so pair sources must offer cheap half-walks, and `Backward` needs no interface of its own |
| a set is a `HoldsKeys` | its element is its key; `ValuesOf(set)` does not compile |
| `AsSlice` is non-nil only for caller-owned slices | never a container's backing array |
| `KeysOf` requires a native `Keys()` | no silent fallback to deriving, which would cost 14.9x at wide values |
| `Collector2` has no `AsSlice` | pairs have no contiguous form |
| no consumer-side helper | `AsSeq` always works; the branch is hand-written where it is worth ~2x and skipped where it is worth ~15% |
| `SizeHint() == 0` means unknown | a consumer never asks whether a length exists |
| reverse is a source, named `Backward` | matching `slices.Backward`; composes with every `Collector` constructor |
| a reverse source has no `Backward` | double reverse is a compile error |
| sealing costs callers nothing | a caller's type with `Len` and `Values` is a `HoldsValues` already |
| `RangeKeys` / `RangeAll` name reversibility | so it can appear in a signature; `Range` returns one, and a container is the widest one |
| a sub-range cannot be sub-ranged | `RangeAll` has no `Range`, as a reverse source has no `Backward` |
| there is no `RangeValues` | nothing in the taxonomy is values-only |
| **`Elems` and `Elems2` are removed**, not renamed or aliased | the `Holds*` family replaces them, `SizeHint` takes over the length job, and the library has no external users to break |

**Verified, not assumed.** Inference resolves `KeysOf(d)` and `ValuesOf(d)` on a
dict that has both, with no type arguments, and a mismatch is a compile error
naming the method. Inference also reaches *through* the embedding, which the
un-embedded form failed to do. `d.Backward()` boxes with zero allocations.
`KeysOf(d.Backward())` yields `[c b a]`.

**Which problems it closes.** Problem 1, by requiring native per-shape methods.
Problem 2, by collapsing `X`/`XSeq` into one argument type. Problem 3, for whole
containers. Problem 5, entirely — `CollectVector(KeysOf(d))` is the case that had
no spelling.

**What remains open: nothing.** The last item — reverse over a sub-range — is
closed by `Range` returning a `RangeAll` rather than an `iter.Seq2`, which also
answers ADR `0013`'s deferred "should `Range` return a view?" in the negative:
it returns a source, and a source has nothing to deny.

**Problem 4 is settled** and no longer gates this proposal — the taxonomy is
adopted, ordered containers keep values-only conversion as a recorded exception,
positions are never converted, and "key" keeps two documented scopes.

**The cost to callers**, stated plainly: every existing set and vector iteration
changes. `s.All()` becomes `s.Keys()` and `v.All()` becomes `v.Values()`. That is
the price of the whole proposal, and it is paid at every call site in every
program using this library.
