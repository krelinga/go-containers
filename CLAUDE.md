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
Go itself — not tests of this library. Each is **its own module**, so the root
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
