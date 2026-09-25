# 20. Views by carried witness

- **Status:** **ABANDONED** (2026-09-25). Both shapes proposed here are
  withdrawn: the problem they existed to solve was answered more cheaply by ADR
  `0021`, and measurement showed they were paying for it with the zero-value rule
  in exchange for a performance gain of roughly nothing. **Superseded in intent
  by ADR `0022`**, which keeps `0018`'s view structs and seals the shape
  interfaces instead. The measurements below stand and are cited by `0021`; only
  the proposals are dead. See *Why this was abandoned*.
- **Date:** 2026-09-23, abandoned 2026-09-25
- **Evidence:** `experiments/witness/` (`RESULTS.md`), with history in
  `experiments/views/` (ADR `0011`'s harness). The measurements that killed it
  are in `experiments/sealing/`.
- **Relates to:** ADR `0011` (which measured a witness and found it cheapest),
  `0012` (which closed it off), `0013` (whose erratum this tried to remove),
  `0018` (whose view structs this proposed to replace, and which stands),
  `0019` (which no longer waits on this), `0021` (which solved the motivating
  problem), `0022` (which supersedes this in intent).

## Why this was abandoned

Four things, in the order they landed.

**1. The motivating problem got a cheaper answer.** This ADR exists because
iterating a view allocates (ADR `0013`'s erratum), and both shapes here are ways
to restructure views so that it does not. ADR `0021`'s `Each` removes the
allocations **without touching view shape at all** — 6 allocations to 0, a fixed
~72 ns to ~0.6 ns, and 4.5x on early exit. Once the erratum has a direct fix,
nothing here is load-bearing.

**2. The span merge was not free after all.** The parameterised-container shape
became the favoured one because it merges spans with views, dissolving the
instantiation cycle that defeats every other attempt. That still holds as a type
result. But `experiments/witness` then measured a span returned through a view
interface at **one allocation** — the span is a container plus two bounds,
wider than one word, so it cannot box free — against **zero** for a direct
`EachKeyInRange`. The merge buys uniformity, not speed.

**3. The performance case evaporated.** The concrete view struct's measured
penalty was boxing into a shape interface: 14.8 ns and 1 allocation against
~2.3 ns and 0. `experiments/sealing` found that a read-only API naming the
**concrete view type** has nothing to box into — **1.86 ns, 0 allocations**. The
penalty only lands on code generic over *container kind*, of which this library
currently contains none.

**4. And the wrapper was never slow.** The comparison nobody had run: today's
`struct{ impl … }` against the bare interface it wraps, same implementation
underneath.

| n=64 | StructView (today) | bare interface |
|---|---|---|
| point read | 1.40 ns / 0 | 1.06 ns / 0 |
| iterate | 422 ns / 3 | 425 ns / 3 |
| `View()`, identity | **0.39 ns / 0** | 0 allocs |
| **zero value** | **reads empty** | **PANICS** |

Iteration is indistinguishable. The extra hop inlines away; there is one dynamic
call either way.

**So the trade both shapes were offering was: give up the zero-value rule — one
of the library's three central rules, and the thing `0018` was explicitly asked
to preserve — to win ~0.3 ns on point reads and an allocation that a concrete
parameter type already avoids.** A nil interface cannot read as empty, and no
amount of design changes that.

Two costs specific to the parameterised container are also worth keeping on the
record, because they were real: it moves five type parameters onto the
*container*, the type every caller writes; and it requires a **stateless**
witness, which loses stateful inbound key conversion (`FromKeyView` with a
registry) — the exact objection ADR `0012` raised, reappearing in a shape that
had seemed to escape it.

**What survives.** Everything measured below, in particular the attribution of
the three allocations, the break-even curve, and the 34 ns cost-context table —
all cited by `0021`. The finding that a zero-size field is free at the front of a
struct and padded at the back. And the instantiation-cycle analysis in *Could
spans be expressed through the witness?*, which remains the reason ADR `0019`
cannot express spans through a type parameter. **ADR `0019` no longer waits on
this ADR**; its open question 2 is void.

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

## The viewer interfaces, per container

The viewer interfaces themselves do not change — `viewers.go` already declares
exactly the set a witness needs, because a witness is constrained by an
interface rather than holding one. What changes is that the viewer becomes a
**named type parameter** on the view, so its identity is visible in every
signature that spells the view out.

| container | viewer constraint | converts | view type parameters |
|---|---|---|---|
| `MapSet[T]` | `CanViewMapSet[T, NT]` | keys, both ways | `[T, NT, VW]` — 3 |
| `SortedSet[T]` | **none** | nothing | `[T]` — 1 |
| `Map[K, V]` | `CanViewMap[K, NK, V, NV]` | keys **and** values | `[K, NK, V, NV, VW]` — **5** |
| `SortedMap[K, V]` | `CanViewSortedMap[V, NV]` | values only | `[K, V, NV, VW]` — 4 |
| `Vector[T]` | `CanViewVector[T, NT]` | values only | `[T, NT, VW]` — 3 |
| `[]T` | `CanViewSlice[T, NT]` | values only | `[T, NT, VW]` — 3 |

And the two primitives they are built from, unchanged:

```go
type KeyViewer[K, NK any] interface {
	ToKeyView(K) NK            // outbound
	FromKeyView(NK) (K, bool)  // inbound; a failed conversion reads as a miss
}

type ValueViewer[V, NV any] interface {
	ToValueView(V) NV
}
```

### Four things the table says

**`SortedSet` needs no viewer at all**, so it takes no witness and keeps its
single type parameter. Its keys are `cmp.Ordered`, which admits only immutable
value types, so there is nothing to protect (ADR `0012`). It is the one view
that is *simpler* under this design than the others, and the one whose spelling
does not change.

**`Map` is the expensive case: five type parameters.** A witness view must name
its *source* types as well as its presented ones — `K, V` for the container,
`NK, NV` for what it shows, `VW` for the converter. Today `MapView[NK, NV]` is
two. This is the concrete shape of the verbosity cost, and it lands hardest on
the container most likely to be projected.

**Inference still works at construction**, verified: `ViewMap(m, vw)` resolves
all five, because `K, V` come from the container argument and `NK, NV, VW` from
the viewer argument. The parameters have to be written only where the *type* is
written — a field, a return type, a variable declaration — which is what an
alias exists for:

```go
type ItemNameView = containers.MapView[*item, string, *item, itemView, itemViewer]

v := containers.ViewMap(m, itemViewer{known: registry})  // inferred
func handOut() ItemNameView                              // aliased
```

**Three of the six constraints are the same interface.**
`CanViewSortedMap[V, NV]`, `CanViewVector[T, NT]` and `CanViewSlice[T, NT]` all
declare exactly `ValueViewer`, with nothing added. They are three names for one
shape, kept distinct so each container documents what it requires rather than
pointing at a shared name. That was defensible when a viewer was an ordinary
argument; under a witness the constraint is part of the view's *type*, so the
redundancy becomes visible in the API rather than only in the declaration —
`MapSetView[…, VW CanViewVector[…]]` would compile just as happily as the right
one. Whether to collapse them into `ValueViewer` is an open question below.

### Identity viewers

Both existing identity viewers are `struct{}` — **zero-size**, which is what the
design needs: a stateless witness declared as the view's first field makes the
view one word, and a one-word view boxes into a shape interface for free
(0.3105 ns and no allocation, against 12.27 ns and one).

```go
type IdentityViewer[K, V any] struct{}      // both halves
type IdentityValueViewer[V any] struct{}    // the value half only
```

So `ViewMapSetIdentity(s)` becomes `ViewMapSet(s, IdentityViewer[T, T]{})` under
the covers, and the identity path is the *cheapest* one rather than a special
case — which reverses today's position, where an identity view and a converting
view cost the same because both hold an interface.

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

## Could spans be expressed through the witness?

ADR `0019`'s spans and this ADR's witnesses both ride along with a view, so an
obvious economy suggests itself: **expand the `CanView*` constraints for sorted
containers with a bounds question, and let the concrete witness hold the start
and limit keys.** A span then *is* a view, with no new types at all.

It was tried. The first form does not compile, and the form that does costs more
than it saves.

### Form 1: narrowing wraps the witness — rejected by the language

`Range` must have a single return type per instantiation, so narrowing has to
wrap the existing witness in a bounding one:

```go
func (v SortedMapView[K, V, NV, VW]) Range(lo, hi K) SortedMapView[K, V, NV, bounded[K, V, NV, VW]]
```

Go rejects it outright:

```
instantiation cycle:
    VW instantiated as bounded[K, V, NV, VW]
```

**This is a hard limit, not a preference.** And it is exactly the operation
ADR `0019` requires: sub-spans, `m.Range(0, 100).Range(50, 60)`, which under
this shape would need `bounded[bounded[VW]]`.

### Form 2: the witness re-bounds itself — compiles, and costs

The repair is a self-referential constraint: the witness names the concrete type
it returns, so narrowing replaces the witness rather than wrapping it.

```go
type CanWindow[K cmp.Ordered, V, NV any, Self any] interface {
	ValueViewer[V, NV]
	Bounds() (K, K, bool)
	Reversed() bool
	WithBounds(lo, hi K) Self
	WithReversed(bool) Self
}
```

Verified: this compiles, and `v.Range("a","z").Range("b","y").Backward()` chains
with no nesting. Three costs follow, and together they are decisive.

**Every sorted view becomes a span, and pays for it.** Bounds cannot be optional:
an unbounded witness and a bounded one would be different types, which is Form 1
again. So a *plain* sorted view carries `lo`, `hi` and a direction whether or not
anyone wanted a window — **48 B with string keys, against 8 B for the
zero-size-witness view this ADR's main proposal gives.** That forfeits the
pointer-shapedness that makes a stateless view box for free, on the whole sorted
half of the library, to serve the windowed case.

**The constraint stops describing a conversion.** `CanViewSortedMap` would
declare four methods, two of which have nothing to do with viewing, and **every
custom viewer would have to implement them** — including viewers written by
callers who only ever wanted to project a value type. The name `CanView…` would
be wrong, and so would the concept: viewing converts *element types*, windowing
bounds an *extent*. They are different axes and this makes them one parameter.

**It is ADR `0019`'s span, with the fields moved somewhere worse.** A span has to
hold bounds and a direction; the question is only where. On a span struct, they
sit next to the container and cost nothing that is not inherent. On the witness,
they cost the same bytes, plus a self-referential constraint, plus four methods
on every viewer, plus the loss of the free-boxing case.

### Form 3: `Range` belongs wholly to the witness

Both forms above fail because **narrowing re-parameterizes the view**. Giving
the windowing job entirely to the witness avoids that — nothing ever changes
type — and it splits into two readings, one of which is the best version of this
idea.

**3a — the window is chosen at construction, and there is no `Range` at all.**

```go
plain  := containers.ViewSortedMap(m, Unbounded[V]{})      // 8 B, boxes free
window := containers.ViewSortedMap(m, Window(lo, hi))      // bounds only when asked
```

This compiles, and it **restores what Form 2 lost: bounds are optional again.**
The witness type is fixed once, so an unbounded view keeps the zero-size witness
and stays one word. No cycle, no mandatory bounds, no four-method constraint.

Its cost is functional rather than structural: **a view can no longer be
narrowed or reversed.** To window, you construct a new view *from the
container* — and a caller holding only a view cannot, because a view is the
read-only boundary and does not expose what it wraps. That is precisely the
caller ADR `0019` is for. Composition — `m.Range(0, 100).Backward()` — goes
entirely.

**3b — `Range` is a free function that wraps the witness.** A method could not
(instantiation cycle); a function can, verified. But the wrapping is real:

| narrowings | width | witness type |
|---|---|---|
| 0 | 8 B | `Unbounded` |
| 1 | 40 B | `Bounded[…, Unbounded]` |
| 2 | 72 B | `Bounded[…, Bounded[…, Unbounded]]` |
| 3 | **104 B** | …and so on |

**+32 B and one type layer per narrowing, without bound** — and every layer
keeps bounds the inner layer already subsumes, so the growth is carrying dead
information. The type name grows with it, which an alias cannot fix because the
depth is a property of the call chain rather than of the declaration.

### So: keep them separate

Three forms, three failures of different kinds: Form 1 is rejected by the
language, Form 2 makes bounds mandatory for every sorted view, Form 3a gives up
windowing from a view, and Form 3b grows without bound. **Spans stay the
separate types ADR `0019` specifies, and witnesses stay about conversion.** The two do meet, but at a smaller joint than this: a span produced
by a *view* must carry that view's witness so the conversion survives into the
window — which is a type parameter on the span, not bounds on the witness.

That is the interaction to settle when `0019` resumes, and it is recorded there
as an open question rather than here.

## Alternative: parameterise the container, keep views as interfaces

A different placement of the same witness, and — measured — a better trade than
the carried view for this library.

**The container carries the witness as a type parameter; `View()` returns an
interface.**

```go
type Map[K comparable, V, NK, NV any, VW CanViewMap[K, NK, V, NV]] map[K]V

type MapView[NK, NV any] interface {   // an INTERFACE again -- few parameters
	Len() int
	Keys() iter.Seq[NK]
	// …
}

func (m Map[K, V, NK, NV, VW]) View() MapView[NK, NV] { return m }
```

Verified to compile: a defined map type does carry type parameters absent from
its underlying type, and stays **8 B**, so `View()` boxes it for **free**
(0.3154 ns, 0 allocations).

### What it fixes that the carried witness does not

**It merges spans with views** — and this is the decisive property, because it
dissolves the instantiation cycle rather than working around it.

When the view is an interface, a **span is simply another implementation of it**.
`Range` returns the interface, so narrowing never re-parameterizes anything:

```go
func (m SortedMap[…]) Range(lo, hi K) SortedView[K, NV]   // returns a span
func (s span[…])      Range(lo, hi K) SortedView[K, NV]   // returns itself, re-bounded
```

Verified: `m.Range("a","z").Range("b","y").Backward().Range("c","x")` compiles
and is still `SortedView[K, NV]` at every depth. **No cycle, no nesting, no
32 B per narrowing, no growing type name** — the interface erases the concrete
type, which is exactly what the four rejected forms in the section above were
each failing to do a different way.

**It also solves the verbosity**, from the caller's side: `MapView[NK, NV]`
rather than `MapView[K, NK, V, NV, VW]`.

### What it costs

**The type parameters move to the container rather than disappearing.** A method
cannot introduce type parameters, so `View()` must name its projected types from
the receiver — `Map[K, V, NK, NV, VW]`, five on the type every caller writes,
against two today. That is the real price, and it is paid at every signature
mentioning a container, not only at ones mentioning a view.

**The witness must be stateless.** A defined map type has nowhere to store a
value. That is ADR `0012`'s original objection, now accepted deliberately rather
than worked around: `ValueViewer` is naturally stateless, but
`FromKeyView(NK) (K, bool)` is not — turning `"alpha"` back into the `*item` it
names needs a registry. **Inbound key conversion therefore stops working for
anything but a pure computation**, and `Has(nk)`/`Get(nk)` on a key-converting
view are the casualties. The library's own `views_test.go` viewer holds such a
registry today.

**And it keeps half of ADR `0013`'s erratum.**

| iterating 64 elements | | allocs |
|---|---|---|
| view holds an interface (today) | 552.6 ns | **6** |
| **this shape** | **440.3 ns** | **3** |
| carried witness, concrete view | 408.4 ns | **0** |
| this shape used concretely, no `View()` | 399.1 ns | **0** |

Today has two dynamic hops — the view's `impl` call, then a per-element viewer
call. This shape makes the conversion **static** (the witness is concrete on the
container) and leaves only the outer `Keys()` dynamic. That halves it. It does
not remove it, because one dynamic call is enough.

## What three allocations actually are, and when they matter

Worth pinning down, because the whole choice turns on whether they are
affordable.

**Where they come from**, attributed by measurement rather than inspection:

| | allocs |
|---|---|
| obtain the iterator only, **through the interface** | **1** |
| obtain the iterator only, **concretely** | 1 |
| obtain **and** range, through the interface | **3** |
| obtain **and** range, concretely | **0** |
| range a **pre-obtained** iterator | **2** |

One is the iterator closure; two are the range machinery — escape analysis names
them as the loop variable, the compiler's range-over-func state, and the yield
closure the loop body becomes.

**The instructive pair is the concrete rows.** Obtaining a closure concretely
also costs one allocation in isolation, yet the full concrete loop costs
**zero**: when the call is ranged immediately, the compiler inlines `Keys`,
proves the closure never outlives the loop, and fuses both into an ordinary
loop. Through an interface it cannot see which `Keys` will run, so nothing
fuses and all three become real.

**How big the container must be before it stops mattering.** The overhead is a
flat **~34.4 ns**, independent of size; the walk costs **~6.81 ns** per element.

| n | concrete | via view | overhead |
|---|---|---|---|
| 0 | 3.3 ns | 35.9 ns | **990%** |
| 8 | 43.5 ns | 75.9 ns | 74% |
| 32 | 220 ns | 256 ns | 16% |
| 64 | 435 ns | 458 ns | 5.4% |
| 128 | 816 ns | 831 ns | 1.8% |
| 1024 | 6.90 µs | 6.76 µs | **−2.1%** |
| 4096 | 27.6 µs | 26.0 µs | **−5.6%** |

| overhead drops below | at roughly |
|---|---|
| 10% | n ≈ 50 |
| 5% | n ≈ 100 |
| 2% | n ≈ 250 |
| 1% | n ≈ 500 |

Past a few hundred elements the difference goes **negative** in places, which is
the signature of reading jitter rather than signal.

Two consequences. **The empty container is the worst case by far** — 990%,
because a concrete walk over nothing compiles to almost nothing while the view
still pays all three. A hot loop over views that are usually empty is the
pathological pattern, and it is a plausible one. And **the cost is per `Keys()`
call, not per element**: ten thousand walks of five elements costs 340 µs of
overhead; one walk of fifty thousand costs 34 ns.

**What 34 ns is, in company.** Measured on the same machine:

| | |
|---|---|
| interface method call | 1.8 ns |
| map lookup (3 entries) | 2.7 ns |
| map lookup (65k) | 4.5 ns |
| mutex lock+unlock, uncontended | 7.7 ns |
| allocate 64 B | 14.1 ns |
| buffered channel send+recv | 24.8 ns |
| `fmt.Sprintf("%d")` | 27.0 ns |
| **the view overhead** | **34.4 ns** |
| `time.Now()` | 35.3 ns |
| `json.Marshal`, two fields | 58.6 ns |
| goroutine spawn + join | 222 ns |
| allocate 4 KiB | 507 ns |
| `os.Stat` | 617 ns |

**It is one `time.Now()`.** Any path that formats a string, logs a line,
marshals JSON, touches a channel or allocates a page cannot measure it. It is
~19x an interface call and ~14x a map lookup, so the one place it is real is a
tight in-memory loop that does nothing else — which is the profile ADR `0015`
already tells callers to use a plain `[]T` for rather than a container.

## What to decide — VOID

Kept for the record; none of these is live. Items 1 and 4 are answered by ADR
`0022` (the shape interfaces survive and are sealed; identity views keep a
spelling, now `View()`). Item 2 is void: ADR `0019`'s spans inherit nothing from
a witness that is not being built. Item 3 — whether `CanViewSortedMap`,
`CanViewVector` and `CanViewSlice` collapse into `ValueViewer` — is genuinely
still open and independent of this ADR; it should be reopened wherever the viewer
interfaces are next touched.


1. **Whether the shape interfaces survive this.** They exist partly to abstract
   over view types; a witness makes view types *more* numerous, which argues
   they matter more — but boxing a stateful view into one costs the allocation
   the witness just removed. These two questions are the same question.
2. **Whether ADR `0019`'s spans inherit the witness** as a type parameter. A
   span produced by a view must carry that view's `VW` so the conversion
   survives into the window, or erase it and pay the dispatch the span shape
   exists to avoid. That is the *small* joint between the two ADRs; the large
   one — expressing spans through the witness itself — is examined above and
   rejected. `0019` is paused; this should be settled before it resumes.
3. **Whether `CanViewSortedMap`, `CanViewVector` and `CanViewSlice` collapse
   into `ValueViewer`.** They are the same interface under three names. Distinct
   names document each container's requirement; one name removes a redundancy
   that a witness makes visible in the type rather than only in the declaration.
4. **Whether identity views keep a separate spelling.** With a zero-size
   identity viewer the general form is already optimal, so
   `ViewMapSetIdentity(s)` could become a thin wrapper — or stay, since it is
   what most callers want and it hides two type parameters.
