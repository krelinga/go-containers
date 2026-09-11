# Reverse iteration and sub-ranges: what the SHAPE costs

The walk itself is the same loop in every shape here; what differs is what the
caller is handed. See `bench.txt` for raw output and provenance.

## 1. The shape that must not ship

Materialising to reverse — clone the window, `slices.Reverse`, walk forward:

| | | allocs |
|---|---|---|
| full reverse, in place | 1.071–1.131 µs | 0–2 |
| full reverse, **materialised** | **2.176 µs** | 4, **8.2 KiB** |
| sub-range reverse, in place | 115.5–123.0 ns | 2–3 |
| sub-range reverse, **materialised** | **205.8 ns** | 4, **600 B** |

**1.9x and 8 KiB on a full walk, 1.8x on a 64-element window.** The cost is
linear in the range, so it grows exactly where reverse iteration is most worth
having. This is the baseline every other shape has to beat, and all three do.

## 2. The three shapes

| | full reverse | sub-range fwd | sub-range rev | construct only |
|---|---|---|---|---|
| **A** method pairs | 1.131 µs, **0 allocs** | 132.6 ns, 3 | 123.0 ns, 3 | — |
| **B** range returns a view | 1.119 µs, 5 | 148.4 ns, 4 | 151.3 ns, 5 | 23.44 ns, 1 alloc |
| **C** range returns a span | **1.071 µs, 2** | **118.0 ns, 2** | **115.5 ns, 2** | **10.14 ns, 0 allocs** |

**C wins on every axis measured**, and the construction column is why: a
concrete span holds its window directly, so building one — and reversing it —
allocates **nothing**, where the view form costs 1 allocation to construct and
2 to reverse. Reversal is a flag flip in both, but B has to wrap an interface to
express it.

**A is only free on the full walk.** Its 0 allocations there come from the
closure being inlined into the range statement; once a window has to be computed
first, that inlining is lost and A costs 3 allocations, more than C's 2.

## 3. What this says about composition

B and C read identically at the call site — `Range(lo, hi)` returns a value with
`Backward()` on it — so the choice between them is not ergonomic. It is that B
pays interface dispatch to be polymorphic over backings, and a *span* does not
need to be: it is always a window over a slice, whoever owns the slice.

A needs no new type, and costs a `Backward` twin on every read method: ten
methods on a sorted map, six on a vector.

## 4. Where a span's allocation actually comes from

The first cut of this harness concluded that a span is free *because it holds a
concrete slice rather than an interface*. That is wrong, and the correction
matters because a **view** cannot hand out a concrete slice: it holds an
interface, and a converting view's underlying elements are not even the type it
presents.

| constructing a span | | allocs |
|---|---|---|
| boxing a **3-word** impl (a slice header) into the interface | 22.27 ns | **1** |
| boxing a **pointer-shaped** impl (one word) | 10.42 ns | **0** |
| **copying an interface a view already holds** | **0.8407 ns** | **0** |
| copying it, then reversing | 6.601 ns | **0** |

**A span may hold an interface and still be free to construct.** Two conditions,
and ADR `0018`'s library satisfies both:

- **The boxed dynamic type must be pointer-shaped.** Every container under ADR
  `0018` is exactly one word, so boxing one costs nothing. The 1-allocation row
  above came from boxing a bare slice header, which is an artifact of how that
  constructor was written, not of the design.
- **Direction must be a FIELD, not a wrapper.** Reversing by setting a flag on a
  copied struct is free; reversing by wrapping the implementation in a
  `reverseWrapper` re-boxes and costs an allocation — 2 against 1 in the earlier
  table.

So the real distinction between the view-returning shape and the span shape is
**not** "interface versus concrete". It is **wrapper versus field**. A view
expresses reversal by wrapping, because a view is only an interface and has
nowhere to put a flag; a span has a struct to put it in.

That also removes a cost this ADR charged the span shape: it need **not** be one
type per container. A span holding an interface can window a set, a map's keys
or a view alike, so the three-types-per-shape estimate was too pessimistic.

## 5. `Len` on a span is O(log n), and the cost is inherent

A span must hold KEY bounds, not indices (ADR `0017`: index bounds are 14x
dearer to construct and silently wrong after a mutation). So its window is
re-resolved per operation:

| | | |
|---|---|---|
| container `Len` | 0.2322 ns | O(1) |
| key-bounded span `Len` | 9.982 ns | **43x** |
| index-bounded span `Len` | 0.1935 ns | O(1), stale-prone |
| construct, key-bounded | 0.4546 ns | |
| construct, index-bounded | 10.17 ns | |

The cost moves rather than vanishing: key bounds are ~22x cheaper to build and
pay per operation. And `Len` is not what introduces it — `Keys()` pays the same
two searches — so the choice is about whether a shape interface should hide the
difference, not about whether to compute it.

## Durable / perishable

**Durable.** Materialising to reverse costs ~1.9x and memory linear in the
range; every in-place shape beats it. A window value is free to construct and
free to reverse **whether or not it holds an interface**, provided the boxed
type is pointer-shaped (or already boxed) and direction is a field rather than a
wrapper. Expressing reversal by wrapping always costs an allocation. Bare-iterator methods
avoid allocation only while the closure inlines, which a computed window
defeats.

A key-bounded window is cheap to construct and O(log n) to measure; an
index-bounded one inverts both and is incorrect after a mutation.

**Perishable.** Every absolute number, and the exact allocation counts, which
track the compiler's inlining decisions.
