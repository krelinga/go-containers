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

## What this ADR does not do

It does not decide. Three things should be settled before it becomes a decision,
and two of them are already open elsewhere:

1. **Whether `All` should mean `Seq2`**, matching the stdlib. Everything in
   problem 1 follows from that answer — and the corrected finding means the
   answer also decides whether callers with wide values can walk keys cheaply,
   which is a performance question rather than a naming one.
2. **Whether `Elems` stays an interface.** ADR `0006` and `0008` assume it does.
3. **Whether `Range` returns a view** (ADR `0013`'s follow-up) and whether the
   ordered contract tier lands (ADR `0008`'s). Problem 3's shape depends on both.

## Follow-ups absorbed into this ADR

These were recorded elsewhere and belong here now:

- ADR `0015`'s bulk-mutator consistency follow-up — problem 2.
- ADR `0016`'s "the iteration contracts want their own ADR" — this document.
- ADR `0013`'s view-iteration allocation erratum: iterating a view allocates
  three times per call, because `All` returns a closure through a dynamic call.
  It is a cost of the contract's *shape* and belongs in whatever replaces it.
