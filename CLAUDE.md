# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status

Still greenfield: there is no library source and no tests yet, so `go build ./...` at the root
matches no packages. Treat design decisions as open and do not assume a prior structure exists.

What does exist: `docs/adr/` (accepted design decisions), `experiments/` (measurement harnesses,
each its own module), and this file.

Intent, per the module path `github.com/krelinga/go-containers`: a generic (type-parameterized)
container library.

## Commands

```sh
go build ./...
go vet ./...
go test ./...
go test -run '^TestName$' ./...        # single test
go test -run '^TestName$/^subtest$' ./...
go test -race ./...
gofmt -l .                             # list unformatted files; -w to rewrite

# Experiments are separate modules; the root ./... does not reach them.
cd experiments/<name> && ./run.sh      # regenerate that experiment's bench.txt
COUNT=20 BENCHTIME=1s ./run.sh         # more samples
```

## Conventions

- **Package name is `containers`, not `go-containers`.** The `go-` prefix belongs to the repo name
  only; it is not part of the import identifier.
- `go.mod` pins `go 1.26.7` at patch granularity, so the toolchain must be at least that version.
  The devcontainer's Go feature supplies it; a host Go older than 1.26.7 will refuse to build.
- Single flat package at the repo root — add new container types as sibling files, not subpackages,
  unless there is a reason to split.
- **Read `docs/adr/` before designing a container type.** Accepted ADRs are binding on new code.
  `0001` governs when an accessor returns a read-only view rather than a copy.

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
