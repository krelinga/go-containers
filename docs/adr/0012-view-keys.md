# 12. Converting keys through a view

- **Status:** Accepted. Not yet implemented, so this constrains the change
  rather than describing it.
- **Date:** 2026-09-07
- **Evidence:** `experiments/viewkeys/` (`RESULTS.md`)
- **Relates to:** ADR `0011` (views project values only — this closes the
  follow-up it recorded), `0008` (the contract layers, whose constraints this
  relaxes), `0003` (whose `SortedDictFunc` follow-up would reopen decision 3).
- **Supersedes:** ADR `0011`'s decision that every container has a `View` method
  returning the identity view. See decision 5. ADR `0011`'s rule that **every
  container must have a view** survives unchanged; only its shape changes.

## Context

ADR `0011` shipped views that project **values**. Keys pass through untouched,
which is unsound for the hash-backed containers: `comparable` admits pointer
types, so `HashDict[*Item, string]` is legal and its view hands back the raw
`*Item` for the holder to mutate.

The stronger argument is not the leak but the **API surface**:

| view | satisfies |
|---|---|
| value-only, over a `*Item` key | `Dict[*Item, ItemView]` |
| key-converting | `Dict[string, ItemView]` |

The raw mutable type appears in the contract's own signature. Every function
accepting a value-only view over a pointer key has `*Item` in its parameter list,
so this is not only about what `All` yields — it is about what the type
*advertises*. Converting keys removes the raw type from the API entirely.

Converting keys also repairs something `0011` recorded as a loss. A projecting
set view whose `Has` takes the raw element type while `All` yields the projected
one satisfies `Elems[NT]` but not `Set[NT]`, because the two halves disagree
about which type they speak. Converting inbound makes them agree.

## Decision

### 1. Keys convert in both directions; the inbound direction may fail

Outbound conversion is total: every key in the container maps to a projected key.
Inbound is not — a projected key need not correspond to a real one — so it
returns a success flag. **A failed `FromKeyView` reads as a miss**, not a panic
and not a wrong answer, so that `Get` on a view behaves exactly as `Get` on the
container does for an absent key.

The flag exists primarily so the *viewer author* never has to fabricate a key. A
total `func(NK) K` would leave them choosing between panicking — which makes
panic policy a per-viewer decision — and returning a zero `K` that then looks up
something unintended.

Values convert outbound only. A view is read-only, so no value travels inward.

### 2. Two families of interface: capabilities and requirements

**Capabilities** describe what a viewer can do. A viewer author implements these
and never names anything else.

```go
type KeyViewer[K, NK any] interface {
	ToKeyView(K) NK
	FromKeyView(NK) (K, bool)
}

type ValueViewer[V, NV any] interface {
	ToValueView(V) NV
}
```

**Requirements** describe what one operation needs. There is one per constructor
that needs a viewer, owned by that constructor, so it can evolve with it.

```go
type CanViewHashSet[T, NT any]           interface{ KeyViewer[T, NT] }
type CanViewHashDict[K, NK, V, NV any]   interface { KeyViewer[K, NK]; ValueViewer[V, NV] }
type CanViewMap[K, NK, V, NV any]        interface { KeyViewer[K, NK]; ValueViewer[V, NV] }
type CanViewSortedDict[V, NV any]        interface{ ValueViewer[V, NV] }
```

The split is worth the extra names because **the error message names the
operation**, which is the actionable thing — "you cannot `ViewSortedDict` with
this viewer" rather than "you are not an `OrderedKeyValueViewer`". A combined
interface would name a structural property the caller failed to have; an
anonymous interface in the signature would print the whole literal in every error
and every godoc entry.

Some requirements are redundant today — `CanViewHashSet` is exactly `KeyViewer`,
and `CanViewHashDict` and `CanViewMap` are identical. That is accepted: they name
different requirements that happen to coincide now and may diverge, and the
uniformity is worth more than the saved declarations.

There is no `KeyValueViewer`. Capabilities stay atomic; composition happens in
the requirement interfaces, and in the caller's own viewer:

```go
type releaseKeys struct{}   // ToKeyView, FromKeyView
type releaseValues struct{} // ToValueView

type releaseViewer struct { // satisfies CanViewHashDict
	releaseKeys
	releaseValues
}
```

A stateless composed viewer is **0 bytes**.

### 3. Ordered containers convert values only

`SortedSet` and `SortedDict` constrain their key to `cmp.Ordered`, which admits
only integers, floats and strings — and defined types over them. **Every one is
an immutable value type with no reachable interior**, so a key handed out by
`All` is a copy of something that could not have been mutated anyway. A pointer
is not `cmp.Ordered`; neither is a struct.

There is therefore nothing for key conversion to protect on an ordered container,
and offering it would buy nothing while costing a great deal:

- `Range`, `Floor` and `Ceil` take **`K` directly**, so there is no inbound
  conversion to fail and no question of what a failure means for a bound.
- **No order-preservation apparatus is needed.** A key conversion that is not
  monotonic does not merely reorder results — it changes *which entries are
  returned*, since bounds converted into `K` space select a different set than
  the caller asked for in `NK` space. Guarding against that would require an
  annotation, a marker interface, and a rule that the type system can check the
  presence of but never the truth of.

`SortedSet` needs no viewer at all: its elements are its keys, so a `SortedSetView`
is a plain sealed handle at **8 bytes** — ADR `0011`'s cheapest shape, arrived at
because there is genuinely nothing to convert.

The cost is real and worth stating: **an ordered key can never be re-presented as
another type.** `SortedDict[int64, Event]` keyed by unix timestamps cannot offer
a view whose keys are `time.Time`, even though that conversion is monotonic and
`time.Time` cannot be the container's key type. If that case arrives with a
motivating example, an ordered-key viewer plus its annotation is a strictly
additive change to this design.

### 4. A view carries one viewer, as an interface value

```go
type HashDictView[K comparable, V, NK, NV any] struct {
	d      *HashDict[K, V]
	viewer CanViewHashDict[K, NK, V, NV]
}
```

**One interface value, not function fields.** `experiments/viewkeys/` measured
the difference: **24 bytes against 32**, at identical call cost, because a `func`
field is already an indirect call and an itab lookup adds nothing measurable. An
interface field also composes by embedding, which a struct of functions does not.

### 5. No `View` method; every view is constructed explicitly

ADR `0011` gave each container a `View()` method returning the identity view.
**That is withdrawn.** `View()` reads as "give me a view" with no hint that key
and value handling is a dimension at all, which is exactly where a caller stops
thinking.

Constructors come in pairs, except `SortedSet` which has nothing to configure:

```go
func ViewHashDict[K comparable, V, NK, NV any](
	d *HashDict[K, V], viewer CanViewHashDict[K, NK, V, NV],
) HashDictView[K, V, NK, NV]

func ViewHashDictIdentity[K comparable, V any](
	d *HashDict[K, V],
) HashDictView[K, V, K, V]

func ViewSortedSet[T cmp.Ordered](s *SortedSet[T]) SortedSetView[T]
```

The identity form is exactly as terse as `View()` was — **zero type arguments at
the call site**, since `K` and `V` come from the container and `NK`/`NV` are fixed
in the return type — but it is *named*. A caller writing `Identity` has stated
that identity is the choice, rather than accepting a default they never saw.

Type inference works for the general form too: `ViewHashDict(d, itemViewer{})`
needs no type arguments either.

### 6. The contracts relax from `comparable` to `any`

`Set`, `Dict`, `MutableSet` and `MutableDict` constrain their key or element
parameter to `comparable`. A converted key need not be comparable, so a
converting view could not satisfy them.

Relaxing all four was tried against the real package: **it compiles and the
entire test suite passes with no other changes.** The constraint was never
load-bearing at the interface — only implementations need comparable keys, and
they declare that themselves in their own type parameters.

It was also never a good guarantee to make. A future container keyed by a
comparison function, or by a type compared structurally, would have been excluded
by a constraint that bought the contracts nothing.

## Consequences

- **Hash and ordered views have different shapes.** A hash view converts keys and
  values; an ordered dict view converts values; an ordered set view converts
  nothing. That asymmetry tracks a real difference in the containers rather than
  an inconsistency, but it is one more thing to know.
- **Hash views widen from 16 bytes to 24**, and a lookup costs ~24% more:
  ~6.5 ns against 5.4 ns, with iteration up ~10% since it pays only the outbound
  conversion. `SortedSetView` moves the other way, to 8 bytes.
- **Nine view constructors and four viewer interfaces.** The count is up on ADR
  `0011`; what it buys is that each name has one job and error messages name the
  operation.
- **Projecting set views satisfy `Set[NT]` again**, which ADR `0011` recorded as
  lost. A gain this ADR delivers rather than a cost.
- **Callers write a type with methods, not a closure.** More ceremony for a
  one-off projection, less for a reused one: a named viewer is self-documenting
  at the call site and shared across every view over that key type.
- **The library still cannot supply the projection.** For `K = *Item` somebody
  writes the read-only key type and both directions of its conversion. ADR
  `0011`'s finding that strong view semantics are composed from caller-supplied
  types applies to keys as it did to values.
- **`comparable` is gone from the contracts**, so a type satisfying `Dict[K, V]`
  no longer promises its keys can be compared with `==`. Nothing in the package
  relied on that; generic code written against the contracts must not either.
- **Nothing in this design can produce a silent wrong answer.** Decision 3
  removed the only mechanism that could.

## Rejected alternatives

- **Carrying the conversions as function fields**, either three loose ones or a
  struct bundling them. This was the shape this ADR was first drafted around.
  Rejected on measurement and on structure: three func fields are **32 bytes
  against the interface's 24** at identical call cost, they cannot be composed by
  embedding, and separating the two key directions invites a mismatched pair that
  nothing detects.
- **Key conversion on ordered containers, guarded by an annotation.** A marker
  interface with an unexported method, embedded by a `PreservesKeyOrder` struct,
  so only viewers declaring monotonicity could be used with a sorted container.
  It works — the seal is real, and a same-named method in another package cannot
  forge it. Rejected because `cmp.Ordered` makes the capability unnecessary: it
  would have added an interface, an annotation, a padding rule, and an unresolved
  question about failed bound conversion, all to serve a use case with no
  motivating example. Recorded because decision 3's premise may not hold forever
  — see follow-ups.
- **A single combined interface** such as `OrderedKeyValueViewer`, with no
  requirement family. Fewer names, but the error names a structural property
  rather than the operation the caller attempted, and the interface belongs to
  nothing in particular, so it cannot evolve with any one constructor.
- **An anonymous interface in each constructor's signature.** Saves the
  declarations and spends it at every signature, every godoc entry, and every
  error message, which then prints the whole literal.
- **Outbound key conversion only.** Simpler, one function per key type, and it
  stops `All` from yielding raw keys. Rejected because it leaves the raw type in
  the contract the view satisfies, so the API surface still names it — and
  because it leaves projecting set views unable to satisfy `Set[NT]`.
- **A total inbound conversion, `func(NK) K`.** Rejected because it moves the
  impossible case onto the viewer author, who can then only panic or fabricate.
- **Keeping the `View()` identity methods** alongside the explicit constructors.
  Rejected because the shortcut is precisely where a caller stops thinking about
  key handling, and `ViewHashDictIdentity` costs the same keystrokes while
  stating the choice.
- **Constraining keys to immutable types.** Not expressible: Go has no way to say
  "this type contains no pointers", and `comparable` explicitly does not mean it.

## A future possibility, deliberately not foreclosed

The same interfaces can be carried as a **type parameter** rather than a field,
with the view materialising the viewer's zero value per call:

```go
type HashDictView[K comparable, V, NK, NV any, W CanViewHashDict[K, NK, V, NV]] struct {
	d *HashDict[K, V]
}
```

`experiments/viewkeys/` measured it at **8 bytes with allocation-free boxing** —
0.37 ns against 13.24 ns — and marginally faster calls.

**The interface definitions are identical in both forms.** Only where the viewer
lives changes, so a viewer written today works unaltered under either. That is
the reason to adopt the interface vocabulary now rather than function fields: it
keeps the door open at no cost.

It is **not** a free migration, and this ADR does not commit to it. Materialising
the zero value **forecloses a stateful viewer**, which is the capability ADR
`0011` chose the field form to keep, and it adds a type parameter that inference
cannot supply, recovered only by generic aliases.

## Follow-ups

- **Sorting by a comparison function would reopen decision 3.** ADR `0003`
  records `SortedDictFunc` as a follow-up: a sorted container ordered by a
  supplied comparator rather than `<`. Such a container could be keyed by **any**
  type, including pointers and structs — which reinstates both the mutable-key
  hazard and the order-preservation question decision 3 dodges. The annotation
  design in Rejected alternatives is the answer if that day comes; it should be
  read before, not reinvented after.
- Whether the package should ship more prebuilt viewers than `IdentityViewer` and
  `IdentityValueViewer` — one for string-keyed dicts, say.
- Views that compose, so a view of a view applies both viewers. Still open from
  ADR `0011`.
- A `Range` returning a restricted view rather than an iterator. Still open from
  ADR `0011`. Carrying bounds widens a view considerably — 24 to 64 bytes for a
  string key — which argues for a separate type.
- Whether `LinkedList` (ADR `0010`) needs a viewer family of its own, since its
  keys are cursors rather than values.
