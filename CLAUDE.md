# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status

`package containers` at the repo root is the library. Three containers, each with a `_test.go`
beside it:

| file | type | backing |
|---|---|---|
| `hashset.go` | `HashSet[T comparable]` | map, unordered |
| `sortedset.go` | `SortedSet[T cmp.Ordered]` | sorted slice |
| `hashdict.go` | `HashDict[K comparable, V any]` | map, unordered |
| `sorteddict.go` | `SortedDict[K cmp.Ordered, V any]` | sorted slice |
| `map.go` | `Map[K comparable, V any]` | a defined `map[K]V` — an **adapter**, not a default |
| `vector.go` | `Vector[T any]` | an owned slice, insertion-ordered (ADR `0015`) |
| — | *(no `Slice` type)* | a plain `[]T` gets a view instead (ADR `0016`) |
| `keyvalue.go` | `KeyValue[K, V]` | the pair type bulk operations carry (ADR `0017`) |
| `contracts.go` | the interfaces below | — |
| `views.go` | `SetView`, `DictView`, `SortedSetView`, `SortedDictView`, `IndexedView` | sealed read-only interfaces (ADRs `0011`, `0012`, `0013`, `0015`, `0016`, `0017`) |
| `viewers.go` | `KeyViewer`, `ValueViewer`, the `CanView<Container>` set | the conversions a view applies (ADR `0012`) |

There is **one** contract layer: `MutableSet[T]` and `MutableDict[K, V]` (ADR `0008`, narrowed by
`0013`, flattened by `0017`). Each declares the reads and the writes directly.

**`Elems` and `Elems2` are gone** (ADR `0017`). Their job was carrying a length for preallocating
constructors; bulk operations now take slices, which carry their own. Nothing replaced them — do
not reintroduce a universal read tier.

They take `any` elements and keys, not `comparable` (ADR `0012`). Implementations state their own
constraints.

**There is no read-only contract tier.** ADR `0013` deleted `Set` and `Dict`: a container
satisfies a structural read contract inherently, so one prevents nothing — a holder asserts back
and writes. Read-only is expressed by the sealed view interfaces instead. A function that must not
write takes `SetView`/`DictView`; a caller holding a container wraps it with
`View<Container>Identity`, which converts nothing and allocates nothing.

`callsites_test.go` holds every stdlib-vs-container comparison. Alongside: `docs/adr/` (design
decisions) and `experiments/` (measurement harnesses, each its own module). **Check an ADR's
status before treating it as binding** — most are Accepted, `0010` is Proposed, and `0014` is
**Rejected**, so its contents describe a road not taken. `0017` is Accepted **as proposal D**; it
also contains proposals A, B and C in full, which are *not* binding — they are the rejected
alternatives, kept because what they cost is the reason D was chosen.

Intent, per the module path `github.com/krelinga/go-containers`: a generic (type-parameterized)
container library. Still early — several ADRs constrain code not yet written more than they
describe code that exists, so treat unsettled areas as open.

## Commands

```sh
go build ./...
go vet ./...                           # REQUIRED before pushing -- see note below
go test ./...
go test -vet=all ./...                 # tests plus the FULL vet set, incl. copylocks
go test -run '^TestName$' ./...        # single test
go test -run '^TestName$/^subtest$' ./...
go test -race ./...
gofmt -l .                             # list unformatted files; -w to rewrite

# Experiments are separate modules; the root ./... does not reach them.
cd experiments/<name> && ./run.sh      # regenerate that experiment's bench.txt
COUNT=20 BENCHTIME=1s ./run.sh         # more samples
```

**`go test` does not run copylocks.** It runs only a high-confidence subset of vet
(`atomic, bool, buildtags, directive, errorsas, ifaceassert, nilfunc, printf, stringintconv,
tests`). The `noCopy` protection that container types rely on (ADR `0002`) is therefore invisible
to a plain `go test` — a shallow-copy `Clone` that silently shares the underlying map passes both
`go test ./...` and `go build ./...`. Run `go vet ./...` or `go test -vet=all ./...`.

## Conventions

- **Package name is `containers`, not `go-containers`.** The `go-` prefix belongs to the repo name
  only; it is not part of the import identifier.
- `go.mod` pins `go 1.26.7` at patch granularity, so the toolchain must be at least that version.
  The devcontainer's Go feature supplies it; a host Go older than 1.26.7 will refuse to build.
- Single flat package at the repo root — add new container types as sibling files, not subpackages,
  unless there is a reason to split.
- **Read `docs/adr/` before designing a container type.** Accepted ADRs are binding on new code;
  `0014` is Rejected and is not.
- **Containers stay pointers to non-copyable structs** (ADR `0002`), and `0014` re-affirmed that
  after designing the alternative in full. Reference-type containers were measured to be *free*
  at the representation level, but a reference container cannot be compared to nil, so it needs
  an `IsZero`/`IsNil` method while views — sealed interfaces since `0013` — use `== nil`. Every
  route to one spelling either kept two or reintroduced Go's typed-nil trap. **Do not reopen this
  without a new answer to that question**; the cost measurements are already done in
  `experiments/reftypes/`.
  `0001` governs when an accessor returns a read-only view rather than a copy. `0002` fixes the
  shape of a container type — uniform pointer receivers, a usable zero value, a `noCopy` field
  declared *first*, and no nil-receiver or nil-argument special cases. `0004` and `0006` govern
  bulk insertion and the sized read-only contract.
- **A new container satisfies `MutableSet` or `MutableDict` whole** (`contracts.go`), and the
  compile-time assertions at the bottom of that file are how you find out. That means the iterator
  reads, the `*Slice` materialisers, the single-element accessors and the mutators.
- **A view carries the same `*Slice` methods its container does** (ADR `0017`), so a container can
  be built from a view — otherwise the read-only boundary would be a dead end. Handing out a slice
  costs a view nothing, because the result is a copy; a converting view applies its viewer once per
  element while materialising.
- **Every container has a view, and a new one is not finished without it** (ADRs `0011`, `0012`
  and `0013`; `views.go`, `viewers.go`). Build one with `View<Container>(c, viewer)`, or
  `View<Container>Identity(c)` to convert nothing. A constructor returns a **sealed interface** —
  `SetView[NT]` or `DictView[NK, NV]`, the `Sorted*` forms which embed those and add the ordered
  reads, or `IndexedView[NT]`, which stands alone because a sequence's reads are positional. The
  concrete structs are unexported and may change.
- **A plain `[]T` has a view too, and it views a *value*** (ADR `0016`). `ViewSlice(s)` takes a
  slice, not `*[]T`, because a slice is a value and a reallocated slice is a new slice — which is
  exactly why `Vector` exists. So an element write is visible through the view and an **append is
  not**: the append made a different slice value the view was never given. It is the one view that
  costs an allocation, a slice header being three words. Reach for it to expose a `[]T` field
  read-only without changing the field's type; reach for `Vector` when growth must be visible to
  every holder.
- **`IndexedView` is named for the capability, and `ListView` is its declared destination**
  (ADR `0016`). A list is position/value pairs whose position type varies — `int` here, a cursor
  for `LinkedList` — so the general contract is `ListView[P, NT]`, and `IndexedView[NT]` becomes
  a generic alias for `ListView[int, NT]` when that lands. `ListView` is deliberately unclaimed
  until then, for the reason ADR `0010` gives for not calling itself `List`.
  The seal is one unexported method, and it works in both directions at compile time: a container
  cannot be passed where a view is expected (`missing method sealedView`), and a view cannot be
  asserted back to its container (`impossible type assertion`).
  A view is not a snapshot, is not proof against `reflect`+`unsafe`, and is not automatic: a
  boundary declared with an unsealed type — `MutableDict`, which containers satisfy — accepts the
  container as happily as ever. The seal binds where the boundary names it.
- **Ordered views substitute for unordered ones.** `SortedDictView[K, NV]` embeds
  `DictView[K, NV]`, and `SortedSetView[T]` embeds `SetView[T]`, so a consumer naming the base
  never names an implementation — a `Map` view and a `HashDict` view are the *same type*. This
  type-checks only because ordered views convert values only; **converting ordered keys would
  break the hierarchy** as well as reopening order preservation.
- **A view carrying a viewer costs one allocation, at construction, and nothing per boundary
  crossing.** A view that converts nothing — every `Identity` view, and every `SortedSetView` —
  is one word and allocates nothing at all, which is what makes the identity wrap free. Adding a
  method to a view interface is *not* a breaking change, since nothing outside the package can
  implement one.
- **A nil view panics when used, and nothing checks for it.** A view is an interface now, so
  `var v DictView[K, V]` is nil and `v == nil` compiles. Per ADR `0002` no method or constructor
  special-cases it.
- **There is deliberately no `View()` method** (ADR `0012`). It read as "give me a view" with no
  hint that key and value handling is a dimension at all, so it invited the assumption that keys
  were safe. Callers name the conversion, or name its absence with the `Identity` constructor.
- **Hash containers convert keys both ways; ordered containers convert values only** (ADR `0012`).
  `KeyViewer` has `ToKeyView(K) NK` outbound and `FromKeyView(NK) (K, bool)` inbound, where a
  failed conversion reads as a miss. `SortedSet` and `SortedDict` key on `cmp.Ordered`, which
  admits only immutable value types, so their keys need no protection: `Range`/`Floor`/`Ceil` take
  `K` directly and `SortedSetView` carries no viewer at all. This is what sidesteps
  order-preservation entirely — **revisit it if sorted containers ever sort by a function.**
- **`Vector` is not a replacement for `[]T`** (ADR `0015`). It earns its place at an API
  boundary, where a `[]T` field cannot be handed out read-only and an accessor over one must copy
  — O(n) per call, O(n²) in a caller's loop — or return a mutable interior. A `Vector` field hands
  out a `IndexedView` at O(1) and no allocation. It is *worse* than a slice for local code:
  an indexed loop costs ~24% because bounds-check elimination does not survive `At`, and ranging
  `All` costs ~6x a raw range. Two of ADR `0015`'s four call-site tasks are recorded as
  explicitly **not** wins so they are not cited as motivation later.
- **Every single-element mutator takes one element; every bulk one is variadic** (ADRs `0015`,
  `0017`). `Append(e T)` / `AppendAll(vs ...T)`, `Add(v T)` / `AddAll(vs ...T)`,
  `Delete(k K)` / `DeleteAll(ks ...K)`, `Set(k, v)` / `SetAll(kvs ...KeyValue[K, V])`. The split is
  measured, not stylistic: a `...T` call costs a fixed ~0.3-0.8 ns and **no allocation**, which is
  ~60% of a slice append and ~5% of a map insert. It survives because collapsing it *cannot* remove
  it — a dict's `Set` takes two arguments and its `SetAll` takes pairs — so splitting is what is
  uniform here.
- **`HashDict` is the default hash dict; `Map` is an adapter** (ADR `0009`). Reach for `Map` only
  when you need a free conversion from an existing `map[K]V`, builtin syntax, to pass the result
  where a `map[K]V` is expected, or working `encoding/json`. Everything else should use `HashDict`,
  which follows the same shape rules as the rest of the package.
- **Bulk operations take slices, and every `*Slice` result is a full copy** (ADR `0017`). A
  container materialises with `KeySlice`, `ValueSlice` or `AllSlice`; the result shares nothing with
  the container in either direction. That contract is what makes `d.DeleteAll(d.KeySlice()...)`
  correct rather than lucky — under a streaming API the same line silently corrupts a sorted
  container. Cross-container construction is `NewVector(d.KeySlice()...)`.
- **`New*` constructors copy their input and never take ownership of it.** This is uniform across
  every container so that `New*` means one thing. An adopting constructor is worth 1.16x-1.70x and
  is deliberately deferred to a later ADR, under a *distinct name*, taking `[]T` rather than `...T`,
  and storing `slices.Clip` of what it is given — a spread carries the source's **capacity**, so
  `src[:2]` arrives as len 2, cap 1024.
- **An iterator reaches a bulk operation through `slices.Collect`**, and a `Seq2` through a
  hand-written pair loop. The `*Seq` method twins are gone; this is the one place the slice currency
  forces an allocation a streaming API would not.
- **`Keys`/`Values`/`All` mean what the stdlib means** (ADR `0017` problem 4). `All` always yields
  pairs; `Keys` and `Values` each yield one half. A set is **key-only** — its element is its key, so
  it has `Keys`/`KeySlice` and no value side. A `Vector` is keyed by position, so `Values` yields
  elements and `All` yields index/element pairs.
- **`HashSet` and `HashDict` are structurally near-identical**, both a `noCopy` plus a map. A bug
  or an optimisation found in one applies to the other — check both.
- **Name new types per ADR `0008`.** Implementations are `<Ordering><Concept>` — `HashSet`,
  `SortedDict`. Contracts are `Mutable<Concept>` for writes and the bare concept for reads. The one
  exception is `Map`, which keeps the builtin's name because it is a thin naming of the builtin;
  a future `Slice[T] []T` would take the same exception.
- **The eager-dereference rule from `0002` bites repeatedly.** Every method must dereference its
  receiver on a path that *always* executes. Variadic methods, empty slices, and loops that can run
  zero times all skip it silently — so `v.AppendAll()` with no arguments must still panic on a nil
  receiver. It has been violated three times so far; `TestEmptyBulkCallsStillDereference` in
  `contracts_test.go` covers every bulk method, and is the test to extend when a container is added.
- **Serialization is unsettled library-wide, and currently silently lossy.** Container types
  have only unexported fields, so `json.Marshal` of a populated container returns `{}` with a
  **nil error**, discarding its contents; `json.Unmarshal` fails asymmetrically. This follows from
  the shape ADR `0002` fixes, so it will recur for every container added. **Handle it once across
  the library in its own ADR — do not add `MarshalJSON`/`UnmarshalJSON` to a single container in
  the meantime**, or the types will diverge before the decision is made.
- **ADRs `0001`-`0007` use the pre-`0008` names** and are deliberately left that way, since they
  record decisions as they were made. ADR `0008` section 2 has the old-to-new mapping. In
  particular `Set` there means today's `HashSet`, and `SortedMap` means `SortedDict`; `Set` now
  names the read-only set contract instead.

## Experiments

`experiments/` holds measurement harnesses that answer design questions about
Go itself — not tests of this library. For comparisons that exercise this
library's own API, see **Call sites** below. Each is **its own module**, so the root
`go test ./...` never runs them and they stay out of the library's dependency
graph. `experiments/copycost/` is the worked example; copy its shape.

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
