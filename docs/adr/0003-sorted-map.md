# 3. SortedMap[K, V]: an ordered map backed by a sorted slice

- **Status:** Accepted. No `SortedMap` code exists yet, so this constrains the
  implementation rather than describing it.
- **Date:** 2026-09-06
- **Evidence:** `experiments/sortedbacking/` (`RESULTS.md`) for the backing
  decision; the call-site comparison below for whether the container is
  justified at all. Stdlib baselines compile and agree with one another.

## Context

Go has no ordered map. Two idioms exist today, and a fair comparison needs both
— the weaker one alone would make this container look like an easy win.

**`map[K]V` sorted on demand** is the obvious one and it is bad: every ordered
operation is O(n log n) because it re-sorts the key set. A floor lookup sorts
every key to answer one question.

**A sorted slice with binary search** is the real competitor, and it is strong.
It gives ordered iteration for free, range scans as a sub-slice, and O(log n)
lookup. `experiments/sortedbacking/` shows it beating a mature B-tree on lookup,
on iteration by ~8x, and on range scans by ~13x at scale.

So the question is not "is a sorted structure useful" but **what does the
container add over roughly twenty-five lines of sorted slice.**

## Call sites

Tasks use a rate table: key = minimum quantity for a tier, value = unit price.

### The scaffolding the baseline needs first

Before any task can be written, the sorted-slice baseline requires this — and
requires it again for every key/value type pair:

```go
type Table []Tier

func (t Table) find(k int) (int, bool) {
	return slices.BinarySearchFunc(t, k, func(e Tier, k int) int { return e.MinQty - k })
}

func (t *Table) Set(k int, v string) {
	if i, found := t.find(k); found {
		(*t)[i].Price = v
	} else {
		*t = slices.Insert(*t, i, Tier{k, v})
	}
}
```

### Task A — all tiers in key order

Baseline: `return t`. Already ordered. **The container cannot win this**, and
the sketch below is no better. Recorded so ordered iteration is not later cited
as motivation.

### Task B — tiers with lo <= key < hi

Today:

```go
func rangeSlice(t Table, lo, hi int) []Tier {
	i, _ := t.find(lo)
	j, _ := t.find(hi)
	return t[i:j]
}
```

Proposed (sketch — does not compile):

```go
for k, v := range tiers.Range(lo, hi) { ... }
```

Close on brevity. The difference is that the baseline **returns a sub-slice
aliasing its internal array**, so a caller can mutate the table through it.

### Task C — which tier applies to a quantity (floor)

Today:

```go
func floorSlice(t Table, qty int) (Tier, bool) {
	i, found := t.find(qty)
	if !found {
		i--
	}
	if i < 0 {
		return Tier{}, false
	}
	return t[i], true
}
```

Proposed (sketch):

```go
k, v, ok := tiers.Floor(qty)
```

**This is where the container earns its place.** Not on line count — on the
`if !found { i-- }` adjustment and the `i < 0` guard, which are exactly the
off-by-one that hand-rolled ordered lookup gets wrong, rewritten per type.

### What the comparison actually shows

The case for `SortedMap` is **correctness and reuse, not brevity**. Individual
call sites shrink modestly; what disappears is per-type scaffolding whose
binary-search boundary conditions are easy to get subtly wrong. This is a
different justification from ADR `0002`, where `Set` won by naming algebra, and
it should be held to honestly: if the surface grows beyond operations that carry
such boundary conditions, that growth is not justified by this ADR.

## Decision

### 1. Backed by a sorted slice

`experiments/sortedbacking/` measured the alternative directly:

- The slice loses to a B-tree **only** on random-order insertion, and only above
  roughly a hundred entries — the crossover falls between n=64 and n=512.
- Insertion *order* dominates insertion *count*: in ascending order the slice is
  a plain append and beats the tree ~2.3x at every size measured, flat to 65 536.
- The slice wins every read — lookup ~1.4x, ordered iteration ~8x, range scans
  ~13x at scale.

A B-tree buys one workload and loses the rest, for roughly four times the code.

### 2. Keys are `cmp.Ordered`, compared with `cmp.Compare`

Chosen specifically to preserve ADR `0002`'s usable zero value. A
comparator-function design cannot have one — the zero value has a nil
comparator — so it would require amending `0002` rather than taking an exception
from it.

The cost is real: no struct keys, no descending order, no custom collation. A
separate `SortedMapFunc` type is the only way to add those, since Go cannot
express an optional comparator in one type. Deliberately not now.

### 3. Surface

`Get`, `Set`, `Delete`, `Len`, `All() iter.Seq2[K, V]`, plus the ordered
operations: `Range(lo, hi) iter.Seq2[K, V]`, `Floor`, `Ceil`, `Min`, `Max`.

**No positional `At(i)`.** It is natural on a slice and would be cheap, but it
is the one operation that would drag ADR `0001`'s view-versus-copy decision into
scope, and nothing here needs it yet.

`Range` returns `iter.Seq2`, **not** a sub-slice. Returning the sub-slice would
be faster and is what the baseline does, but it aliases the internal array and
hands callers a mutation path into the map — precisely the ADR `0001` situation.
The iterator costs O(log n) to create and O(k) to consume, same as the baseline.

### 4. ADR 0002's shape rules apply unchanged

Uniform pointer receivers, a usable zero value, a `noCopy` field declared first,
and no nil-receiver or nil-argument special cases. In particular the eager
dereference rule from `0002`: **every method must dereference its receiver on a
path that always executes**, which for a sorted map means the variadic and
early-return paths need the same audit `Set`'s `Remove` and `Difference` needed.

## Consequences

- **Random-order insertion into a large map is the documented weak spot.** Above
  a few hundred entries each insert memmoves half the backing array. This must
  be stated on the type, with the measurement cited, so callers with that
  workload know to look elsewhere rather than discover it in production.
- **Bulk loading should be done in key order** where the caller controls it,
  which turns every insert into an append.
- `cmp.Ordered` keys exclude struct keys and custom orderings entirely. Expect
  this to be the first thing anyone asks for.
- No `At(i)`, so ADR `0001` still does not bite. It applies the moment positional
  access or a sub-slice accessor is added.
- Serialization is unresolved here exactly as it is for `Set` — see the
  library-wide follow-up in ADR `0002`.

## Follow-ups

- `callsites_test.go` gains the sorted-map baselines **before** any `SortedMap`
  code, per the call-site convention.
- `SortedMapFunc` for arbitrary key types and orderings, if wanted.
- A `Bulk`/`FromSorted` constructor that skips per-insert binary search when the
  caller already has ordered data.
