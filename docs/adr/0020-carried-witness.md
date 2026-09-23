# 20. Views by carried witness

- **Status:** **Proposed.** The shape is measured and specified; it is not
  chosen, and two of its consequences want deciding first.
- **Date:** 2026-09-23
- **Evidence:** `experiments/witness/` (`RESULTS.md`), with history in
  `experiments/views/` (ADR `0011`'s harness).
- **Relates to:** ADR `0011` (which measured a witness and found it cheapest),
  `0012` (which closed it off), `0013` (whose erratum this removes), `0018`
  (whose view structs this replaces), `0019` (whose spans would inherit the
  same parameters).

## The problem

**Iterating a view allocates, and has since ADR `0013`.** It is the oldest
unfixed thing in the library, it is live in shipped code, and three ADRs have
recorded it without removing it:

| iterating 64 elements | | allocs |
|---|---|---|
| the container | 350.2 ns | **0** |
| a converting view over it | 552.8 ns | **6** |

The cause is structural rather than incidental. A view erases the container's
type parameter behind an interface — it must, because `MapSetView[NT]` cannot
name the `T` it came from — and **an `iter.Seq` returned through a dynamic call
cannot be stack-allocated at the range site.** Every mitigation tried so far has
moved the cost rather than removed it: ADR `0018` measured that wrapping the
interface in a struct changes nothing, because the inner call stays dynamic.

## The history, and the mistake in it

ADR `0011` measured a **type-parameter witness** — carry the converter as a type
parameter rather than a value — and found it the cheapest shape available: one
word, free to construct, free to box.

ADR `0012` closed it off:

> A witness must be **stateless**, and `FromKeyView(NK) (K, bool)` generally
> cannot be — turning `"alpha"` back into the `*Item` it names needs a registry,
> which is state.

That is correct, and it is about **one variant of two**. The variant measured
and rejected materialises the converter's zero value per call:

```go
type pureWitness[T, NT any, VW toKeyView[T, NT]] struct{ c Container[T] }

func (v pureWitness[T, NT, VW]) Keys() iter.Seq[NT] {
	var vw VW   // <- the zero value; this is what cannot hold state
	…
}
```

**A carried witness keeps the converter in a field**, and the call is still
static, because `VW` is a concrete type known at compile time:

```go
type carriedWitness[T, NT any, VW toKeyView[T, NT]] struct {
	vw VW               // a value -- may hold a registry
	c  Container[T]
}
```

Measured, that form dispatches statically **and** holds state:

| | | allocs |
|---|---|---|
| view holding a shape interface (today) | 552.8 ns | **6** |
| carried witness, **stateful** viewer | **401.3 ns** | **0** |

**So ADR `0012`'s objection does not reach the shape worth wanting.** It rejected
the idea on the pure form's constraint; the carried form appears never to have
been considered. ADR `0013` already flagged the closure as "a real cost of
`0012` that was not priced at the time" — this ADR is that cost being re-read.

## What it measures

From `experiments/witness/`.

**Iteration — the erratum, removed:**

| | | allocs |
|---|---|---|
| the container, no view | 350.2 ns | **0** |
| interface view (today) | 552.8 ns | **6** |
| pure witness | 411.2 ns | **0** |
| carried witness, stateless viewer | 423.1 ns | **0** |
| carried witness, **stateful** viewer | **401.3 ns** | **0** |

The residual ~60 ns over the bare container is the per-element conversion, which
is the work a view exists to do.

**Point reads — 3.4x:** 0.3749 ns against 1.290 ns.

**Boxing into a shape interface, where field order decides the answer:**

| | width | boxing |
|---|---|---|
| carried, viewer **last**, stateless | 16 B | 12.27 ns, **1 alloc** |
| carried, viewer **first**, stateless | **8 B** | **0.3105 ns, 0 allocs** |
| carried, viewer first, **stateful** | 16 B | 12.23 ns, 1 alloc |
| interface view (today) | 16 B | 12.23 ns, 1 alloc |

A zero-size field is padded at the back of a struct and free at the front. **With
the viewer declared first, a stateless converting view is one word and boxes
free** — strictly better than today. A stateful one is two words and boxes
exactly as today does.

## The design

```go
type MapSetView[T comparable, NT any, VW CanViewMapSet[T, NT]] struct {
	vw VW          // FIRST -- see the field-order rule below
	s  MapSet[T]
}

func ViewMapSet[T comparable, NT any, VW CanViewMapSet[T, NT]](
	s MapSet[T], vw VW,
) MapSetView[T, NT, VW]
```

**Caller code:**

```go
// construction -- the viewer's type is inferred from the argument
v := containers.ViewMapSet(items, itemKeys{known: registry})

// the type, spelled in full
func handOut() containers.MapSetView[*item, string, itemKeys]

// or behind an alias, which is how it would usually be written
type ItemNameView = containers.MapSetView[*item, string, itemKeys]
func handOut() ItemNameView
```

**Three rules it needs:**

1. **The viewer field is declared first.** At the back it is padded and the view
   stops being pointer-shaped, which costs an allocation at every generic
   boundary — silently. Same class of hazard as ADR `0018`'s "do not add a
   second field to a view struct", and it wants the same doc line.
2. **An identity view uses a zero-size identity viewer**, so it stays one word
   and boxes free. `IdentityViewer` already exists (`viewers.go`) and is
   zero-size today.
3. **`VW` is inferred at construction and named everywhere else.** It appears in
   the return type, so a caller writing the type writes all three parameters, or
   an alias.

## What it costs

- **The type carries three parameters instead of one.** ADR `0011` already
  recorded this: *"four type arguments, and inference cannot help — `R` and `C`
  appear in no argument. Without an alias every construction site spells all
  four."* Inference does help at construction here, because `vw` is an argument;
  it does not help anywhere the type is written.
- **One view type per viewer, not per container.** Today `MapSetView[string]` is
  one type whatever produced it. Under a witness,
  `MapSetView[*item, string, itemKeys]` and `MapSetView[*item, string, otherKeys]`
  are unrelated types. Boundaries that accept either must take a shape
  interface — and boxing a *stateful* view into one is exactly where the
  witness stops being free.
- **Aliases move the verbosity rather than removing it.** They work at use
  sites, which is most sites. They do not help a caller writing a generic
  signature, who takes a shape interface instead.
- **It is an unusual idiom.** ADR `0011`: *"a reader meeting it for the first
  time has to work out what `var conv C` is doing."* Less true of the carried
  form, where the field is visible, but still true of the type parameter.

## What to decide

1. **Whether the shape interfaces survive this.** They exist partly to abstract
   over view types; a witness makes view types *more* numerous, which argues
   they matter more — but boxing a stateful view into one costs the allocation
   the witness just removed. These two questions are the same question.
2. **Whether ADR `0019`'s spans inherit the witness.** A span produced by a view
   would carry the same `VW`, or erase it and pay the dispatch the span shape
   exists to avoid. `0019` is paused; this should be settled before it resumes.
3. **Whether identity views keep a separate spelling.** With a zero-size
   identity viewer the general form is already optimal, so
   `ViewMapSetIdentity(s)` could become a thin wrapper — or stay, since it is
   what most callers want and it hides two type parameters.
