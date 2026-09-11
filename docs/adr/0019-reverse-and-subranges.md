# 19. Reverse iteration and sub-ranges

- **Status:** **Proposed.** Three solutions are specified and measured; none is
  chosen yet.
- **Date:** 2026-09-11
- **Evidence:** `experiments/reverse/` (`RESULTS.md`).
- **Relates to:** ADR `0013` (which deferred "should `Range` return a view?"),
  `0015` (which left `Vector` without a `Range`), `0017` (which deferred reverse
  iteration and deferred `Range` again), `0018` (whose concrete view structs are
  the shape any answer here has to fit).

## The problem

**The library cannot read anything backwards, and its sub-ranges are
half-built.** Both have been deferred three times, and they are deferred
together because they are the same question asked twice.

Today:

```go
m.Range(lo, hi)      // SortedMap: a forward iter.Seq2 over a key window
m.RangeKeys(lo, hi)  // the key-only form
s.RangeKeys(lo, hi)  // SortedSet
// Vector: nothing -- ADR 0015 left it out
// reverse: nothing, anywhere
```

A caller wanting the last few entries of a sorted map has no spelling. A caller
wanting a window of a vector has no spelling. And ADR `0017` measured what the
workaround costs: reversing a forward `iter.Seq` from outside is **3.4x and
twelve allocations**, because you must materialise the whole thing first.

### They are one problem, not two

A sub-range is a **window**; reverse is a **direction**. Both describe a walk
over part of a container in some order, and a caller who wants "the last three
entries below 100" wants both at once. Solving them separately produces either
two mechanisms that do not compose, or a method for every combination.

### The constraint: efficient, or not offered

**Nothing here may be surfaced if the implementation has to materialise.** A
`Backward()` that clones the range and walks the copy is exactly what must not
ship — it would make the API look like it supports something it merely
simulates, and the cost is linear in the range, so it grows precisely where the
feature is most wanted.

Measured (`experiments/reverse`):

| | | allocs |
|---|---|---|
| full reverse, in place | 1.071–1.131 µs | 0–2 |
| full reverse, **materialised** | **2.176 µs** | 4, **8.2 KiB** |
| 64-element window reversed, in place | 115.5–123.0 ns | 2–3 |
| 64-element window reversed, **materialised** | **205.8 ns** | 4, **600 B** |

**1.9x and 8 KiB.** That is the shape being ruled out, not a shape being
compared.

### Which containers qualify

Efficiency decides the surface, and it draws a clean line:

| container | backing | reverse | sub-range |
|---|---|---|---|
| `SortedSet` | sorted slice | O(1) setup, walk in place | key-bounded, two binary searches |
| `SortedMap` | sorted slice | same | same |
| `Vector` | slice | same | **position-bounded — see open question** |
| `MapSet` | map | **meaningless** — no order | meaningless |
| `Map` | map | **meaningless** | meaningless |

So this ADR is about the slice-backed containers only, and the map-backed ones
get nothing. That asymmetry is the honest one: a hash container has no order to
reverse, and pretending otherwise is what the constraint above forbids.

## Solution A: a `Backward` twin for every read

No new types. Every read method gains a reversed counterpart returning a bare
`iter.Seq`.

```go
func (s SortedSet[T]) Keys() iter.Seq[T]
func (s SortedSet[T]) KeysBackward() iter.Seq[T]
func (s SortedSet[T]) RangeKeys(lo, hi T) iter.Seq[T]
func (s SortedSet[T]) RangeKeysBackward(lo, hi T) iter.Seq[T]
```

**Caller code:**

```go
// the last entries below 100, highest first
for k := range m.RangeKeysBackward(0, 100) {
	fmt.Println(k)
}

// a whole container, backwards
for k, v := range m.AllBackward() {
	fmt.Println(k, v)
}
```

**What it costs in surface.** A `SortedMap` needs ten: `All`, `Keys`, `Values`,
`Range`, `RangeKeys` and a `Backward` for each. A `Vector` needs six. And the
combinations are not closed — a future "range by position, backwards, values
only" is another method rather than a composition.

**What it costs at runtime:** 1.131 µs and **0 allocations** on a full reverse
walk, which is the best number in the experiment. But that zero is fragile: it
comes from the closure inlining into the range statement, and **once a window
has to be computed first the inlining is lost** — the sub-range forms cost 3
allocations, more than solution C's 2.

## Solution B: `Range` returns a view, and views gain `Backward`

One concept covers both. A sub-range *is* a view — of a window rather than a
whole container — and direction is a method on it. This is the shape ADR `0013`
deferred and `0017` deferred again.

```go
func (m SortedMap[K, V]) Range(lo, hi K) SortedMapView[K, V]
func (v SortedMapView[K, NV]) Backward() SortedMapView[K, NV]
```

**Caller code:**

```go
window := m.Range(0, 100)
for k, v := range window.All() { … }
for k, v := range window.Backward().All() { … }

// and a window is a view, so everything a view does works on it
count := window.Len()
keys := window.KeySlice()
handOut(window)          // still read-only, still a view
```

**What it buys.** One new method per container and one per view, no new types,
and windows compose with everything ADR `0018` already gives views — including
the shape interfaces, so a window can be passed to anything taking `Keys[K]`.
It also finally answers ADR `0013`'s question in the affirmative.

**What it costs at runtime**, and this is the problem: a view holds an
interface, so a window costs **1 allocation to construct and 2 to reverse**,
and iterating one costs **5 allocations** against solution C's 2. Reversal has
to be expressed by wrapping the inner implementation, and the wrapper is the
second allocation.

## Solution C: `Range` returns a span

Solution B's ergonomics without its allocations. A span is a **struct** holding
a window and a direction — where B is only an interface, and so has nowhere to
put a direction except in a wrapper.

```go
type KeySpan[K any]      struct{ … }  // over a set, or a map's keys
type KeyValueSpan[K, V any] struct{ … }  // over a map's entries

func (m SortedMap[K, V]) Range(lo, hi K) KeyValueSpan[K, V]
func (sp KeyValueSpan[K, V]) Backward() KeyValueSpan[K, V]
func (sp KeyValueSpan[K, V]) All() iter.Seq2[K, V]
```

**A span may hold an interface internally, and still be free.** This ADR first
claimed the opposite — that a span is cheap *because* it holds a concrete slice
— and that was wrong in a way that mattered, because a **view** cannot hand out
a concrete slice: it holds an interface, and a converting view's underlying
elements are not even the type it presents. If spans required concreteness, a
view could not produce one without materialising, which the constraint forbids.

Measured, the freedom has two conditions and ADR `0018` satisfies both:

| | | allocs |
|---|---|---|
| boxing a 3-word impl into the span's interface | 22.27 ns | 1 |
| boxing a **pointer-shaped** impl — every container is one word | 10.42 ns | **0** |
| **copying the interface a view already holds** | **0.8407 ns** | **0** |
| copying it, then reversing | 6.601 ns | **0** |

So the real difference between B and C is **not interface versus concrete**. It
is **wrapper versus field**: a view has nowhere to record a direction, so it must
wrap its implementation and re-box; a span has a struct field. That is the whole
of B's extra cost, and it is why C works uniformly for spans produced by
containers and by views.

**Caller code — identical to B:**

```go
window := m.Range(0, 100)
for k, v := range window.All() { … }
for k, v := range window.Backward().All() { … }
```

**What it costs at runtime:** the best numbers in the experiment except for A's
inlined full walk.

| | full reverse | sub-range fwd | sub-range rev | construct | reverse |
|---|---|---|---|---|---|
| A | 1.131 µs, **0** | 132.6 ns, 3 | 123.0 ns, 3 | — | — |
| B | 1.119 µs, 5 | 148.4 ns, 4 | 151.3 ns, 5 | 23.44 ns, 1 | 35.20 ns, 2 |
| C | **1.071 µs, 2** | **118.0 ns, 2** | **115.5 ns, 2** | **10.14 ns, 0** | **10.35 ns, 0** |

**A span is free to construct and free to reverse** — zero allocations for both,
because there is no interface to wrap. That is the whole difference between B
and C, and it is worth 2-3 allocations on every windowed walk.

**What it costs in surface:** new exported types — but fewer than first
estimated. Because a span may hold an interface, one `KeySpan[K]` can window a
set, a map's keys, or a view over either; it need not be one type per container.
Spans still do not substitute for views (`KeyValueSpan` and `SortedMapView` are
unrelated types), but both satisfy the ADR `0018` shape interfaces, so generic
read code takes either.

### Solution C, specified

#### The span types

Two, matching the shapes ADR `0018` already uses. Both hold an implementation
interface plus bounds plus a direction **field** — that field is what makes
reversal free, and it is the whole difference from solution B.

```go
type KeySpan[K cmp.Ordered]             struct{ … }  // a window of keys
type KeyValueSpan[K cmp.Ordered, V any] struct{ … }  // a window of entries
```

**The names mirror the shape interfaces**, which is what leaves room for the
positional side: `KeySpan` is to `Keys[K]` as `KeyValueSpan` is to
`KeyValues[K, V]`, and a future window over a sequence would be
`PositionValueSpan[P, V]`, to `PositionValues[P, V]`. A name like `EntrySpan`
would have read as the natural one for *that* type too, and taken the slot.

| method | `KeySpan[K]` | `KeyValueSpan[K, V]` | direction-dependent? |
|---|---|---|---|
| `Len() int` | ✓ | ✓ | no — but O(log n), not O(1); see below |
| `Has(K) bool` | ✓ | ✓ | no |
| `Get(K) (V, bool)` | — | ✓ | no |
| `Keys() iter.Seq[K]` | ✓ | ✓ | **yes** |
| `Values() iter.Seq[V]` | — | ✓ | **yes** |
| `All() iter.Seq2[K, V]` | — | ✓ | **yes** |
| `KeySlice() []K` | ✓ | ✓ | **yes** |
| `ValueSlice() []V` | — | ✓ | **yes** |
| `AllSlice() []Entry[K, V]` | — | ✓ | **yes** |
| `Backward() Self` | ✓ | ✓ | — |
| `Range(lo, hi K) Self` | ✓ | ✓ | no |

Verified: `KeySpan[K]` satisfies `Keys[K]`, and `KeyValueSpan[K, V]` satisfies
`Keys[K]`, `Values[V]` **and** `KeyValues[K, V]`. So generic read code takes a
span, a container or a view indifferently — which is the payoff of ADR `0018`'s
shape interfaces, applied here for free.

#### `Len` is not free on a span, and that is inherent

A span holds **key** bounds, not resolved indices. ADR `0017` settled that and
the reasoning is unchanged: index bounds go *silently wrong* the moment the
container changes — inserting a key inside the window makes it miss an element,
inserting one before it slides the whole window — and they cost 14x more to
construct.

The consequence lands on `Len`. Measured over 1024 elements:

| | | |
|---|---|---|
| a container's `Len` | 0.2322 ns | O(1) |
| a **key-bounded** span's `Len` | 9.982 ns | **43x** — two binary searches |
| an index-bounded span's `Len` | 0.1935 ns | O(1), and stale-prone |
| constructing a key-bounded span | 0.4546 ns | |
| constructing an index-bounded span | 10.17 ns | |

**The cost does not disappear, it moves.** Key bounds are ~22x cheaper to
construct and pay at every operation; index bounds pay once and are wrong
afterwards. Correctness picks key bounds, so `Len` on a span is O(log n).

**But it is not a cost `Len` introduces.** A key-bounded span re-resolves its
window on *every* operation, so `Keys()` pays the same two searches that `Len()`
does. `Len` is no more expensive than iterating; it simply is not the O(1)
subtraction every container's `Len` is.

**So should spans have `Len` at all?** ADR `0017` said no — "a sub-range holds
its bounds as keys, not indices, and simply has no `Len`". **That conclusion is
orphaned.** It was in service of `CanLen`, the optional size-hint interface that
`0017`'s *proposal A* used to presize a destination; the accepted proposal D
deleted `CanLen` and the whole collector machinery with it. The reason for
omitting `Len` went with it, exactly as ADR `0014`'s decisive argument went with
`Elems2`.

Weighed afresh, `Len` stays, for two reasons and against one:

- **Without it a span satisfies nothing.** `Keys[K]` requires `Len`, so a span
  with no `Len` cannot be handed to generic read code — which was one of the
  main payoffs of this shape.
- **10 ns is small where `Len` is actually used.** Its usual job is presizing a
  destination before an O(n) walk, and the walk dwarfs it.
- **Against:** it is the one place in the library where a shape interface hides a
  complexity difference. Every container's `Len` is O(1) and a span's is not, and
  a caller holding a `Keys[K]` cannot tell which they have. That wants a doc line
  on the interface, not a guard.

#### What is deliberately absent, and why

**`MinKey`, `MaxKey`, `FloorKey`, `CeilKey` are not on spans.** These are the
methods whose meaning goes soft once a window can be reversed: on a backward
span, "the minimum" and "the first thing yielded" stop coinciding, and a reader
has no way to tell which one a method named `MinKey` means. Rather than split
the type in two — a forward span with lookups and a read-only backward one —
the ordered lookups simply stay on the **container**, which owns the whole
ordered structure and has no direction to confuse them with.

The loss is real and small: a caller wanting the floor within a window calls
`m.FloorKey(x)` and checks the result is in range. A span's own minimum and
maximum are its first and last elements, which iteration already gives.

**`Backward` is therefore the only direction-bearing operation**, and every
method above is either direction-free or an ordered read that honours it.

#### Sub-spans: yes

`Range` narrows a window, and it composes:

```go
recent := m.Range(0, 100).Range(50, 60)     // a window of a window
```

**Bounds are on keys, not positions, so they are independent of direction** — a
narrowing is well defined on a backward span and keeps the receiver's direction:

```go
m.Range(0, 100).Backward().Range(50, 60)    // 59, 58, … 50
m.Range(0, 100).Range(50, 60).Backward()    // the same window, same order
```

That is the reason `Range` survives on spans where `MinKey` does not: a key
bound means the same thing whichever way you walk.

#### Double reversal: allowed

`sp.Backward().Backward()` is forward again, costs nothing, and makes `Backward`
an involution — the easiest rule to hold in your head. ADR `0017` deliberately
made double reversal a **compile error**, but that was for a *source*, which had
no state to flip; a span has a field, and a type to forbid it would buy nothing.
A caller who writes it has a bug, and it is not worth a second type to catch.

#### `Backward` on every container and view that can afford it

The rule: **a container or view gets `Backward` exactly when it is slice-backed**,
because that is when reversal is a decrementing loop rather than a
materialisation. `Backward()` returns the whole contents as a reversed span, so
it is `Range` over the full bounds with the flag set.

| type | `Backward()` returns | why |
|---|---|---|
| `SortedSet[T]` | `KeySpan[T]` | sorted slice; walk it downwards |
| `SortedSetView[T]` | `KeySpan[T]` | holds a `SortedSet`; same walk |
| `SortedMap[K, V]` | `KeyValueSpan[K, V]` | sorted slice of entries |
| `SortedMapView[K, NV]` | `KeyValueSpan[K, NV]` | copies the interface it holds; conversion applies per element as it walks |
| `Vector[T]` | **open — see below** | slice-backed, but see the position question |
| `VectorView[NT]` | **open** | same, including the `ViewSlice` case |
| `MapSet[T]` | **nothing** | no order to reverse |
| `MapSetView[NT]` | **nothing** | — |
| `Map[K, V]` | **nothing** | — |
| `MapView[NK, NV]` | **nothing** | — |

**The map-backed containers get no `Backward` at all**, and that asymmetry is
the point rather than an oversight: a hash container has no order, so a
`Backward` on one could only mean "materialise, reverse, walk" — which is
exactly the shape this ADR forbids. The absence is the constraint being
honoured.

**A converting view costs nothing extra.** `SortedMapView.Backward()` copies the
interface the view already holds into the span — measured at **0.8407 ns and no
allocation** — and the viewer is applied per element during the walk, exactly as
it is for a forward walk. Direction does not interact with conversion.

#### The one thing still open: `Vector`

A `Vector` is slice-backed, so reversal is efficient and `Backward()` is
obviously affordable. A **sub-span** is the problem, and it is ADR `0017`'s
unchanged objection: a `Vector` window would be bounded by **position**, and
position stability is "an artifact of an incomplete surface" — nothing in
`Vector` shifts an index today, but a `Remove` would, silently invalidating
every outstanding span.

Two sub-questions, neither answered here:

- **Does `Vector` get `Backward()` without `Range`?** Reversing the whole thing
  needs no bounds, so it is affordable today with no stability assumption.
- **If a `Vector` span exists, do its positions renumber?** A window over
  `[3, 7)` could present positions `0..3` or `3..6`. Keeping the original
  numbering is the only choice that makes `At(p)` mean the same thing on the
  span and the parent, but it means `PositionSlice()` does not start at zero.

## What to decide

1. **Which shape.** C is fastest and composes; A is smallest and is fastest only
   on the un-windowed walk; B is C's ergonomics at a worse price, and the price
   is specifically that an interface has nowhere to record a direction.
2. **`Vector`**, in two parts: whether it gets `Backward()` (affordable today,
   no stability assumption) and whether it gets a position-bounded span (which
   inherits ADR `0017`'s objection intact). Spelled out in the section above.
3. **Whether `Len` belongs on a span**, given it is O(log n) where every
   container's is O(1), and that ADR `0017` said no for a reason that no longer
   applies. Keeping it is what lets a span satisfy the shape interfaces.
4. **Whether dropping the ordered lookups from spans is the right trade.** The
   alternative is two span types per shape — a forward one carrying `MinKey` and
   friends, and a read-only backward one — which the type system would enforce
   but which doubles the vocabulary.
