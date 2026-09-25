# 23. Composable views

- **Status:** **Proposed.** Recommends option A for the source parameter, and
  keeping the `View*` functions as functions — the second is not a preference,
  the language forbids the alternative.
- **Date:** 2026-09-25
- **Evidence:** `experiments/sealing/` (`RESULTS.md` §3).
- **Relates to:** ADR `0011` and `0012`, which both recorded *views that compose*
  as an open follow-up; `0012` decision 5 (the constructor shape this changes);
  `0022` (whose seal makes the gap permanent for a view's holder); `0002`
  (methods cannot have type parameters — decisive here); `0015` (the `Vector`
  growth contract this fixes by accident); `0021` (`Each`, which removes the
  per-layer allocations).

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

This was an open follow-up in ADR `0011`, restated in `0012`, and ADR `0022` has
now made it **permanent rather than merely awkward**: the seal means a view's
holder genuinely cannot reach the container to re-view it. Before `0022` the gap
was an inconvenience; now it is a wall.

Three consequences, in increasing order of how much they should bother us.

**1. Multi-stage conversion is caller-side boilerplate.** It *works* — a caller
can write a composing viewer and pass it to the existing constructor:

```go
type composedValue[V, M, NV any] struct {
	a interface{ ToValueView(V) M }
	b interface{ ToValueView(M) NV }
}
func (c composedValue[V, M, NV]) ToValueView(v V) NV {
	return c.b.ToValueView(c.a.ToValueView(v))
}
```

Verified end to end (`int → string → "n=7"` through `ViewSortedMap`). But it only
works **if you own the container**, it must be written per viewer family, and it
composes *viewers* where the caller was thinking about *views*.

**2. A view handed across a boundary is terminal.** The holder can read it and
nothing else. They cannot narrow the element type further for their own callers,
which is exactly the layering a view exists to support.

**3. A converting `Vector` view silently stops tracking growth.** This one is a
live defect, not a missing feature:

```go
v := containers.NewVector(1, 2)
conv  := containers.ViewVector[int, int](v, dbl{})  // converting
ident := v.View()                                   // identity
v.Append(3)
// conv.Len() == 2, ident.Len() == 3, v.Len() == 3
```

Measured. `ViewVector` is `convertedElems[T, NT]{v.ValueSlice(), viewer}` — it
**snapshots at construction**, because `convertedElems` holds a `[]T`. The
identity view holds the `Vector` and tracks growth, as ADR `0015` requires and as
`View`'s doc comment promises. Nobody decided the converting form should differ;
it fell out of the impl holding a slice. Nothing documents it and no test covers
it.

Fixing composition fixes this for free, which is the strongest argument here: a
converting view whose source is a `VectorView` walks the live view rather than a
copy.

## Thread 2, first: the language settles it

`ViewWith` cannot be a method.

```go
func (v MapSetView[T]) ViewWith[NT any](vw KeyViewer[T, NT]) MapSetView[NT]
// syntax error: method must have no type parameters
```

A converting view introduces `NT`, which is not on the receiver and cannot be
inferred from it. ADR `0002` already recorded that methods cannot have type
parameters; this is that rule biting. **`View()` works as a method only because
identity means `NT == T`** — there is no new parameter to introduce. That
asymmetry is not an inconsistency to fix, it is the shape of the language.

So the `View*` functions stay functions. Three escapes were considered:

- **One method per target type** (`ViewWithStrings`, …). Absurd, and unbounded.
- **Parameterise the view by its viewer**, putting `NT` on the receiver. That is
  ADR `0020`'s carried witness, **abandoned**: it costs the zero-value rule,
  because a view would then have to be an interface or carry the viewer's type
  in every signature.
- **A method on the *viewer*** — this one is legal, and worth recording because
  it is not obvious:

  ```go
  func (b byName) Apply(v MapSetView[*Item]) MapSetView[string]   // compiles
  ```

  The viewer's concrete type knows both `T` and `NT`, so no method type parameter
  is needed. **Rejected anyway:** every viewer author would hand-write `Apply`,
  it cannot be supplied generically (a generic function cannot be a method, and an
  embedded helper cannot name its outer type), and a hand-written `Apply` is a
  place to get the wiring wrong. A free function is written once in the library.

## Thread 1: where the source parameter should point

Two shapes, both composable. Measured at n=64:

| | construct | iterate |
|---|---|---|
| baseline — source is the **container** (today; not composable) | **1** alloc | 537 ns / 6 |
| **A** — source is the **concrete view type** | **1** alloc | 539 ns / 6 |
| **B** — source is the **sealed shape interface** | **2** allocs | 538 ns / 6 |

```go
// A
func ViewMapSet[T, NT any](v MapSetView[T], viewer CanViewMapSet[T, NT]) MapSetView[NT]
// B
func ViewMapSet[T, NT any](k Keys[T], viewer CanViewMapSet[T, NT]) MapSetView[NT]
```

**Taking a view as the source costs nothing per element.** 537 / 539 / 538 ns is
one number written three times. The extra dynamic call a view source introduces
is paid **once per `Keys()` call**, obtaining the iterator, not once per element —
the same per-call-not-per-element shape as every other dispatch result in this
repo. I expected a per-element penalty and there is none.

**A is recommended.** Same walk cost as B, and B must box a two-word view to
enter the sealed interface, so it construct-allocates twice where A allocates
once. B's compensation is that it accepts *any* key-exposing view, whatever
container it came from — but that is a reason to reject it, not to prefer it:
re-viewing a `SortedSetView` through a `MapSetView` constructor would silently
discard the ordering guarantee its type was carrying. **Per-kind constructors
preserve the kind**, which is what ADR `0018`'s naming scheme is for.

### Composition depth

| depth | iterate | delta |
|---|---|---|
| 1 | 539 ns / 6 | — |
| 2 | 697 ns / 9 | +160 ns, +3 allocs |
| 3 | 874 ns / 12 | +177 ns, +3 allocs |

**Linear, ~160 ns and 3 allocations per layer, per call.** That is the nested
range-over-func machinery: each additional `for range` in the chain costs its own
closure plus range state and yield closure. So depth is cheap to add and cheap to
walk, but a deeply composed view is a worse hot-loop candidate than a flat one —
and `Each` (ADR `0021`) removes the per-layer allocations for exactly the reason
it removes the flat ones.

### What it costs callers

Every converting call site gains one `.View()`:

```go
// before
containers.ViewMapSet(s, viewer)
// after
containers.ViewMapSet(s.View(), viewer)
```

`View()` is free — 0.39 ns, 0 allocations (ADR `0022`) — so this is spelling, not
cost. And it is *consistent* with the seal: after `0022`, handing a container
anywhere read-only already means writing `.View()`.

## What composition means per view kind

| view | composes | notes |
|---|---|---|
| `MapSetView[NT]` | keys, both directions | `FromKeyView` must chain in reverse and may fail at either stage |
| `MapView[NK, NV]` | keys both ways, values outbound | |
| `SortedMapView[K, NV]` | **values only** | keys are `cmp.Ordered` and pass through (ADR `0012` §3) |
| `VectorView[NT]` | elements outbound | **and gains live growth tracking** — see problem 3 |
| `SortedSetView[T]` | **not at all** | nothing to convert; keys are `cmp.Ordered` and converting them reopens order preservation (`0012` §3) |

`SortedSetView` having no composing form is not an inconsistency to paper over:
it has no converting constructor today for a decided reason.

## Call sites

**Sketch — the layering this exists for:**

```go
// a registry package hands out a view keyed by name
func (r *Registry) ByName() containers.MapSetView[string] {
	return containers.ViewMapSet(r.items.View(), r.nameViewer())
}

// its caller narrows further for ITS callers, without reaching the container
func Shortened(v containers.MapSetView[string]) containers.MapSetView[string] {
	return containers.ViewMapSet(v, firstEight{})
}
```

Today the second function cannot be written at all.

**Sketch — the `Vector` defect, as a regression test:**

```go
v := containers.NewVector(1, 2)
conv := containers.ViewVector(v.View(), dbl{})
v.Append(3)
if conv.Len() != 3 {
	t.Error("a converting view snapshotted instead of tracking growth")
}
```

That test **fails today** and passes under this ADR. It belongs in
`callsites_test.go` either way, since the current behaviour contradicts
`View`'s doc comment and ADR `0015`.

## What to decide

1. **Option A or B for the source parameter.** A is recommended: one fewer
   allocation, and it keeps a view's kind in its type.
2. **Whether the `Vector` growth fix ships as part of this or separately.** It is
   a behaviour change to `ViewVector` — a converting view would start seeing
   appends. Anyone relying on the snapshot is relying on undocumented behaviour
   that contradicts `View`'s own doc comment, but it is still a change.
3. **Whether the library ships viewer composition too.** `composedValue` above
   works and is small; the question is whether it is worth the surface once view
   composition exists, since the two overlap for everyone who owns their
   container. Recommendation: no — one mechanism, and views are the one callers
   are asking for.
4. **Whether `ViewSlice` keeps a slice source.** A `[]T` has no view and no
   `View()` method, so `ViewSlice(s, viewer)` cannot take a view. It stays the
   one constructor with a non-view source, alongside `ViewSliceIdentity`.
5. **Whether composing constructors keep the same names.** `ViewMapSet(v, …)`
   reads fine for both a fresh view and a re-view, so no new names appear to be
   needed — but a reader seeing `ViewMapSet(x.View(), …)` may wonder why the
   `View()` is there, which is a doc-comment problem rather than a naming one.
