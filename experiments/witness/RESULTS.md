# The carried witness: static conversion for views

ADR `0011` measured a type-parameter witness as the cheapest view shape and
priced `Get`. ADR `0012` closed it off, because a witness must be stateless and
`FromKeyView` generally needs a registry. ADR `0013` then found that iterating a
view allocates per call — an erratum neither had in view.

This harness prices the witness on that operation, and separates two variants
nobody had separated:

- **pure** — `struct{ c C }` plus a type parameter `VW`, its zero value
  materialised per call. One word. **Stateless viewers only** — this is what
  ADR `0012` rejected.
- **carried** — `struct{ vw VW; c C }` with `VW` a *concrete* type parameter and
  a field. The call is still static, because `VW` is known at compile time, and
  the viewer may hold state.

See `bench.txt` for raw output and provenance.

## 1. The witness removes ADR 0013's erratum

Iterating a converting view over 64 elements:

| | | allocs |
|---|---|---|
| the container, no view at all | 350.2 ns | **0** |
| view holding a **shape interface** (today) | 552.8 ns | **6** |
| view with a **pure** witness | 411.2 ns | **0** |
| view with a **carried** witness, stateless viewer | 423.1 ns | **0** |
| view with a **carried** witness, **stateful** viewer | 401.3 ns | **0** |

**Zero allocations, including with a stateful viewer.** An `iter.Seq` returned
through a dynamic call cannot be stack-allocated at the range site; a static
call has nothing to box. The remaining ~60 ns over the bare container is the
per-element conversion, which is work the view exists to do.

## 2. ADR 0012's objection applies to one variant only

A **carried** witness keeps the viewer in a field, so a registry-holding viewer
works and still dispatches statically — 401.3 ns and no allocations above,
against 552.8 ns and six for the interface form.

That is the finding: *"a witness must be stateless"* is true of the pure form
and false of the carried one. ADR `0011` measured the pure form; ADR `0012`
rejected the idea on the pure form's constraint; the carried form appears never
to have been considered.

## 3. Point reads

| | |
|---|---|
| view holding a shape interface | 1.290 ns |
| pure witness | 0.3641 ns |
| carried witness | 0.3749 ns |

**3.4x**, and the same shape as every other dispatch result in this repo: a
fixed indirect-call cost, large as a share of something cheap.

## 4. Field order decides whether a stateless view is one word

A zero-size field at the END of a struct is padded, so a pointer to it cannot
run past the allocation. At the FRONT it costs nothing.

| | width | boxing into a shape interface |
|---|---|---|
| carried, viewer **last**, stateless | 16 B | 12.27 ns, **1 alloc** |
| carried, viewer **first**, stateless | **8 B** | **0.3105 ns, 0 allocs** |
| carried, viewer first, **stateful** | 16 B | 12.23 ns, 1 alloc |
| interface view (today) | 16 B | 12.23 ns, 1 alloc |

With the viewer declared first, a **stateless** converting view is one word and
boxes free — better than today on every axis. A **stateful** one is two words
and boxes exactly as today does. Declare the fields in the wrong order and the
first row is what you get, silently.

## Durable / perishable

**Durable.** A static conversion removes the per-call allocations that a
dynamic one forces on a returned iterator, whether or not the viewer holds
state. A carried witness dispatches statically *and* carries state; only the
pure form is constrained to stateless viewers. A zero-size struct field is free
at the front of a struct and padded at the back, which decides pointer-shapedness
and therefore whether boxing allocates.

**Perishable.** Every absolute number, and the ~60 ns conversion overhead, which
tracks the viewer's own cost.
