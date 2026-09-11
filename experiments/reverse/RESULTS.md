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

## Durable / perishable

**Durable.** Materialising to reverse costs ~1.9x and memory linear in the
range; every in-place shape beats it. A concrete window value is free to
construct and free to reverse. An interface-wrapped one is not, and the wrapper
needed to express reversal costs a second allocation. Bare-iterator methods
avoid allocation only while the closure inlines, which a computed window
defeats.

**Perishable.** Every absolute number, and the exact allocation counts, which
track the compiler's inlining decisions.
