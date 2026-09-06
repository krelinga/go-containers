# 9. HashDict[K, V]: a struct-backed hash dict, and Map's demotion

- **Status:** Accepted. Not yet implemented, so this constrains the change
  rather than describing it.
- **Date:** 2026-09-06
- **Evidence:** `experiments/hashdict/` (`RESULTS.md`)
- **Relates to:** ADR `0002` (shape rules), `0006` (sized constructors), `0007`
  (`Map`, whose exceptions this ADR narrows), `0008` (naming).

## Context

`Map[K, V] map[K]V` is the only container here that does not follow ADR `0002`'s
shape rules, and `0007` justified each exception on the grounds that a thin
naming of a builtin should behave like the builtin. That reasoning holds for
what `Map` *is*. It does not follow that `Map` should be the type most callers
reach for.

Four differences ADR `0007` recorded, each of which surprises someone expecting
the rest of this package:

- **Copies alias silently.** No `noCopy`, so `go vet` says nothing.
- **Interface satisfaction is asymmetric.** `f(myMap)` but `f(&mySortedDict)`,
  because `Map` has value receivers and every other container has pointer ones.
- **The zero value panics on write**, where every other container's is usable.
- **Nil semantics differ**: reads on a nil `Map` succeed rather than panicking.

`0007` called the asymmetry "inherent to Go, not a defect to be fixed later".
That is true only while `Map` is the sole hash dict. A struct-backed sibling
removes it for callers who want the package's ordinary semantics.

The grid ADR `0008` drew has an acknowledged hole where `Map` sits. This fills it.

## Call sites

Today, a caller who wants a hash dict with the semantics the rest of the package
has cannot get one:

```go
var m containers.Map[string, int]
m.Set("a", 1)                    // panics: assignment to entry in nil map

d := m                           // aliases m; go vet is silent
prune(m, keep)                   // no & -- unlike every other container
```

Proposed:

```go
var d containers.HashDict[string, int]
d.Set("a", 1)                    // works, like every other zero value here

e := d                           // go vet: assignment copies lock value
prune(&d, keep)                  // & -- like every other container
```

`Map` keeps working unchanged for what it is good at, which is interop.

## Decision

### 1. Add `HashDict[K, V]`, a struct following ADR 0002

```go
type HashDict[K comparable, V any] struct {
	_ noCopy
	m map[K]V
}
```

The name follows ADR `0008`'s scheme mechanically: implementations are
`<Ordering><Concept>`.

Uniform pointer receivers, usable zero value with lazy initialisation, `noCopy`
declared first, uniform nil-pointer panics, and the eager-dereference rule.
Structurally the same as `HashSet`, which wraps a map in exactly this way.

`experiments/hashdict/` measured the cost at **3–11% per operation** — 0.39 ns on
`Get`, 0.53 ns on `Set`, 0.12 ns on `Delete`. The largest share is the nil check
that provides the usable zero value, so the fee buys the feature.

### 2. Surface mirrors `SortedDict` minus the ordered operations

`Get`, `Set`, `Delete`, `Len`, `All`, `Clone`, `SetAll`, `SetAllSeq`, plus
`NewHashDict`, `CollectHashDict` and `CollectHashDictSeq`.

No `Floor`, `Ceil`, `Min`, `Max` or `Range`: a hash dict has no order to expose.

`*HashDict[K, V]` satisfies `Elems2`, `Dict` and `MutableDict`, and does so
through a pointer like every other container.

### 3. The sized constructor is justified far better here than in ADR 0006

`CollectHashDict(src Elems2[K, V])` presizes with `make(map[K]V, src.Len())`.

`experiments/hashdict/` measured presizing at **2.4x to 4.1x in time and about
half the allocation volume**, at every size from 16 to 65 536 entries.

That is a different conclusion from `experiments/sizedcollect/`, which found a
size hint worth only 0–10% in wall time for the sorted containers — because an
O(n log n) sort followed and dominated. A hash dict has no sort, so the hint is
the whole difference. **Assuming ADR `0006`'s result carried over would have been
wrong**, which is why it was measured again rather than inherited.

**`SetAll` inserts in place and does not presize.** Go exposes no growth hint for
an existing map, so `SetAll` presizes only when the receiver is still at its zero
value.

A struct-backed dict *could* presize by rebuilding — allocate a new map at
`len(existing)+len(added)`, copy across, insert, replace — which a defined map
type cannot do, since callers hold the same map. `experiments/hashdict/` measured
it and **rejects it**: rebuild wins only around k ≥ 4n, and below that in-place
insertion wins by up to orders of magnitude, because rebuild pays its O(n) copy
however few entries are added.

Two things make that penalty worse than it looks. `maps.Clone` is **3–4x faster**
than `make` plus a copy, because it duplicates the hash table structure instead
of re-hashing every key — and it sizes to its source, so there is no way to clone
into a presized map. Choosing to presize forces the slower copy. And the regime
where rebuild does win is one where a caller would construct a new dict instead,
which `CollectHashDict` already presizes. Adopting it would also put a size-ratio
threshold inside `SetAll`, the hidden-constant shape ADR `0004` rejected.

So unlike `SortedDict.SetAll`, this carries no algorithmic advantage over calling
`Set` repeatedly. It exists for uniformity across the `MutableDict`
implementations, and its doc comment should say so rather than implying a saving
it does not deliver.

### 4. `Map` is demoted to an adapter

`Map` keeps every method and every property. What changes is what the package
tells people to reach for.

**Use `HashDict` by default.** Use `Map` when you specifically need what only it
can do:

- wrapping an existing `map[K]V` with no copy, as a free conversion
- passing the result where a `map[K]V` is expected
- builtin syntax — `m[k]`, `len(m)`, `delete(m, k)`, `range`
- `encoding/json`, which works for `Map` with string or integer keys and marshals
  every struct-backed container here to `{}`

That is an interop role, and a real one. It is not the role of a default.

## Consequences

- **Two hash dicts is a choice callers must make**, where before there was none.
  Decision 4 is the guidance, and it belongs in doc comments and CLAUDE.md, not
  only here. This is the main cost of the ADR.
- **ADR `0007`'s asymmetry consequence is narrowed, not repealed.** `Map` still
  satisfies interfaces as a value while everything else needs a pointer. What
  changes is that a caller who does not want that now has an alternative.
- **A fourth container marshals to `{}` with a nil error.** The library-wide
  serialization question gets more urgent, and `Map` becomes the only dict that
  round-trips — which is now an argument for keeping it rather than an accident.
- **`HashSet` and `HashDict` are structurally near-identical**, both wrapping a
  map in a struct. Bugs found in one should be checked against the other.
- The grid ADR `0008` drew is now complete, with `Map` sitting outside it as an
  adapter rather than filling a cell.

## Rejected alternatives

- **Fixing `Map` in place.** Impossible: a defined map type can carry neither a
  `noCopy` field nor a lazily-initialising zero value, because it has no struct
  to hold one and no pointer receiver to initialise through. Those two properties
  are the substance of the complaint.
- **Replacing `Map` with `HashDict`.** Rejected because `Map`'s free conversion,
  builtin syntax and working JSON have no substitute; deleting it would remove
  the only interop path this package has.
- **Omitting `SetAll`/`SetAllSeq`**, since neither can presize an existing dict.
  Rejected because `MutableDict` implementations that differ in surface make
  generic code harder to write for a saving of two short methods, but the doc
  comment must be honest that the benefit is uniformity rather than speed.

## Follow-ups

- The library-wide serialization decision, now covering four struct-backed
  containers and one type that already works.
- Whether `HashSet` should gain `AddAll`/`CollectHashSet` with the same presizing
  argument. `experiments/hashdict/` measured the map presizing benefit, and
  `HashSet` is a map underneath, so the finding likely transfers — but it has not
  been measured for that type.
