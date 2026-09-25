# 23. Composable views, and a `Slice` type

- **Status:** **Proposed.** Two changes, scoped together for a specific reason:
  Part 2 makes every converting constructor take a *view* as its source, and a
  plain `[]T` has no view and no `View()` method — so without Part 3 the slice
  constructor would be the one exception to the rule Part 2 establishes.
- **Date:** 2026-09-25
- **Evidence:** `experiments/sealing/` — `RESULTS.md` §3 for composition and the
  `Slice` adapter costs. The analysis this reverses is
  `experiments/sliceadapter/`.
- **Relates to:** ADR `0011` and `0012`, which both recorded *views that compose*
  as an open follow-up; `0012` decision 5 (the constructor shape this changes);
  `0022` (whose seal makes the composition gap permanent, and whose `View()`
  method is the new argument for a `Slice` type); `0002` (methods cannot have type
  parameters — decisive); `0015` (`Vector`, whose growth contract this repairs);
  `0009` (`Map`, the adapter `Slice` is modelled on); `0021` (`Each`, which
  removes the per-layer allocations).
- **Supersedes:** ADR `0016` decision 1, *"Add no new type"*. `0016`'s decisions 2
  and 3 stand, and its analysis of why a slice cannot be `Map` stands — see
  *Part 3*.

## The problem

Three gaps, and they share a shape: **the view layer treats a container as the
only thing a view can come from.**

### 1. A view cannot be re-viewed

Every converting constructor takes the concrete container:

```go
func ViewMapSet[T comparable, NT any](s MapSet[T], viewer CanViewMapSet[T, NT]) MapSetView[NT]
```

so handing it a view does not compile:

```
cannot use s.View() (value of struct type containers.MapSetView[int])
as containers.MapSet[int] value in argument to containers.ViewMapSet
```

An open follow-up in ADR `0011`, restated in `0012` — and ADR `0022` has made it
**permanent rather than awkward**: the seal means a view's holder cannot reach the
container to re-view it. A view handed across a boundary is terminal. Its holder
can read it and nothing else; they cannot narrow it further for their own callers,
which is the layering a view exists to support.

Multi-stage conversion is possible today only as caller-side boilerplate: write a
composing *viewer* and pass it to the existing constructor. Verified working
(`int → string → "n=7"` through `ViewSortedMap`), but it works only if you own
the container, must be written per viewer family, and composes viewers where the
caller was thinking about views.

### 2. A converting `Vector` view silently stops tracking growth

A live defect, not a missing feature:

```go
v := containers.NewVector(1, 2)
conv  := containers.ViewVector[int, int](v, dbl{})   // converting
ident := v.View()                                    // identity
v.Append(3)
// conv.Len() == 2, ident.Len() == 3, v.Len() == 3
```

Measured. `ViewVector` is `convertedElems[T, NT]{v.ValueSlice(), viewer}` — it
**snapshots at construction**, because `convertedElems` holds a `[]T`. The
identity view holds the `Vector` and tracks growth, as ADR `0015` requires and as
`View`'s doc comment promises. Nobody decided the converting form should differ;
it fell out of the impl holding a slice. Nothing documents it and no test covers
it.

Part 2 fixes it for free, which is the strongest single argument in this ADR.

### 3. `[]T` is the only view source with no container

Every container has one method and one function. A plain slice has **two
functions and no method**:

| | identity | converting |
|---|---|---|
| every container | `c.View()` | `View<Kind>(c, viewer)` |
| a plain `[]T` | `ViewSliceIdentity(s)` | `ViewSlice(s, viewer)` |

ADR `0022` introduced `View()` as a method and a slice has no receiver to hang it
on. That is a **new** argument for a `Slice` type: ADR `0016` rejected one, but
`View()` did not exist when it did.

## Part 1: the method question is settled by the language

`ViewWith` cannot be a method:

```go
func (v MapSetView[T]) ViewWith[NT any](vw KeyViewer[T, NT]) MapSetView[NT]
// syntax error: method must have no type parameters
```

A converting view introduces `NT`, which is not on the receiver and cannot be
inferred from it. ADR `0002` already recorded that methods cannot have type
parameters; this is that rule biting. **`View()` works as a method only because
identity means `NT == T`** — there is no new parameter to introduce. That
asymmetry is not an inconsistency to fix; it is the shape of the language.

So the converting constructors stay functions. **They gain a `With` suffix**, so
the split reads as deliberate rather than arbitrary: `c.View()` for identity,
`View<Kind>With(v, viewer)` for a conversion.

## Part 2: a converting constructor's source is a view

```go
func ViewMapSetWith[T, NT any](v MapSetView[T], viewer CanViewMapSet[T, NT]) MapSetView[NT]
```

Two shapes were measured at n=64. The third row is the alternative where the
source is the sealed shape interface rather than the concrete view type.

| | construct | iterate |
|---|---|---|
| source is the **container** (today; not composable) | **1** alloc | 537 ns / 6 |
| **source is the concrete view type** | **1** alloc | 539 ns / 6 |
| source is the **sealed shape interface** | **2** allocs | 538 ns / 6 |

**Taking a view as the source costs nothing per element.** 537 / 539 / 538 ns is
one number written three times. The extra dynamic call a view source introduces
is paid **once per `Keys()` call**, obtaining the iterator — not once per element.
A per-element penalty was expected and there is none.

The sealed-tier source is rejected on two counts. It must box a two-word view to
enter the interface, so it construct-allocates twice. And it would accept *any*
key-exposing view whatever container it came from — which sounds like more
composability but means re-viewing a `SortedSetView` through a `MapSetView`
constructor would silently discard the ordering guarantee its type was carrying.
**Per-kind sources keep a view's kind in its type**, which is what ADR `0018`'s
naming scheme is for.

### Composition depth

| depth | iterate | delta |
|---|---|---|
| 1 | 539 ns / 6 | — |
| 2 | 697 ns / 9 | +158 ns, +3 allocs |
| 3 | 874 ns / 12 | +177 ns, +3 allocs |

**Linear, ~160 ns and 3 allocations per layer, per call** — the nested
range-over-func machinery, one closure plus range state and yield closure for each
additional `for range` in the chain. Depth is cheap to add and cheap to walk, but a
deeply composed view is a worse hot-loop candidate than a flat one, and `Each`
(ADR `0021`) removes the per-layer allocations for the same reason it removes the
flat ones.

### What it costs callers

One `.View()` at each converting call site:

```go
containers.ViewMapSet(s, viewer)            // before
containers.ViewMapSetWith(s.View(), viewer) // after
```

`View()` is free for every container whose value is one word — 0.39 ns, 0
allocations (ADR `0022`) — so for those this is spelling, not cost. It is also
*consistent* with the seal: after `0022`, handing a container anywhere read-only
already means writing `.View()`.

## Part 3: `Slice[T]`

```go
type Slice[T any] []T
```

A defined slice type, modelled on `Map[K, V]` being a defined `map[K]V` (ADR
`0009`). Value receivers, the position-keyed reads, `Set(i int, e T)` for in-place
element writes, **no `Append` method**, and **no constructor** — `make` and a
composite literal both work on it natively.

Measured rather than assumed: a defined `[]T` satisfies the whole of
`innerPositionValues[int, T]` — `Len`, `At`, `Positions`, `PositionSlice`,
`Values`, `ValueSlice`, `All`, `AllSlice` — on value receivers, and `Set` works
too, because an element write goes through the shared backing array. Only `Append`
is impossible. The compile-time assertion is in `experiments/sealing`.

### What ADR 0016 decided, and what has changed

ADR `0016` is titled *"A view over a plain slice, and **no `Slice` type**"*. It
set out to propose exactly this type and its measurements turned it around, so
this needs to answer it directly rather than quietly reverse it.

**What still stands, unchanged:**

- **A `Slice[T]` cannot have an `Append` *method*.** A slice's length lives in its
  header, which *is* the value, so a value receiver cannot grow it and a pointer
  receiver would forfeit the assignability that makes an adapter an adapter.
  **Growth itself is not lost** — `s = append(s, x)` works on a `Slice[T]`
  variable exactly as on a `[]T`, verified — so what the type lacks is the
  *method*, not the capability. That is a smaller gap than ADR `0016`'s framing
  suggests, but it is still a real asymmetry with `Vector`, whose `Append` is a
  method precisely because its state is behind a pointer.
- **It fixes no slice hazard.** Naming a slice type changes no slice semantics.
  Every hazard ADR `0015` records survives — in particular, appends off a shared
  `Slice[T]` still alias. **`Vector` remains the answer where growth must be
  visible to every holder**, and this ADR does not narrow that.

**What changed:**

- **ADR `0022` introduced `c.View()` as a method.** `0016`'s reason for an adapter
  was that *the view might need one*, and it measured that the view does not. It
  could not have weighed "a slice has no receiver to hang `View()` on", because
  there was no `View()` method. **That is the only new argument being claimed**,
  and it is the whole of gap 3 above.
- **`Elems`/`Elems2` are gone** (ADR `0017` proposal D), which voids `0016`'s
  sharpest objection: that a type has one `All`, so a `Slice` yielding values
  could not also serve a pairs-shaped constructor.
- **`0016`'s "a fourth exception to ADR `0008`'s naming scheme" reads differently
  after `0018`**, which made `Map` *the centre* of the naming scheme rather than
  an exception to it. `Map` : `map` :: `Slice` : `[]T` — both are adapters named
  after the builtin they wrap, while the richer containers (`MapSet`, `SortedMap`,
  `Vector`) are `<Backing><Concept>`. On that reading `Slice` is consistent.
- **`0016`'s "a second sequence type whose difference from `Vector` is invisible
  in the name" is not answered**, and is accepted as a cost. The difference is
  real — `Slice` has value semantics and no growth; `Vector` hides reallocation
  from every holder — but it is not in the names, and the doc comments have to
  carry it.

The reason to take the type is the one ADR `0016` could not weigh, plus one it
did not consider: **a caller may find their own code easier to express by
declaring the field `Slice[T]` in the first place**, rather than converting at
each boundary. That is an ergonomic claim, deliberately not quantified (CLAUDE.md
forbids it), and it is the caller's judgement to make rather than ours to
measure.

### `CastSlice` and `CastMap` restore inference

A conversion cannot infer its type argument:

```go
containers.Slice(s)        // cannot use generic type Slice without instantiation
containers.Slice[int](s)   // the type argument is mandatory
```

A function call can. So:

```go
func CastSlice[T any](s []T) Slice[T]                 { return Slice[T](s) }
func CastMap[K comparable, V any](m map[K]V) Map[K, V] { return Map[K, V](m) }
```

Measured: both infer, and `CastSlice` costs **0 allocations** — the conversion is
free, since a defined slice type has the same representation as its underlying
type, and the call inlines away.

`CastMap` closes the same gap for `Map`, where a caller has had to write
`containers.Map[string, int](m)` in full since ADR `0009` introduced it. `0009`
did not discuss inference; the gap was simply never noticed.

### `Slice` keeps its own view type

`SliceView[NT]`, distinct from `VectorView[NT]` — **not merged, and not an
alias.** The two are identical in shape today and are expected to diverge; a
shared type or a generic alias would make that divergence a breaking change
rather than an addition.

The duplication is smaller than it looks, because **the plumbing is shared**: both
view structs wrap the same unexported `innerPositionValues[int, NT]`, so
`rawSlice`, the converting impl, and every inner-tier assertion serve both. What
duplicates is one view struct and its forwarding methods.

Measured in `experiments/sealing` (`compose.go`): two distinct view types over one
shared inner tier compile and convert correctly, each keeps a zero value that reads
as empty, and neither is assignable to the other — which is the point.

`ViewSliceWith` and `ViewVectorWith` therefore stay separate functions with
identical bodies over a shared impl. That is the price of not merging, and it is
paid deliberately.

### `Slice.View()` is the one `View()` that allocates

| | width, measured | `View()` |
|---|---|---|
| `Map[K,V]` | **8 B** — one word (map header) | **0 allocs** |
| `Vector[T]` | **8 B** — one word (`*state`) | **0 allocs** |
| `MapSet[T]` | **8 B** | **0 allocs** |
| `Slice[T]` | **24 B** — three words (ptr, len, cap) | **1 alloc** |

A one-word value boxes into the view's interface field for free; a three-word
slice header cannot. **This is not a regression** — `ViewSliceIdentity(s)` already
costs one allocation today, for the same reason, and it is inherent rather than
fixable. But ADR `0022` and CLAUDE.md both say `View()` is free, with "the
container is one word" as the stated reason, so `Slice` is a documented exception
and its doc comment must say so.

## The resulting surface

| container | identity | converting |
|---|---|---|
| `MapSet[T]` | `s.View()` | `ViewMapSetWith(v, viewer)` |
| `SortedSet[T]` | `s.View()` | — nothing to convert (ADR `0012` §3) |
| `Map[K,V]` | `m.View()` | `ViewMapWith(v, viewer)` |
| `SortedMap[K,V]` | `m.View()` | `ViewSortedMapWith(v, viewer)` |
| `Vector[T]` | `v.View()` | `ViewVectorWith(v, viewer)` |
| `Slice[T]` | `s.View()` | `ViewSliceWith(v, viewer)` |

`ViewSliceIdentity` is deleted outright — `CastSlice(s).View()` replaces it.
`ViewSlice` is not so much deleted as re-sourced: `ViewSliceWith` takes a
`SliceView[T]` where `ViewSlice` took a `[]T`, so it is a different function
wearing a related name, and a caller starting from a bare slice writes
`ViewSliceWith(CastSlice(s).View(), viewer)`.

**A method for identity, a `…With` function for a conversion, on all six.** The
asymmetry between them is required by the language (Part 1) and now the names say
so.

## What composition means per view kind

| view | composes | notes |
|---|---|---|
| `MapSetView[NT]` | keys, both directions | `FromKeyView` chains in reverse and may fail at either stage |
| `MapView[NK, NV]` | keys both ways, values outbound | |
| `SortedMapView[K, NV]` | **values only** | keys are `cmp.Ordered` and pass through (ADR `0012` §3) |
| `VectorView[NT]` | elements outbound | **and gains live growth tracking** — gap 2 |
| `SliceView[NT]` | elements outbound | a slice has no growth to track; ADR `0016` decision 2 |
| `SortedSetView[T]` | **not at all** | no converting constructor exists, by decision (`0012` §3) |

`SortedSetView` having no composing form is not an inconsistency to paper over.

## Call sites

**Sketch — the layering this exists for.** The second function cannot be written
at all today:

```go
// a registry hands out a view keyed by name
func (r *Registry) ByName() containers.MapSetView[string] {
	return containers.ViewMapSetWith(r.items.View(), r.nameViewer())
}

// its caller narrows further for ITS callers, without reaching the container
func Shortened(v containers.MapSetView[string]) containers.MapSetView[string] {
	return containers.ViewMapSetWith(v, firstEight{})
}
```

**Sketch — the `Vector` defect, as a regression test.** This **fails today**:

```go
v := containers.NewVector(1, 2)
conv := containers.ViewVectorWith(v.View(), dbl{})
v.Append(3)
if conv.Len() != 3 {
	t.Error("a converting view snapshotted instead of tracking growth")
}
```

**Sketch — the declaration site `Slice` is for**, which is the case
`CastSlice` does *not* serve:

```go
type Config struct {
	Hosts containers.Slice[string] // declared as the adapter, not converted at use
}

func (c Config) HostsView() containers.SliceView[string] { return c.Hosts.View() }
```

Compare a caller handed a plain `[]T` from elsewhere, who pays the cast:

```go
func hostsView(hosts []string) containers.SliceView[string] {
	return containers.CastSlice(hosts).View()
}
```

All three belong in `callsites_test.go` as a stdlib-baseline-first diff.

## Rejected alternatives

- **`ViewWith` as a method.** Illegal Go: `method must have no type parameters`.
- **`Apply` on the *viewer*.** This one is legal — the viewer's concrete type
  knows both `T` and `NT`, so no method type parameter is needed, and it compiles.
  Rejected because every viewer author would hand-write it, it cannot be supplied
  generically (a generic function cannot be a method, and an embedded helper
  cannot name its outer type), and a hand-written `Apply` is a place to get the
  wiring wrong. A free function is written once, in the library.
- **Parameterising the view by its viewer**, putting `NT` on the receiver so a
  method becomes possible. That is ADR `0020`'s carried witness, **abandoned**: it
  costs the zero-value rule.
- **A sealed-tier source parameter.** One more allocation, and it discards a
  view's kind. See Part 2.
- **Merging `SliceView` into `VectorView`**, or aliasing one to the other with
  `type SliceView[NT any] = VectorView[NT]`. Verified that generic aliases work at
  this Go version, so this is available — and rejected: the two types are expected
  to diverge, and an alias makes divergence a breaking change instead of an
  addition. Identical shape today is not evidence of identical shape later.
- **Collapsing `ViewSliceWith` into `ViewVectorWith`.** Follows from merging the
  views, and rejected with it. Their bodies are identical over a shared impl;
  their *types* are the point.
- **`Slice[T]` with a pointer receiver so it can `Append`.** Rejected by ADR
  `0016` and still rejected: a `*Slice[T]` cannot be produced by a free conversion
  from a `[]T` value, cannot be passed where a `[]T` is expected, and reintroduces
  the mixed-receiver problem ADR `0002` decision 2 rejected. A type that needs a
  pointer is `Vector`.
- **`AsSlice` / `ToSlice` / `SliceOf` instead of `CastSlice`.** Go's spec calls
  these *conversions*, not casts, and "cast" carries an unchecked-reinterpretation
  connotation from other languages that does not apply here. `Cast*` is kept for
  being short and unambiguous in practice, and because it pairs across the two
  adapters; the objection is recorded rather than dismissed.
- **Viewer composition in the library.** A composing viewer works and is a handful
  of lines, but it overlaps entirely with view composition for anyone who owns
  their container. One mechanism, and views are the one callers ask for.

## Decisions

1. Converting constructors take the **concrete view type** as their source.
2. They are renamed with a **`With` suffix**.
3. The **`Vector` growth defect is fixed here**, not separately. It is a behaviour
   change — a converting `Vector` view starts seeing appends — and nothing depends
   on the old behaviour, which contradicted `View`'s doc comment and ADR `0015`.
4. **`Slice[T]` is added**, superseding ADR `0016` decision 1, with `CastSlice`
   and `CastMap` for inference.
5. **`SliceView[NT]` is distinct from `VectorView[NT]`**, and `ViewSliceWith` from
   `ViewVectorWith`.
6. **No viewer composition** in the library.
7. **`Slice[T]` carries `Set(i int, e T)`**, spelled exactly as `Vector.Set` is, so
   the two sequence types share a mutator vocabulary as far as they can. An
   element write goes through the shared backing array and sticks. On the zero
   value it panics with an index error rather than a nil error — which is what
   `var s []T; s[0] = e` does, so the rule in CLAUDE.md holds even though the
   panic's text differs from the map-backed containers'.
8. **`Slice[T]` gets no constructor**, and needs none. Unlike every other
   container, its state *is* its underlying type, so the caller already has both
   spellings natively:

   ```go
   containers.Slice[int]{1, 2, 3}       // composite literal
   make(containers.Slice[int], 0, 64)   // sized construction, no API required
   ```

   That second line matters more than the first: ADR `0006`'s size hint is worth
   2.7x–3.7x for the map-backed containers and needs a `New*` to carry it, whereas
   `Slice` gets it from `make` for free. A constructor would add a name and buy
   nothing. Left open as a future change if a reason appears.
9. **`SliceView` and `VectorView` keep `PositionValues[P, V]`** rather than a
   narrower shared interface. Two satisfiers is not yet evidence that the
   contract is wrong.

## Implementation order

1. Repoint the four *container-sourced* converting constructors — `ViewMapSet`,
   `ViewMap`, `ViewSortedMap`, `ViewVector` — at view sources, and rename to
   `…With`. There are **five** converting constructors today; the fifth,
   `ViewSlice`, cannot move yet because its view source (`SliceView`) does not
   exist until step 3. The `Vector` fix falls out of this step —
   `convertedElems` stops holding `v.ValueSlice()` — so land its regression test
   here.
2. Add `Slice[T]`, its reads, `Set(i int, e T)`, and the inner-tier assertions. No
   `Append` method and no constructor (decisions 7 and 8).
3. Add `SliceView[NT]` and `ViewSliceWith`, sharing `rawSlice` and the converting
   impl with `VectorView`.
4. Add `CastSlice` and `CastMap`.
5. Delete `ViewSlice` and `ViewSliceIdentity`.
6. Update CLAUDE.md: the container table gains a `Slice` row and loses the
   *(no `Slice` type)* row; the view-construction bullets gain the `…With`
   spelling; the `View()` bullet gains the three-word exception.

## Open

- **A `Slice[T]` constructor**, if a reason ever appears. Decision 8 records why
  there is none today: `make` and a composite literal both work natively, so there
  is nothing a `New*` would carry.
- **`Slice[T]` has no mutation contract**, and neither does `Vector` — ADR `0015`
  punted one until there was a second mutable sequence, and `Slice` is now that
  second sequence. So the question `0015` deferred is live again, but it is a
  question about the *setter* vocabulary across both, which is wider than this ADR
  and should not be answered inside it. The mutation tripwire in `contracts.go`
  (`mutatesKeys`/`mutatesKeyValues`, ADR `0022`) is key-shaped and fits neither.
