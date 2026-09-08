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

- a **dict** is key/value → `Elems2[K, V]`
- a **sequence** is position/value → `Elems2[int, T]`
- a **set** has keys and no values → `Elems[T]` is the honest shape

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

Against: it discards `Elems` as an interface, which ADRs `0006` and `0008` are
built on — containers would no longer *satisfy* anything, they would *produce*
something. That is a large conceptual change, and it removes the ability to write
a function generic over "any container" without naming a producing method.

### 1D. Keep both `All`s and accept the collision

The status quo, made explicit: `Elems` and `Elems2` stay exclusive, `AllIndexed`
stays, `ListView` stays unbuildable, and the disagreement with the stdlib stays.
Recorded so that doing nothing is a choice rather than a default.

## Directions for problem 2

### 2A. Fill the matrix

Give `HashSet` its `CollectHashSet`, `AddAll` and `AddAllSeq`; add
`Vector.AppendMany(es ...T)`; decide `Map` deliberately rather than by omission.
Smallest change, leaves the shape duplication in place.

### 2B. Collapse `X` and `XSeq` into one

Follows from 1C: if a length is carried by the argument rather than by the
argument's type, one function covers both. Eight functions and eight methods
become four and four.

### 2C. Free functions instead of methods

```go
// sketch
func InsertAll[T any](dst MutableSet[T], src Elems[T])
```

One implementation instead of one per container, and it cannot be forgotten on a
new container. Against: it reads worse than a method, and ADR `0002` records that
anything generic over containers has to be a free function anyway — so this may
be where it ends up regardless.

## Directions for problem 3

### 3A. `Backward` methods on the ordered containers

```go
// sketch
func (s *SortedSet[T]) Backward() iter.Seq[T]
func (d *SortedDict[K, V]) Backward() iter.Seq2[K, V]
func (v *Vector[T]) Backward() iter.Seq2[int, V]
```

Mirrors `slices.Backward`, free at every backing this library has, and it is the
only place the capability can come from. The open part is `Range`: a reversed
range needs either `RangeBackward(lo, hi)` or something better, and adding a
second method per ordered operation does not scale.

### 3B. Reversal as a view operation

```go
// sketch
func Reverse[NT any](v IndexedView[NT]) IndexedView[NT]
```

Free for an `IndexedView`, whose `At(i)` makes reversal a subtraction. Does not
work for `SetView` or `DictView`, which have no positional access — so it solves
reversal for sequences only, which may be enough.

### 3C. `Range` returns a view, and views reverse

ADR `0013` already carries "should `Range` return a view rather than an
iterator?" as a follow-up, and ADR `0008` carries a deferred ordered contract
tier. Combined with 3B, one answer covers all three: `Range` yields a view,
views can be reversed, and the ordered tier declares both.

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

## Directions for problem 5

### 5A. Add the constructors

Eighteen or more functions, each trivial, each needing a doc comment and a test,
and each a thing to forget when a container is added. Recorded so that "just add
them" is a weighed option rather than the default — it is not obviously wrong,
only large.

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
