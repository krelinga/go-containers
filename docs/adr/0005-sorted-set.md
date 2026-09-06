# 5. SortedSet[T]: an ordered set over its own sorted slice

- **Status:** Accepted. No `SortedSet` code exists yet, so this constrains the
  implementation rather than describing it.
- **Date:** 2026-09-06
- **Evidence:** `experiments/sortedsetbacking/` (`RESULTS.md`) for the backing
  decision; the call-site comparison below, whose baselines compile and agree.
- **Relates to:** ADR `0002` (Set: algebra, shape rules), `0003` (SortedMap:
  ordered lookups, `cmp.Ordered`), `0004` (bulk insertion).

## Context

This library ships `Set[T]` (unordered, map-backed) and `SortedMap[K, V]`
(ordered, sorted-slice-backed). A sorted set is the missing corner.

The obvious implementation is `SortedMap[T, struct{}]`, and that was measured
rather than assumed.

## Call sites

Domain: deployed release version numbers.

### Task H — all versions in order

Baseline: `return v`. A maintained sorted slice is already ordered. **The
container cannot win this**, exactly as ordered iteration was a non-win in ADR
`0003` and dedup was in `0002`.

### Task I — versions with lo <= n < hi

```go
func versionsInRangeStdlib(v Versions, lo, hi int) []int {
	i, _ := slices.BinarySearch(v, lo)
	j, _ := slices.BinarySearch(v, hi)
	return v[i:j]
}
```

Close to what the container offers. Two differences, both found by writing it:
it returns a sub-slice **aliasing the caller's backing array**, and it **panics
on inverted bounds**, because it reslices between two searches without comparing
them.

### Task J — newest version at or before n

```go
func versionAtOrBeforeStdlib(v Versions, n int) (int, bool) {
	i, found := slices.BinarySearch(v, n)
	if !found {
		i--
	}
	if i < 0 {
		return 0, false
	}
	return v[i], true
}
```

versus `s.Floor(n)`. The same off-by-one that justified `SortedMap`, and the
same conclusion: **the win is correctness and reuse, not brevity.**

### The baseline a user of this library would actually write

`Set[int]` already exists here, so the natural thing is
`slices.Sorted(s.All())` and then binary search. It is correct and **O(n log n)
per query** — it re-sorts the entire set to answer one question. Recorded
because it is the version most likely to be written, and the clearest thing
`SortedSet` improves on.

## Decision

### 1. Its own `[]T`, not `SortedMap[T, struct{}]`

`experiments/sortedsetbacking/` measured the wrapper directly:

- **Layout costs 2x.** `sortedEntry` puts the value field last, and Go pads a
  struct whose final field is zero-sized, so `SortedMap[T, struct{}]` is twice
  the width of a bare `[]T` in both memory and insert time. Reordering the
  fields fixes this at no cost, and was done separately in `57ffdc9` because it
  benefits `SortedMap[K, struct{}]` whether or not this ADR is adopted.
- **Lookup costs ~3.4x, and cannot be fixed.** Not layout: reordered and current
  layouts are indistinguishable, and a bare `[]int` searched through a
  comparator closure reproduces the gap exactly. It is the per-comparison
  closure call. A `[]T` set calls `slices.BinarySearch` directly because `T` is
  already `cmp.Ordered`; anything storing entries cannot.
- **Delegation itself is free.** An `iter.Seq2` adapter discarding values costs
  ~1%, within noise.

So the wrapper is nearly free except on the one operation a set exists for.
Membership is the hot path; give up 3.4x there and the type is compromised at
its core.

The cost is duplication: `find`, insert, merge, range and the boundary logic all
appear twice. That is accepted deliberately, and bounded by decision 4.

### 2. `T` is `cmp.Ordered`

Consistent with ADR `0003` and for the same reason: it preserves ADR `0002`'s
usable zero value, which a comparator-function design cannot have.

### 3. Surface: Set's algebra, SortedMap's ordering, ADR 0004's bulk

- From `0002`: `Add(...T)`, `Remove(...T)`, `Has`, `Len`, `Clone`, `Union`,
  `Intersect`, `Difference`.
- From `0003`: `All() iter.Seq[T]`, `Range(lo, hi) iter.Seq[T]`, `Floor`, `Ceil`,
  `Min`, `Max`. No positional `At`, keeping ADR `0001` out of scope.
- From `0004`: `AddAll(seq iter.Seq[T])` and `CollectSortedSet(seq)`.

`Range` returns `iter.Seq[T]`, not a sub-slice, for the reason `0003` gave: a
sub-slice hands callers a mutation path into the backing array.

**Inverted bounds yield an empty range, not a panic** — matching
`SortedMap.Range`, and unlike the baseline, which panics.

### 4. Algebra merges rather than probes

Both operands are sorted, so `Union`, `Intersect` and `Difference` are a linear
merge — O(n+m) — not n probes of O(log m). This also means algebra and `AddAll`
share one merge routine, which is what keeps decision 1's duplication bounded.

The exception is badly asymmetric sizes: intersecting 10 elements with a million
favours probing, O(n log m). Not handled now; see follow-ups.

### 5. ADR 0002's shape rules apply

Uniform pointer receivers, usable zero value, `noCopy` declared **first**, no
nil-receiver or nil-argument special cases, and the eager-dereference rule —
every method touches its receiver on a path that always executes, which the
variadic mutators and `AddAll` must be audited for, since empty input would
otherwise skip it.

## Consequences

- **The sorted-slice logic now exists twice**, in `sortedmap.go` and
  `sortedset.go`. Bugs must be fixed in both. Decision 4 limits this to the
  merge and boundary helpers; if it grows further, extracting shared unexported
  generics is the escape hatch.
- Random-order insertion is O(n) per entry, inherited from ADR `0003`'s backing.
  `AddAll` is the answer, exactly as in `0004`.
- `Set[T]` and `SortedSet[T]` have overlapping purposes. `Set` should stay the
  default; `SortedSet` earns its place only when ordering is queried, not merely
  output once.

## Follow-ups

- Parallel `keys []K` / `vals []V` in `SortedMap` would recover the same 3.4x
  for its own `Get`, `Floor`, `Ceil` and `Range`. Wants its own experiment.
- Galloping search for `Intersect` on badly asymmetric operands.
