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
type EntrySpan[K, V any] struct{ … }  // over a map's entries

func (m SortedMap[K, V]) Range(lo, hi K) EntrySpan[K, V]
func (sp EntrySpan[K, V]) Backward() EntrySpan[K, V]
func (sp EntrySpan[K, V]) All() iter.Seq2[K, V]
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
Spans still do not substitute for views (`EntrySpan` and `SortedMapView` are
unrelated types), but both satisfy the ADR `0018` shape interfaces, so generic
read code takes either.

## What to decide

1. **Which shape.** C is fastest and composes; A is smallest and is fastest only
   on the un-windowed walk; B is C's ergonomics at a worse price, and the price
   is specifically that an interface has nowhere to record a direction.
2. **Whether `Vector` gets a sub-range at all.** ADR `0017` left it out because
   index stability is "an artifact of an incomplete surface" — nothing in
   `Vector` shifts an index today, but a `Remove` would, silently. That argument
   is unchanged. A position-bounded span would be correct only as long as that
   stays true.
3. **Whether a span can be reversed twice.** `sp.Backward().Backward()` is
   forward again and costs nothing to allow; ADR `0017`'s `Backward` deliberately
   made double-reversal a compile error. The reasoning there was about a
   *source*, and may not transfer.
