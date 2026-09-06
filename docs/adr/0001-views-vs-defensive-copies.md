# 1. Read-only views vs. defensive copies

- **Status:** Accepted. No container implements it yet, so it constrains the
  first one written rather than describing existing code.
- **Date:** 2026-09-06
- **Evidence:** `experiments/copycost/` (`RESULTS.md`, regenerate with `run.sh`)

## Context

A container that wraps `[]T` or `map[K]V` has to decide what an accessor hands
back. Returning the underlying slice lets callers mutate the container's
internals and exposes them to append-aliasing. Returning a copy is safe, but
"defensive copies are cheap" was an assumption we had not measured.

We measured it. The headline finding is that the assumption is **true for the
case it is usually applied to, and false in the two cases that actually hurt**:

- A one-shot copy of a small, pointer-free slice costs ~10 ns plus ~0.5 ns per
  element. That is noise. Copy freely here.
- A **retained** copy of a pointer-bearing type is re-scanned by the GC on every
  cycle for as long as it is held — 2.4x the cycle cost for 200k elements. The
  charge recurs; it is not paid once.
- A **repeated** copy is the real hazard. An O(n) copying accessor called n
  times in a caller's loop is O(n²): 36 800x slower than a view at n=16 384,
  allocating 2.1 GB in a single iteration.

The decisive point is not that copies are expensive. It is that a copying
accessor **hides an O(n) cost behind O(1)-looking syntax**. `s.Items()[i]` reads
as indexing. Nothing at the call site signals that it just cloned the container.

## Decision

**There exist cases where a read-only aliased view is justified on performance
grounds, and they are characterised by two conditions holding together:**

1. **the access crosses an API boundary**, and
2. **the accessor is expected to be called many times.**

That is precisely the case the measurements single out. A copying accessor is
O(n) per call, so a caller looping over it is O(n²) — 36 800x slower than a view
at n=16 384, allocating 2.1 GB in one pass. The cost is also invisible at the
call site, since `s.Items()[i]` reads as indexing.

Where either condition fails, **copy**:

- An accessor called once per operation on a small, pointer-free slice costs
  ~10 ns plus ~0.5 ns per element. That is noise. Copy and move on.
- At a boundary crossed once, where the caller escapes our control entirely and
  the copy is not in a loop, the copy buys real safety for no measurable cost.

Two supporting rules:

- **Maps get their own accounting.** `maps.Clone` is ~6x slower per entry than
  `slices.Clone` and allocates per bucket (258 allocs at 65 536 entries). Do not
  reason about map and slice copies with one rule.
- **Retention is a separate axis from repetition.** A copy held long-term whose
  elements contain pointers is re-scanned every GC cycle. That argues for not
  *retaining* copies; it does not by itself argue for views.

**Deliberately not decided here:** how a view is actually expressed — interface
versus struct with an unexported field, and what if anything enforces
read-only-ness against a determined caller. That is an implementation question
with its own trade-offs, and it gets its own ADR once there is a container to
try it on.

## Consequences

- A view **aliases**; it is not a snapshot. Holders see the provider's later
  mutations. This must be documented per type — it is the same hazard C++ has
  with a `const` view over a reallocating `vector`, and it is a semantics
  decision we own, not something the view type solves for us.
- Two ways to read a container is more API surface than returning `[]T`, and
  every view has to be converted at the boundary of any third-party API, which
  all speak `[]T` and `map[K]V`. This is the standing cost of the decision.
- "This accessor does not allocate" is testable via `testing.AllocsPerRun`, so
  the view rule can become an enforced guardrail rather than a convention.
- The two conditions are a judgement call at design time, not something the
  compiler checks. "Expected to be called many times" is a claim about callers
  we do not control, and we will sometimes get it wrong.
- Until the follow-up ADR lands, any view we write is provisional in form. Do
  not build much on its concrete shape.
