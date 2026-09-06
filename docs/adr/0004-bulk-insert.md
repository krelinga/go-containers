# 4. Bulk insertion into SortedMap

- **Status:** Accepted. No bulk-insert code exists yet, so this constrains the
  implementation rather than describing it.
- **Date:** 2026-09-06
- **Evidence:** `experiments/bulkinsert/` (`RESULTS.md`)
- **Extends:** ADR `0003`, which chose the sorted-slice backing whose insertion
  cost this addresses.

## Context

ADR `0003` accepted a sorted-slice backing knowing its one weakness: an
out-of-order insert memmoves half the backing array, so adding k entries by
repeated `Set` is O(kn). That ADR's mitigation was advice — "bulk-load sorted
data where you control the order" — which only helps callers whose data happens
to arrive sorted.

Sorting the additions once and merging two sorted runs is O(k log k + n + k)
instead. `experiments/bulkinsert/` measured the difference:

- Sort-then-merge overtakes repeated insertion at **k≈4–16**, and the crossover
  barely moves with n.
- Above it the gap widens without bound — **42x at n=100 000, k=1 024**.
- Below it the two are within ~1.4x either way.

So the win is real, it starts at a handful of entries, and it grows.

## Call sites

Today, adding entries from another source:

```go
for k, v := range other.All() {
	m.Set(k, v)          // O(n) memmove per entry
}
```

Proposed (sketch — does not compile):

```go
m.SetAll(other.All())    // one sort, one merge
```

The call site is barely shorter, and that is the point: **this ADR is justified
by cost, not ergonomics.** The loop above is perfectly readable and will stay
perfectly readable while running 42x slower. Nothing at that call site tells the
caller which one they wrote — the same class of hidden cost ADR `0001` found in
copying accessors.

## Decision

### 1. Two entry points over one merge routine

```go
func (m *SortedMap[K, V]) SetAll(seq iter.Seq2[K, V])
func CollectSortedMap[K cmp.Ordered, V any](seq iter.Seq2[K, V]) *SortedMap[K, V]
```

`SetAll` adds into an existing map. `CollectSortedMap` builds a fresh one, where
the merge degenerates to a copy, so it is strictly cheaper than
`NewSortedMap()` followed by `SetAll`. It also matches the `maps.Collect` /
`slices.Collect` idiom.

### 2. Input is `iter.Seq2[K, V]`

Not a slice of pairs, not parallel slices. ADR `0003` already made `iter.Seq2`
the interchange — `All` and `Range` return it — so `m.SetAll(other.All())`,
`m.SetAll(maps.All(plain))` and `CollectSortedMap(m.Range(lo, hi))` all compose
with no adapter and no exported pair type.

### 3. Last write wins among duplicate keys within one input

Repeated `Set` gives last-write-wins, and bulk insertion must not quietly differ
from the loop it replaces.

This is not free to implement: `slices.CompactFunc` keeps the **first** of each
equal run, so the obvious sort-then-compact gives first-write-wins. The
prototype in `experiments/bulkinsert/` has exactly that bug, left in place and
commented, because its agreement test could not catch it — every added value in
the fixture was identical. **The implementation must sort stably and then keep
the last of each run**, and must be tested with distinguishable values.

### 4. No pre-sorted fast path

A `SetSorted` taking the caller's word that input is ascending was rejected. If
the promise is violated the map is silently corrupted: binary search over an
unsorted slice returns wrong answers rather than failing, so `Get`, `Floor` and
`Range` go quietly wrong and the damage surfaces far from its cause.

The measurement says the saving does not justify that. Sorting already-sorted
input is ~12–22x a bare copy — **not free, as an earlier draft of this ADR
claimed** — but it is only ~2.7% of the total when merging into a large map.

The exception is construction: with nothing to merge against, that sort is ~90%
of the work. **If a fast path is ever added, `CollectSortedMap` from pre-sorted
input is the case that justifies it, and it should be validated rather than
trusted.**

### 5. No automatic small-input fallback; document the guidance instead

`SetAll` always sorts and merges. Below the crossover that costs up to ~1.4x
versus repeated `Set`, which is small in absolute terms and only reaches callers
bulk-adding one or two entries.

A hidden threshold would buy that back at the price of two performance models in
one method and a constant that drifts with hardware. The doc comment says to
prefer `Set` for one or two entries and cites the measurement.

### 6. ADR 0002's shape rules apply

Uniform pointer receivers and no nil-receiver special cases. In particular the
eager-dereference rule: `SetAll` must touch the receiver on a path that always
executes, since an empty `seq` would otherwise skip it — the same trap that
`Set.Remove` and `Set.Difference` fell into.

## Consequences

- **Two ways to add many entries**, differing only in speed. The loop stays
  correct and stays slow; nothing warns a caller who writes it.
- `CollectSortedMap` is a free function because Go methods cannot introduce type
  parameters — the same constraint ADR `0002` hit with set algebra.
- **The duplicate-key rule needs a test with distinguishable values**, or it will
  pass while wrong, as the prototype's did.
- `SetAll` collects `seq` into a slice before sorting, so peak memory is n+k
  entries plus the collected input. Not a concern at the sizes measured; worth
  knowing for very large inputs.
- The O(n) insert weakness from ADR `0003` remains for genuinely one-at-a-time
  workloads. This narrows when it bites; it does not remove it.

## Follow-ups

- `callsites_test.go` gains the loop-based baseline **before** `SetAll` lands,
  per the call-site convention.
- A validated pre-sorted fast path for `CollectSortedMap`, if construction from
  ordered data proves common.
