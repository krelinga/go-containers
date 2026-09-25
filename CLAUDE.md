# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status

`package containers` at the repo root is the library. Every container is a
**reference type** (ADR `0018`): a one-word value whose copies share the same contents.

| file | type | backing | its view |
|---|---|---|---|
| `mapset.go` | `MapSet[T comparable]` | map | `MapSetView[NT]` |
| `sortedset.go` | `SortedSet[T cmp.Ordered]` | sorted slice | `SortedSetView[T]` |
| `map.go` | `Map[K comparable, V any]` | a defined `map[K]V` | `MapView[NK, NV]` |
| `sortedmap.go` | `SortedMap[K cmp.Ordered, V any]` | sorted slice | `SortedMapView[K, NV]` |
| `vector.go` | `Vector[T any]` | slice, insertion-ordered (ADR `0015`) | `VectorView[NT]` |
| — | *(no `Slice` type)* | a plain `[]T` | `VectorView[NT]`, via `ViewSlice` |
| `entry.go` | `Entry[S, V]` | the pair type bulk operations carry | — |
| `contracts.go` | the shape interfaces below, sealed (ADR `0022`) | — | — |
| `views.go` | the view structs and their constructors | — | — |
| `viewers.go` | `KeyViewer`, `ValueViewer`, the `CanView<Container>` set | the conversions a view applies (ADR `0012`) | — |

**The shape interfaces** are named for what a type *exposes*, not for what kind of
container it is, so none of them collides with a container name:

```
    Keys[K]              Values[V]             Positions[P]
       |                  |      |                   |
       +-> KeyValues[K,V]      PositionValues[P,V] <-+
       |        |
SortedKeys[K]   +-> SortedKeyValues[K,V]
       |                |
MutableKeys[K]   MutableKeyValues[K,V]
```

They are **sealed** (ADR `0022`): each declares the unexported `callViewFirst`,
which **only a view implements**. So a container cannot be passed where reads are
promised — `publish(s)` is a compile error naming `callViewFirst`, and the fix is
`publish(s.View())`, which is free. Nothing taken out of one can be asserted back
to something writable.

Two things about the seal are easy to get wrong, and both are measured in
`experiments/sealing`. **Sealing only works because the container is excluded** —
a sealed interface a container also satisfies is exactly as leaky as an unsealed
one. And **one token serves the whole package**: an interface-typed value
satisfies another sealed interface only if it *declares* the same unexported
method, so a token per interface would stop the tiers composing.

The unexported `inner*` mirrors at the bottom of `contracts.go` are the unsealed
copies the plumbing needs, because a view is `struct{ impl <tier> }` and an
identity view passes its container straight in. **They are also where "a new
container exposes its reads whole" is now checked** — the public tiers cannot do
it any more.

`mutatesKeys`/`mutatesKeyValues` are the mutation contract, **unexported**. They
replaced exported `MutableKeys`/`MutableKeyValues`, and two properties are
load-bearing: unexported, so no caller can take one as a parameter; and they
**never embed a read tier**, because a container implementing `callViewFirst`
would reopen the hole completely.

`callsites_test.go` holds every stdlib-vs-container comparison. Alongside:
`docs/adr/` (design decisions) and `experiments/` (measurement harnesses, each
its own module). **Check an ADR's status before treating it as binding.** `0014`
is **Rejected and superseded by `0018`**; `0020` is **Abandoned** (its problem was
solved by `0021`, its solutions cost the zero-value rule for no measured gain);
`0010`, `0019`, `0021` and `0023` are **Proposed** and describe nothing that
exists in code. The rest are Accepted, `0022` most recently. `0017` is Accepted **as proposal D** while
containing three rejected proposals in full, and `0018` is Accepted **as option
(b)** and likewise keeps its rejected options.

Intent, per the module path `github.com/krelinga/go-containers`: a generic
(type-parameterized) container library.

## Commands

```sh
go build ./...
go vet ./...
go test ./...
go test -vet=all ./...                 # tests plus the FULL vet set
go test -run '^TestName$' ./...        # single test
go test -race ./...
gofmt -l .                             # list unformatted files; -w to rewrite

# Experiments are separate modules; the root ./... does not reach them.
cd experiments/<name> && ./run.sh      # regenerate that experiment's bench.txt
COUNT=20 BENCHTIME=1s ./run.sh         # more samples
```

## Conventions

### The three rules that have caused the most confusion

These are first because they are the ones that get re-derived wrongly.

- **Reads are total; writes panic. The zero value is the builtin's zero value.**
  `var s MapSet[int]` reads as empty — `Len()` is 0, `Has` is false, iterating
  yields nothing, `KeySlice()` is nil — and `s.Add(1)` panics, exactly as
  `m[k] = v` does on a nil map. **A bulk call that writes nothing does not
  panic** (`s.AddAll()` with no arguments is fine), and **deletes are no-ops**,
  because `delete(nilMap, k)` is. `IsZero()` answers the narrower "was this ever
  constructed", which is as rarely needed as `m == nil` is.
  **This replaced ADR `0002`'s eager-dereference rule**, which required every
  method to panic on an unconstructed receiver. Do not reintroduce it.
  `TestZeroValueMatchesTheBuiltin` asserts the whole rule for every container at
  once, and is the test to extend when a container is added.

- **An iterator captures the slice HEADER, not the contents — and map-backed
  containers capture nothing.** This is weaker than a snapshot and the difference
  is real:

  | | slice-backed (`SortedSet`, `SortedMap`, `Vector`) | map-backed (`MapSet`, `Map`) |
  |---|---|---|
  | later `Add`/`Delete` (length change) | **not** seen | **seen** |
  | later `Set(i, x)` in range | **seen** | n/a |
  | insert shifting within spare capacity | **seen** | n/a |

  What is actually guaranteed is narrow: **taking an iterator never panics and
  never observes a reallocation.** Modifying a container while iterating it is
  unsupported, which is what ranging a builtin map already gives you. Earlier
  ADRs say an iterator is "bound to the contents as of the call" — that
  overstates it in both directions; the doc comment on `SortedSet.Keys` has the
  accurate version.

- **Copies share; `Clone` is how you get a separate one.** `t := s` gives another
  handle on the same container. `noCopy`, the copylocks caveat and the layout
  test are all gone (ADR `0018`) because the bug class they policed no longer
  exists — but the opposite hazard does, and nothing checks it.

### Shape and naming

- **Package name is `containers`, not `go-containers`.**
- `go.mod` pins `go 1.26.7` at patch granularity.
- Single flat package at the repo root — add new container types as sibling
  files, not subpackages.
- **Names are `<Backing><Concept>`** (ADR `0018`, superseding `0008` section 3):
  `MapSet`/`Map` are map-backed, `SortedSet`/`SortedMap` are sorted-slice-backed.
  `Map` is the centre of the scheme rather than an exception to it.
- **A new container satisfies its shape interfaces whole**, and the compile-time
  assertions at the bottom of `contracts.go` are how you find out. Views satisfy
  the read tiers and must **never** satisfy a mutation tier.
- **Every container has a view, and a new one is not finished without it.** Build
  one with `View<Container>(c, viewer)` or `View<Container>Identity(c)`.
- **A view is exactly two words: `struct{ impl <shape interface> }`.** The
  interface erases the container's type parameter, which a converting view needs
  because `MapSetView[NT]` cannot name the `T` it came from. Construction is
  free; the cost is that passing a view into a shape-interface parameter boxes
  (~14.5 ns, 1 alloc) where passing a *container* does not. If that ever matters,
  ADR `0018` records the alternative: per-container `…IdentityView` types.
- **A view's zero value reads as empty**, like its container's. The exception is
  `VectorView.At`, which panics, because an index is out of range for anything
  empty.
- **A plain `[]T` has a view too, and it views a *value*** (ADR `0016`).
  `ViewSlice(s)` takes a slice, not `*[]T`: an element write shows through and an
  **append does not**, because the append made a different slice. Reach for
  `Vector` when growth must be visible to every holder.
- **`c.View()` is the identity view; a conversion is named** (ADR `0022`,
  revisiting `0012`). `View()` is free — the container is one word, so it boxes
  into the view's interface field without allocating — and it replaced the
  `View<Container>Identity` constructors. `ViewSliceIdentity` is the one that
  stayed a function: a plain `[]T` has no methods to hang one on. A *converting*
  view still names its conversion: `ViewMapSet(s, viewer)`.
- **A view is read-only in STRUCTURE, never in contents.** Through a sealed,
  un-castable view over a container of pointers, the pointed-to values stay
  mutable. Go cannot express "contains no pointers" as a constraint — not with
  `comparable`, not with a marker interface, not with a type set — so this half
  is a documented convention, as it is for `slices.Clone`. It is recorded as a
  compiled call site (`TestSealDoesNotFreezeContents`) rather than only a doc
  comment, so it breaks the build if the API drifts. Name the conversion with a
  viewer when it matters.
- **Hash containers convert keys both ways; ordered containers convert values
  only** (ADR `0012`). `SortedSet` and `SortedMap` key on `cmp.Ordered`, which
  admits only immutable value types, so their keys need no protection — which is
  what sidesteps order preservation. **Revisit if sorted containers ever sort by
  a function.**

### Reads, writes and bulk operations

- **`Keys`/`Values`/`All` mean what the stdlib means** (ADR `0017`). `All` yields
  pairs; `Keys` and `Values` each yield one half. A set is **key-only** — its
  element is its key. A `Vector` is keyed by **position**, so it has
  `Positions`/`PositionSlice` and no `Keys`.
- **The ordered reads come in two spellings.** `MinKey`/`MaxKey`/`FloorKey`/
  `CeilKey`/`RangeKeys` return keys and are on sorted sets **and** sorted maps,
  which is what lets one `SortedKeys[K]` cover both. `Min`/`Max`/`Floor`/`Ceil`/
  `Range` return pairs and are on sorted maps only. `RangeKeys` on a map is a
  native key-only walk, not a derived one: deriving it would copy every value to
  discard it, measured at 14.9x for wide values (ADR `0017`).
- **Bulk operations take slices, and every `*Slice` result is a full copy**
  (ADR `0017`). That contract is what makes `d.DeleteAll(d.KeySlice()...)`
  correct rather than lucky. Cross-container construction is
  `NewVector(m.KeySlice()...)`.
- **`New*` constructors copy their input and never take ownership of it.** An
  adopting constructor is worth 1.16x-1.70x and is deferred to a later ADR, under
  a *distinct name*, taking `[]T` rather than `...T`, and storing `slices.Clip`
  of what it is given — a spread carries the source's **capacity**, so `src[:2]`
  arrives as len 2, cap 1024.
- **Single-element mutators take one element; bulk ones are variadic** (ADRs
  `0015`, `0017`). A `...T` call costs a fixed ~0.3-0.8 ns and no allocation —
  ~60% of a slice append, ~5% of a map insert. Collapsing the split *cannot*
  remove it, because a map's `Set` takes two arguments and its `SetAll` takes
  pairs.
- **An iterator reaches a bulk operation through `slices.Collect`**, and a `Seq2`
  through a hand-written pair loop. This is the one place the slice currency
  forces an allocation a streaming API would not.

### Still open

- **Serialization is unsettled and currently silently lossy** for the
  slice-backed containers, which have only unexported fields. `Map` round-trips,
  being a map. **Handle it once across the library in its own ADR** — do not add
  `MarshalJSON` to a single container in the meantime.
- **`Vector` has no mutation contract.** ADR `0015` punted one until there is a
  second mutable sequence; the mutation *shape* interfaces are deferred for the
  same reason (ADR `0018`).
- **ADRs `0001`-`0007` use pre-`0008` names**, and `0008`-`0017` use pre-`0018`
  names (`HashSet`, `HashDict`, `SortedDict`, `KeyValue`). They are left that way
  deliberately: they record decisions as they were made. `0018` has the mapping.

## Experiments

`experiments/` holds measurement harnesses that answer design questions about
Go itself — not tests of this library. For comparisons that exercise this
library's own API, see **Call sites** below. Each is **its own module**, so the root
`go test ./...` never runs them and they stay out of the library's dependency
graph. `experiments/copycost/` is the worked example; copy its shape.
`experiments/sealing/` shows the other thing an experiment can do: its `run.sh`
asserts that a particular assertion still **fails** to compile, so a change that
reopens a closed hole breaks the experiment instead of passing quietly.

An experiment is `experiments/<name>/` containing:

| File | Purpose |
|---|---|
| `go.mod` | `module github.com/krelinga/go-containers/experiments/<name>` |
| `doc.go` | Package comment: the question being asked, and links to the two files below |
| `*_test.go` | The harness |
| `run.sh` | Regenerates `bench.txt`; honours `COUNT`, `BENCHTIME`, `OUT` |
| `bench.txt` | Raw output with a provenance header. Generated — never hand-edit |
| `RESULTS.md` | The findings, ending with a durable/perishable split |

Rules that make the results trustworthy:

- **Benchmarks, never tests.** Measurements are wall-clock and would flake as
  assertions. If something is worth asserting, assert it in the library's own
  tests (`testing.AllocsPerRun`), not here.
- **Use typed sinks, not `any`.** Assigning a result to an `any` sink boxes it,
  adding an allocation and ~12ns to every measurement. See `copycost/sink.go`.
- **Include a zero-cost baseline benchmark.** It is how you detect harness
  overhead leaking into the real numbers.
- **`-count=10` minimum, and report benchstat's ± interval.** A `-count=1`
  number committed as a baseline invites false regressions later.
- **Record provenance** — date, Go version, CPU, virtualization. `run.sh`
  writes this header; without it the numbers are unfalsifiable.
- **Separate durable conclusions from perishable numbers.** Absolute timings are
  machine-specific and rot. Implementation facts (allocation behaviour, GC
  scanning, complexity) do not. State which is which.

When an experiment settles a design question, record the decision in
`docs/adr/NNNN-<slug>.md` citing the experiment. The ADR outlives the numbers.

No CI job should gate on these benchmarks: on shared runners the noise floor
exceeds most effects worth reading.

## Call sites

Before building a container, write the code that would use it. The test:
**if a realistic call site is not clearly shorter or safer than the
`slices`/`maps` equivalent, the container is not earning its place.** This is
the cheapest way to kill a bad design — an afternoon rather than months.

This is deliberately **not** an experiment. `experiments/` isolates each harness
in its own module precisely so it cannot couple to the library. Call sites are
worth something only when they *are* coupled: compiled against the real API, and
breaking the build when it drifts. A call-site comparison that cannot fail is
worthless, and module isolation is what would make it unable to fail.

The artifact changes form once, so it lives in two places.

### Phase 1 — sketches, in the ADR proposing the container

Fenced Go that does not compile, labelled a sketch. Show the same task twice:
once with the stdlib (`map[K]struct{}`, `[]T`, `slices`/`maps`), once with the
API you wish existed. These sketches are the evidence the ADR decides on, exactly
as `copycost` numbers were the evidence for `0001`.

Record **rejected** designs too. A container that failed its call-site test is
the most valuable result to have written down, and the easiest to lose.

### Phase 2 — `callsites_test.go` at the repo root

- **`package containers_test`, never `package containers`.** An internal test
  file can reach unexported identifiers, which gives a flattering and dishonest
  view of ergonomics. The external test package forces the real public import
  path, so the API is experienced the way a caller experiences it.
- **Pair the functions.** `withStdlib` / `withContainer` performing an identical
  task, adjacent, so the comparison reads in one screen.
- Use `Example*` functions where the usage belongs in godoc. They compile and run
  under the root `go test ./...`, so API drift breaks the build.
- ADR `0001`'s `testing.AllocsPerRun` guardrail belongs here — "this accessor
  does not allocate" is a real assertion, unlike anything in `experiments/`.

### Land the stdlib baseline first

Commit `callsites_test.go` containing **only** the stdlib versions, before
writing any container code. When the container lands, rewrite that same file.
The reviewable artifact is the **diff**, not a side-by-side.

A side-by-side written after implementation is rigged: the container is already
paid for, and the baseline gets written to lose. A baseline committed in advance
is the only form of this test that can still come back negative.

### Do not quantify ergonomics

No line counts, token counts, or readability scores. They lend false rigor, and
line count in particular rewards a clever one-liner over three obvious
statements. This judgement is subjective; the structure exists to keep it honest
and recorded, not to fake objectivity.
