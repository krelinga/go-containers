# 6. Elems[T]: a sized read-only contract

- **Status:** Accepted. Not yet implemented, so this constrains the change
  rather than describing it.
- **Date:** 2026-09-06
- **Evidence:** `experiments/sizedcollect/` (`RESULTS.md`)
- **Relates to:** ADR `0004` (bulk insertion), `0005` (SortedSet), and the open
  `SetLike` naming question.

## Context

`iter.Seq` is a function and carries no length, so every constructor and bulk
method in this library drains one with a bare `append`. Building a `SortedSet`
from a `Set` therefore reallocates repeatedly even though the source knew its
size all along.

`experiments/sizedcollect/` measured it:

- Draining an unsized sequence reallocates up to 38 times and overshoots the
  final capacity by as much as 25%.
- Collecting alone costs **3.5–6.4x the time and up to 5x the allocation
  volume** of a length-known collect.
- **But end to end the time saving is 0–10%** above a few hundred elements,
  because every collect here is followed by an O(n log n) sort. At n=4 096 it is
  unmeasurable.
- **The allocation gap survives the sort intact.** A one-million-element build
  churns 41.7 MB against 8.4 MB.

**This is a GC-pressure fix, not a speed fix.** That framing matters: a 0–10%
time saving does not fund much API surface, so the mechanism must be cheap.

## Call sites

Today, converting between containers:

```go
sorted := containers.CollectSortedSet(set.All())   // set.Len() is right there, and lost
```

Proposed:

```go
sorted := containers.CollectSortedSet(set)         // preallocates
```

Shorter *and* faster, which is unusual — the size information already exists at
every one of these call sites and is being discarded on the way in.

## Decision

### 1. Two read-only interfaces

```go
type Elems[T any] interface {
	Len() int
	All() iter.Seq[T]
}

type Elems2[K, V any] interface {
	Len() int
	All() iter.Seq2[K, V]
}
```

`*Set[T]` and `*SortedSet[T]` already satisfy `Elems[T]`; `*SortedMap[K, V]`
already satisfies `Elems2[K, V]`. No container changes to adopt this.

Two interfaces rather than one because Go has no higher-kinded types: `iter.Seq`
and `iter.Seq2` cannot be unified, the same ceiling ADR `0002` hit with set
algebra.

On the name: `Seq`/`Seq2` was rejected because `containers.Seq2[K, V]` and
`iter.Seq2[K, V]` would appear in the same signature. `View` was rejected
because ADR `0001` defers how a read-only view is expressed, and taking the word
here would preempt that. `Sized` and `Finite` name the property precisely but
narrow the contract if it ever grows past `Len` and `All`. `Elems` says what the
interface yields and stays accurate either way.

### 2. The sized form takes the short name

| sized | bare iterator |
|---|---|
| `CollectSortedSet(src Elems[T])` | `CollectSortedSetSeq(seq iter.Seq[T])` |
| `CollectSortedMap(src Elems2[K, V])` | `CollectSortedMapSeq(seq iter.Seq2[K, V])` |
| `(*SortedSet).AddAll(src Elems[T])` | `AddAllSeq(seq iter.Seq[T])` |
| `(*SortedMap).SetAll(src Elems2[K, V])` | `SetAllSeq(seq iter.Seq2[K, V])` |

The container-to-container conversion is the case that motivated this, and it
should get preallocation without the caller thinking about it. Bare iterators
remain first-class — `maps.All`, `slices.Values` and this library's own `Range`
all return them — but they are the case that cannot be preallocated, so they
carry the longer name.

This changes the signature of `CollectSortedSet`, `CollectSortedMap`, `AddAll`
and `SetAll`. Nothing outside this repository depends on them yet.

### 3. `Len` is a capacity hint, never a correctness input

Implementations preallocate with `make([]T, 0, src.Len())` and then append
whatever `All` actually yields. A `Elems` whose `Len` disagrees with its
`All` produces a bad hint and nothing worse — no truncation, no corruption.

This matters because `Elems` is a public interface that outside types can
implement, so its contract must not be load-bearing for correctness.

### 4. `SetLike` is redefined in terms of `Elems`

```go
type SetLike[T comparable] interface {
	Elems[T]
	Add(...T)
	Has(T) bool
}
```

Identical method set to today, expressed as the read-only contract plus the
mutating operations. `*SortedSet[T]` already satisfies it, incidentally rather
than by design.

**The name remains unresolved**, exactly as before; this ADR does not settle it.

### 5. ADR 0002's shape rules still apply

A nil `Elems` argument panics when `Len` is called on it, consistent with
the nil-argument rule. The eager-dereference rule stands: `AddAll` must touch
its receiver on a path that always executes, and must now also touch `src`.

## Consequences

- **The saving is allocation volume, not wall-clock.** Doc comments should say
  so, or the sized forms will be cited for a speed benefit they do not deliver.
- **Two entry points per constructor**, one of which exists only because
  `iter.Seq` cannot report a length. If Go ever gains a sized-iterator
  convention, the `Seq` variants become redundant.
- Passing a container to itself — `s.AddAll(s)` — collects before merging, so it
  is well defined. It is also pointless, and worth a test rather than a guard.
- `Elems` is deliberately read-only, so accepting one commits us to nothing
  about mutation. It is the natural place to hang future generic algorithms,
  which ADR `0002` established must be free functions.

## Follow-ups

- Name the mutating set contract, still `SetLike`.
- Whether `Set` should gain `AddAll`/`AddAllSeq` for symmetry. Its backing is a
  map, so there is no reallocation story — the argument would be uniformity
  alone, which this ADR does not make.
