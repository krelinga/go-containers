# 8. Naming conventions and contract layering

- **Status:** Accepted. Not yet applied, so this constrains the change rather
  than describing it.
- **Date:** 2026-09-06
- **Relates to:** every prior ADR, whose names this changes. Those ADRs are left
  as written; see the mapping below.

## Context

Names here accreted one ADR at a time and no longer describe a system:

- **`Set` is both a type and a method.** `containers.Set[T]` is a container,
  while `Map.Set(k, v)` and `SortedMap.Set(k, v)` are the write operations. Legal
  Go, but ambiguous in prose, in godoc, and at a glance. Note that decision 4
  reuses `Set` for the read-only set contract, so this ADR *relocates* the
  collision rather than removing it — see the consequences.
- **`SetLike` was always a placeholder**, flagged as such in ADR `0002` and in
  CLAUDE.md ever since.
- **`MutableMap` and `SetLike` are the same kind of thing** — the mutation
  contract for a concept — and share no naming pattern.
- **`Map` and `SortedMap` are not the same kind of thing.** One wraps the
  builtin; the other is a sorted-slice container. The shared suffix implies a
  symmetry that does not exist.

**This ADR is not only about names.** Decision 4 restructures the contract
layering and adds `Remove` to the set mutation contract, which is design and
should be reviewed as design. The renames are the larger share of the diff but
the smaller share of the decision.

No behaviour of any existing container changes. There is no call-site
comparison, because that convention exists to test whether a *capability* earns
its place, and the one capability added here — a read-only contract that can ask
`Has` and `Get` — is justified in decision 4 rather than by a benchmark.

## Decision

### 1. The scheme

**Implementations** are `<Ordering><Concept>`, with one exception:

| | Set concept | Dict concept |
|---|---|---|
| hash-backed | `HashSet[T]` | `Map[K, V]` (see 3) |
| sorted | `SortedSet[T]` | `SortedDict[K, V]` |

**The two sides follow different conventions, deliberately.** The dict side is
unqualified-default plus qualified-variant — `Map` and `SortedDict` — which is
Go's usual shape (`sync.Map`, not `sync.HashMap`). The set side qualifies both.

The reason is not aesthetic: **decision 4 takes the name `Set` for the read-only
contract, so the struct has to move**, and once it must be named something,
`HashSet` beside `SortedSet` is the best available. An earlier draft of this ADR
justified `HashSet` by grid symmetry, which was post-hoc; the name is forced, and
the grid is not in fact symmetric.

**Contracts come in three layers**, each adding to the one beneath it:

| layer | set side | dict side | adds |
|---|---|---|---|
| universal | `Elems[T]` | `Elems2[K, V]` | `Len`, `All` |
| read-only, concept-specific | `Set[T]` | `Dict[K, V]` | `Has` / `Get` |
| mutation | `MutableSet[T]` | `MutableDict[K, V]` | `Add`+`Remove` / `Set`+`Delete` |

```go
type Set[T comparable] interface {
	Elems[T]
	Has(T) bool
}
type MutableSet[T comparable] interface {
	Set[T]
	Add(...T)
	Remove(...T)
}
```

`Dict` and `MutableDict` mirror it with `Get`, and `Set`/`Delete`.

**`Dict` carries only `Get`**, not `Floor`, `Ceil`, `Min`, `Max` or `Range`,
because `Map` cannot provide them — the same exclusion ADR `0007` made for
`MutableDict`. Generic code over `Dict` therefore cannot do ordered reads.

**The contracts vary on one axis where the implementations vary on two.**
Implementations differ by concept *and* by ordering; contracts differ only by
concept. So there is no contract for "an ordered container", and no way to write
generic code against `Floor` or `Range` — arguably the genericity this library
would most benefit from. That gap is deliberate for now, not overlooked: a
`SortedDict`-and-friends contract wants a second implementation to design
against, and there is only one. It is recorded as a follow-up so the scheme is
not mistaken for complete.

**Constructors** keep their existing patterns against the new type names:
`New<Type>`, `Collect<Type>`, `Collect<Type>Seq`.

"Dict" rather than "Map" for the concept because `Map` is taken by the builtin
wrapper, and a contract cannot be called `MutableMap` when `Map` is one specific
implementation of it.

That choice then forces `SortedMap` to become `SortedDict`, and it is worth
being honest about the strength of that: **the rename is for vocabulary
consistency, not because anyone was confused.** Go ships `sync.Map` beside the
`map` builtin without difficulty. What does not work is a package where the
concept word and one implementation's name are the same, and picking "Dict" for
the concept is what buys `MutableDict` and `Dict` their legibility.

### 2. The renames

| was | is now |
|---|---|
| `Set[T]` | `HashSet[T]` |
| `SetLike[T]` | `MutableSet[T]` |
| `SortedMap[K, V]` | `SortedDict[K, V]` |
| `MutableMap[K, V]` | `MutableDict[K, V]` |
| `NewSet` | `NewHashSet` |
| `NewSortedMap` | `NewSortedDict` |
| `CollectSortedMap` | `CollectSortedDict` |
| `CollectSortedMapSeq` | `CollectSortedDictSeq` |

Newly introduced (decision 4):

| name | what |
|---|---|
| `Set[T]` | read-only set contract — **the name is reused, not retired** |
| `Dict[K, V]` | read-only dict contract |

Unchanged: `Map`, `SortedSet`, `Elems`, `Elems2`, `NewSortedSet`,
`CollectSortedSet`, `CollectSortedSetSeq`.

Note that `Set` appears on both tables: the struct vacates the name and the
read-only interface takes it. **The danger there is silence, not breakage.**

`var s containers.Set[int]` compiles before and after. Before, it is a usable
empty set; after, it is a nil interface that panics on the first method call.
Writes fail loudly, since `Add` is not on the read-only contract, but reads
compile and then panic at runtime:

```
compiled: var s Set[int]
Has on the zero value: runtime error: invalid memory address or nil pointer dereference
```

So the rename must be done in one pass, and completeness cannot be established
by "it compiles". Grep for the old names as well.

Files follow: `set.go` becomes `hashset.go`, `sortedmap.go` becomes
`sorteddict.go`, and their tests likewise.

### 3. `Map` is a deliberate exception

`Map[K, V] map[K]V` keeps its name rather than becoming `HashDict`, breaking the
grid's symmetry on purpose.

ADR `0007` established that **where a container is a thin naming of a builtin,
it matches the builtin rather than this library's conventions** — that is why it
has value receivers, no `noCopy`, and a zero value that panics on write. Naming
follows the same principle: the type's entire identity is "the builtin map, with
methods", and `HashDict` would obscure exactly what a reader most needs to know.

The cost is real and worth stating: `Map` and `SortedDict` are the two
`MutableDict` implementations and do not look like a pair. That asymmetry is the
price of the name telling the truth.

### 4. Three layers, because `Elems` alone is too thin to read with

`Elems` and `Elems2` keep their names and stay concept-neutral: `Elems[T]` is
satisfied by both set types today and would be satisfied by a `Slice[T]`
tomorrow without that type being a set. They are the floor.

**`Elems2` is not made redundant by `Dict`**, though it looks it — everything
satisfying `Dict` satisfies `Elems2`. It survives because it is the narrower
contract, and Go's convention is to accept the narrowest interface that does the
job. `CollectSortedDict` needs `Len` and `All` and has no use for `Get`, so it
takes `Elems2`; widening it to `Dict` would reject a future pair-source that
cannot answer `Get`. Stated because the redundancy is the obvious reading and
deleting `Elems2` would be a silent narrowing of what those constructors accept.

But a floor is not a useful reading contract. **Reading a set means asking
whether something is in it**, and `Elems[T]` cannot express `Has`. Generic code
that only counts and iterates is a small fraction of what callers actually want,
and forcing it to take `MutableSet` to get `Has` hands out write access it does
not need.

So the read-only layer is concept-specific — `Set[T]` adds `Has`, `Dict[K, V]`
adds `Get` — and mutation builds on that rather than on `Elems` directly.

The revised principle: **iteration is universal; interrogation and mutation are
both concept-specific.** The earlier framing — that reads are universal and only
writes are concept-specific — was wrong, and produced a `MutableSet` that mixed
`Has` in with `Add` because there was nowhere else to put it.

`MutableSet` also gains `Remove`, which `SetLike` never had, so that it mirrors
`MutableDict`'s `Delete`.

### 5. Prior ADRs are left as written

ADRs record decisions at the time they were made. Rewriting `0002` to say
`HashSet` would assert a choice nobody made then, and `0007`'s reasoning about
`Map` naming the builtin stops parsing if the surrounding names shift under it.

The table in section 2 is the mapping. A reader of an older ADR resolves names
through it.

## Consequences

- **Every exported set and sorted-dict name changes at once.** Cheap now,
  because nothing outside this repository imports the package; this is the last
  moment it is cheap.
- **The `Set` name still collides, now with an interface instead of a struct.**
  `containers.Set[T]` is the read-only set contract and `Map.Set(k, v)` is a dict
  write. This ADR therefore does not fix that ambiguity; it moves it. The struct
  is renamed because decision 4 needs the name, not to avoid the collision —
  recorded so the collision is not later cited as something this ADR delivers.
- **`Example*` function names encode type names** and must be renamed with them:
  `ExampleSet_Difference` becomes `ExampleHashSet_Difference`,
  `ExampleSortedMap_Floor` becomes `ExampleSortedDict_Floor`. `go vet` catches
  these, since an `ExampleT_M` naming an unknown method is an error.
- **The grid has a hole where `Map` sits**, permanently and by decision. Anyone
  reading the scheme will notice; section 3 is the answer.
- **`Dict` is not Go vocabulary** — the standard library says "map". This trades
  familiarity for the ability to use `Map` precisely.
- CLAUDE.md's Conventions and Status both name types directly and must follow.

## Rejected alternatives

- **`HashDict` for `Map`.** Completes the grid, and was the symmetric choice.
  Rejected because it hides that the type *is* a map, which is its whole point,
  and because ADR `0007` already committed to the builtin's conventions over this
  library's for exactly this type.
- **Keeping `Set` and renaming only `SetLike`.** The smallest change that fixes
  the acknowledged placeholder. Rejected because it leaves `Set` colliding with
  the `Set` method and leaves `Map`/`SortedMap` implying a symmetry that is false.
- **`ReadOnlySet` / `ReadOnlyDict` for `Elems` / `Elems2`.** Consistent, but
  over-narrows contracts that are deliberately not concept-specific. Decision 4
  instead introduces `Set` and `Dict` *above* them, keeping both.
- **A different name for the read-only set contract** — `ReadSet`, `Membership`,
  `Container` — to avoid reusing `Set` alongside the `Set` method. Rejected
  because `Set`/`Dict` is the pairing that makes the three-layer scheme legible.
  The *contract* layers are symmetric even though the implementations are not,
  and qualifying one side of a symmetric pair would be worse than the ambiguity
  it avoids.
- **Two layers rather than three**, with `Has` and `Get` folded into the mutation
  contracts. That is the shape this ADR originally had. Rejected because it
  forces generic read-only code to accept a contract carrying `Add`, `Remove`,
  `Set` and `Delete` in order to ask one membership question.
- **Rewriting the prior ADRs.** Every ADR would then read correctly against the
  code, at the cost of falsifying what was decided and when.
- **Deferring until a first release.** The rename is cheapest before anything
  depends on it, and every ADR written in the meantime would use names already
  known to be wrong.

## Follow-ups

- **A contract for ordered containers** — `Floor`, `Ceil`, `Min`, `Max`, `Range`
  — closing the axis the scheme currently leaves open. Wants a second ordered
  implementation to design against.
- Whether a future `Slice[T] []T` takes the `Map` exception too, being another
  thin naming of a builtin.
