# 23. Composable views

- **Status:** **Accepted** and **implemented** (2026-09-25).
- **Date:** 2026-09-25 (accepted and implemented the same day)
- **Evidence:** `experiments/sealing/` (`RESULTS.md` §3).
- **Relates to:** ADR `0011` and `0012`, which both recorded *views that compose*
  as an open follow-up; `0012` decision 5 (the constructor shape this changes);
  `0022` (whose seal makes the composition gap permanent); `0002` (methods cannot
  have type parameters — decisive); `0015` (`Vector`, whose growth contract this
  repairs); `0016` (whose decision 1, *no `Slice` type*, **stands** — see
  *Rejected alternatives*); `0021` (`Each`, which removes the per-layer
  allocations); `0009` (`Map`, for `AsMap`).
- **Out of scope:** how the library should handle plain slices. A `Slice[T]` type
  was proposed inside this ADR and withdrawn; the analysis is kept in full under
  *Rejected alternatives*, and **a later ADR will take the question up properly.**

## The problem

**A view cannot be re-viewed.** Every converting constructor takes the concrete
container:

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

### And a live defect that falls out of the same cause

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
it fell out of the impl holding a slice rather than the container. Nothing
documents it and no test covers it.

Part 2 fixes it for free, which is the strongest single argument in this ADR.

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
| source is the **container** (today; not composable) | **1** alloc | 537.1 ns / 6 |
| **source is the concrete view type** | **1** alloc | 538.5 ns / 6 |
| source is the **sealed shape interface** | **2** allocs | 538.4 ns / 6 |

**Taking a view as the source costs nothing per element.** The decimals are shown
deliberately: rounded to whole nanoseconds the last two read 539 and 538, which
would imply an ordering between two numbers 0.1 ns apart. It is one number written
three times. The extra dynamic call a view source introduces
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
| 1 | 538.5 ns / 6 | — |
| 2 | 697.3 ns / 9 | +158.8 ns, +3 allocs |
| 3 | 874.4 ns / 12 | +177.1 ns, +3 allocs |

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

`View()` is free — 0.39 ns, 0 allocations (ADR `0022`) — so this is spelling, not
cost. It is also *consistent* with the seal: after `0022`, handing a container
anywhere read-only already means writing `.View()`.

### `ViewSliceWith` is the one exception, and stays one

A plain `[]T` has no view, because **it has no methods** — `[]T` is not this
package's type to extend. So `ViewSliceWith` keeps a `[]T` source where every
other `…With` takes a view, and `ViewSliceIdentity` stays a function where every
container has a `View()` method.

That is not an API inconsistency to design around; it is the language, and every
Go reader already knows it. A `Slice[T]` adapter would give slices a receiver —
and would cost four measured irregularities to remove this one. See *Rejected
alternatives*.

## Part 3: `AsMap`

Adjacent rather than central, and it fell out of the withdrawn slice work: a
conversion cannot infer its type arguments, so a caller wrapping a `map[K]V`
writes them out in full every time.

```go
containers.Map[string, int](m)   // today: both type arguments, always
containers.AsMap(m)            // inferred
```

```go
func AsMap[K comparable, V any](m map[K]V) Map[K, V] { return Map[K, V](m) }
```

Free — the conversion is free, since a defined map type has the same
representation as its underlying type, and the call inlines away. `Map` has needed
this since ADR `0009` introduced it; `0009` did not discuss inference and the gap
was simply never noticed.

**On the name.** It was `CastMap` in an earlier draft and became `AsMap` to match
how Go libraries spell this. Go's spec calls these *conversions*, not casts, and
"cast" carries an unchecked-reinterpretation connotation from other languages that
does not apply to a free change of named type. `As…` is also the established
prefix for a cheap, non-copying reinterpretation in the standard library's
vicinity, where `To…` tends to imply work. `MapOf` was considered and dropped: it
reads as a constructor taking elements, which is what `New*` does elsewhere in
this package.

## The resulting surface

| container | identity | converting |
|---|---|---|
| `MapSet[T]` | `s.View()` | `ViewMapSetWith(v, viewer)` |
| `SortedSet[T]` | `s.View()` | — nothing to convert (ADR `0012` §3) |
| `Map[K,V]` | `m.View()` | `ViewMapWith(v, viewer)` |
| `SortedMap[K,V]` | `m.View()` | `ViewSortedMapWith(v, viewer)` |
| `Vector[T]` | `v.View()` | `ViewVectorWith(v, viewer)` |
| a plain `[]T` | `ViewSliceIdentity(s)` | `ViewSliceWith(s, viewer)` — `[]T` source |

**A method for identity and a `…With` function for a conversion, for everything
this package defines a type for.** A `[]T` gets functions for both, because it is
a builtin.

## What composition means per view kind

| view | composes | notes |
|---|---|---|
| `MapSetView[NT]` | keys, both directions | `FromKeyView` chains in reverse and may fail at either stage |
| `MapView[NK, NV]` | keys both ways, values outbound | |
| `SortedMapView[K, NV]` | **values only** | keys are `cmp.Ordered` and pass through (ADR `0012` §3) |
| `VectorView[NT]` | elements outbound | **and gains live growth tracking** — see the defect above |
| `SortedSetView[T]` | **not at all** | no converting constructor exists, by decision (`0012` §3) |

A `VectorView` produced by `ViewSliceIdentity` composes like any other, with the
header semantics a slice view has always had (`TestSliceViewObservesTheHeaderNotTheVariable`).

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

Both belong in `callsites_test.go` as a stdlib-baseline-first diff.

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
- **Viewer composition in the library.** A composing viewer works and is a handful
  of lines, but it overlaps entirely with view composition for anyone who owns
  their container. One mechanism, and views are the one callers ask for.
- **A `Slice[T]` adapter.** Below, in full.

### `Slice[T] []T` — proposed in this ADR and withdrawn

**This is the second time the type has been proposed and rejected.** ADR `0016` is
titled *"A view over a plain slice, and no `Slice` type"*; it set out to propose
the type and its measurements turned it around. This ADR proposed it again — to
give a slice a receiver for `View()`, which is an argument `0016` could not have
weighed, because `View()` did not exist until ADR `0022`. **ADR `0016` decision 1
stands.**

The shape was: a defined `[]T` with value receivers, the position-keyed reads,
`Set(i int, e T)`, no `Append` method, no constructor, a distinct `SliceView[NT]`,
`ViewSliceWith` sourced from it, and `CastSlice` for inference.

**What is genuinely attractive about it**, and what a later ADR should weigh
rather than rediscover:

- **It gives slices a receiver.** `hosts.View()` where `hosts` is declared
  `Slice[T]`, rather than `ViewSliceIdentity(hosts)`.
- **Construction is free of API.** Its state *is* its underlying type, so
  `Slice[int]{1,2,3}` and `make(Slice[int], 0, 64)` both work — and that second
  form hands over ADR `0006`'s size hint, worth 2.7x–3.7x, with no `New*` to carry
  it. `Map` already works this way; there is no `NewMap` either.
- **It would be the only *sequence* that round-trips through `encoding/json`.**
  Measured: a declared `Slice[T]` field marshals to `["a","b"]` where a `Vector`
  field marshals to `{}`, because `Vector`'s state is unexported. ADR `0016`'s
  Task K measured this as a non-win, but only *because its decision 1 kept the
  field a plain slice* — the answer changes once the field is the adapter.
- **Callers may simply find it easier to express**, declaring the field as the
  adapter rather than converting at each boundary. Not quantified, per CLAUDE.md.

**Why it was withdrawn: the consistency arithmetic runs the wrong way.** It
removes one irregularity and adds four, each measured in this session.

Removed: a `[]T` needs `ViewSliceIdentity(s)` where every container has
`c.View()`.

Added:

| | measured |
|---|---|
| `SliceView` would be the **only view that does not track its source** | view=2 where the slice is 3 |
| `Slice` would be the **only container where "copies share" is half true** | elements share, length does not |
| `Slice.View()` would be the **only `View()` that allocates** | 24 B cannot box free where 8 B can |
| `Slice` would be the **only sequence with no `Append` method** | `Vector` has one |

**And the irregularity it removes is not an API irregularity at all — it is the
language.** A method cannot be put on `[]T`. `ViewSliceIdentity(s)` being a
function is the obvious consequence of `[]T` not being this package's to extend,
and every Go reader knows that already. The four it adds *are* API
irregularities, because `Slice` would sit in the container table beside the others
and measurably behave unlike them.

**The staleness one is the deepest, and it is not fixable.** A slice view holds a
pointer to a heap *copy* of the slice header — not the header inline (a view is an
interface, two words) and not a pointer to the caller's variable. Every other view
holds something that is itself a reference to shared state, a map header or a
`*state` pointer, so it re-reads and cannot disagree with its source:

| view | holds | later length change visible? |
|---|---|---|
| `MapSetView`, `MapView` | a map header (one word, reference type) | **yes** |
| `SortedSetView`, `SortedMapView` | the container = `*state` | **yes** |
| `VectorView` | `Vector[T]` = `*state` | **yes** (ADR `0015`) |
| a view over a `[]T` | a **boxed copy of the header** | **no** |

A `*Slice[T]` source *does* fix it — 0 allocations too, since one word boxes free,
and it tracks both `append` and `s = s[:1]`. It was built and measured. It fails
for two other reasons:

- **It tracks wholesale variable reassignment, which no real view does.** Measured
  on the shipped library: `Vector` view=2 after the variable is reassigned to a
  4-element vector, `MapSet` view=2 against variable=1, `Map` view=1 against
  variable=2. Every view holds the container *value*. A pointer to a variable
  follows the variable, so this trades *"misses changes the others see"* for
  *"sees a change the others miss"* — a surprise rather than a limitation.
- **It requires a pointer receiver**, so `CastSlice(raw).View()` and
  `Slice[int]{1,2}.View()` stop compiling — you cannot take the address of a
  function result or a composite literal. The prototype spelled the method
  `ViewTracking`, so the error read `cannot call pointer method ViewTracking on
  Slice[int]`; the name is incidental, the rule is not. It also makes `Slice` the
  only container with mixed receivers, where ADR `0002` chose uniformity and
  `0018` kept it while flipping which kind.

**There is no third form.** To track its source a view needs the container to be
*itself* a reference to shared state. For a slice that means holding a pointer —
`struct{ st *[]T }` — at which point it is not a defined `[]T` (no literals, no
`make`, no JSON round-trip, no free conversion) and it **is `Vector`**. ADR
`0016`'s "wall in the analogy" is exactly this: a map is a reference type, a slice
is not, and nothing in the slice world copies that.

So the library's existing answer stands: **`Vector` is the sequence whose growth is
visible to every holder, and a `[]T` gets a view over the header it had when you
handed it over.** Both halves are now asserted, in
`TestSliceViewObservesTheHeaderNotTheVariable` and
`TestContainerViewsTrackTheirSource`.

### Also withdrawn with it

`CastSlice`, `SliceView`, a `Slice`-sourced `ViewSliceWith`, and the deletion of
`ViewSlice`/`ViewSliceIdentity`. `AsMap` survives on its own merits (Part 3).

## Decisions

1. Converting constructors take the **concrete view type** as their source.
2. They are renamed with a **`With` suffix**.
3. `ViewSliceWith` keeps a **`[]T` source**, and `ViewSliceIdentity` stays a
   function. A builtin has no methods; this is the language, not an inconsistency
   to design around.
4. The **`Vector` growth defect is fixed here**, not separately. It is a behaviour
   change — a converting `Vector` view starts seeing appends — and nothing depends
   on the old behaviour, which contradicted `View`'s doc comment and ADR `0015`.
5. **`AsMap` is added.**
6. **No viewer composition** in the library.
7. **No `Slice[T]` type.** ADR `0016` decision 1 stands.

## Implementation order

1. Repoint the four container-sourced converting constructors — `ViewMapSet`,
   `ViewMap`, `ViewSortedMap`, `ViewVector` — at view sources, and rename to
   `…With`. Rename `ViewSlice` to `ViewSliceWith` without changing its `[]T`
   source.
2. The `Vector` fix falls out of step 1: `convertedElems` stops holding
   `v.ValueSlice()`. Land its regression test here.
3. Add `AsMap`.
4. Update CLAUDE.md: the view-construction bullets gain the `…With` spelling, and
   the `[]T` row gains a note that its two functions are the language rather than
   an exception.

## Open — deferred to a later ADR

**How the library should handle plain slices.** The `Slice[T]` analysis above is
the input, not the answer. What that ADR has to reconcile:

- A slice is not a reference type, so no adapter over one can behave like the
  other containers. Anything that does is `Vector`.
- Yet three real wants survive: a receiver for `View()`, `encoding/json`
  round-tripping for a sequence, and whatever ergonomic gain there is in declaring
  a field as the adapter.
- The JSON want overlaps the **library-wide serialization problem** CLAUDE.md
  already records as owed — slice-backed containers are silently lossy — and is
  probably better solved there than by one type that happens to dodge it.
