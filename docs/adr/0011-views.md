# 11. How a read-only view is expressed

- **Status:** Accepted. Not yet implemented, so this constrains the change
  rather than describing it.
- **Date:** 2026-09-07
- **Evidence:** `experiments/views/` (`RESULTS.md`), with context from
  `experiments/copycost/`, `dispatch/` and `hashdict/`.
- **Relates to:** ADR `0001`, which accepted that views are justified and
  explicitly deferred how one is expressed; `0002` (methods cannot have type
  parameters); `0008` (the contract layers a view must satisfy).

## Context

ADR `0001` decided **when** a view is justified — access crosses an API boundary
and is called many times — and deliberately did not decide **what a view is**:

> Deliberately not decided here: how a view is actually expressed — interface
> versus struct with an unexported field, and what if anything enforces
> read-only-ness against a determined caller.

Two problems hide under that, and they are independent.

**Expressiveness.** Handing out `Set[T]` or `Dict[K, V]` gives read-only access
but loses everything type-specific: `Min`, `Max`, `Floor`, `Ceil`, `Range`. ADR
`0008` already records this as the contracts having no ordering axis.

**Enforcement.** A bare contract prevents nothing. Containers structurally
satisfy the read contracts, so a holder asserts back and mutates:

```
bare contract, type assert to container:  SUCCEEDED (Len now 2)
view, type assert to container:           false
```

This ADR is about the second. The first is a contract-design question that can be
settled separately by adding an ordered tier.

## Findings that apply to every option

These came out of `experiments/views/` and constrain all three alternatives.

### Sealing is forgettable, not defeatable

The escape hatches are tiers, not equivalents:

| path | outcome | visibility |
|---|---|---|
| type assertion to the container | blocked by any view | invisible in review |
| `reflect` read of the unexported field | succeeds | needs `reflect` |
| `reflect` **write** through it | **blocked by reflect itself** | — |
| `reflect` + `unsafe` | succeeds | requires importing `unsafe` |

Plain reflection cannot mutate — `reflect.Value.Interface` refuses values from
unexported fields — so reaching the container needs `unsafe`, which is greppable,
lintable and conventionally reviewed. **A view moves the bar from invisible to
auditable, not from possible to impossible.**

The sharper limitation is different: containers must satisfy `Elems2` for the
sized constructors, so they satisfy the read contracts too, and a provider can
always pass the container directly. **The seal engages only where someone
remembered it** — unlike `noCopy`, which vet applies everywhere automatically.

### Pointer-shaped or not is the whole allocation story

A view struct holding **one concrete pointer** boxes into a contract interface
for free. One holding an interface or a `func` value is two words, is not
pointer-shaped, and **allocates every time it is passed as a contract** — which
is the common case. This single fact separates the options more than anything
else does.

### A method cannot introduce the projected type

`func (d *HashDict[K, V]) View[R any](f func(V) R)` is illegal:
`method must have no type parameters`. Any projection that changes the element
type must come from a free function or a type parameter on the view. This is the
same ceiling ADR `0002` recorded for set algebra and `0004` for
`CollectSortedDict`.

### Projection is never a snapshot

A projection wraps the same element. `ItemView` over a `*Item` still reports a
later mutation. It denies writes **through the view**; it does not freeze
anything. Deep immutability means copying, which ADR `0001` measured and
rejected.

### The library cannot supply the projection

For `V = *Item` someone must write `ItemView`; for `V = []byte`, a read-only
bytes type. The container can offer a slot for a projection; it cannot invent the
read-only type. **Strong view semantics are composed from an element-level type
the caller provides**, not achieved by the container.

## Decision

**A view is a struct holding a pointer to its container and a projection
function, constructed by a free function.** Option B below.

```go
type SortedDictView[K cmp.Ordered, V, R any] struct {
	d *SortedDict[K, V]
	f func(V) R
}

func ViewSortedDict[K cmp.Ordered, V, R any](
	d *SortedDict[K, V], f func(V) R,
) SortedDictView[K, V, R]
```

Three reasons, in order of weight.

**It is the most general.** It is the only option that admits a stateful
converter, and a projection that can close over configuration is a capability the
other two foreclose permanently rather than merely defer.

**Its cost is small against what it replaces.** A projecting view costs 5.94 ns
per access and one allocation when passed as a contract. The defensive copy ADR
`0001` rejected costs **10 ns minimum, grows at ~0.3–1 ns per element, and
degrades quadratically** when an accessor is called in a loop — 36 800x a view at
n=16 384. Being ~1.5 ns slower than the cheapest view is not the comparison that
matters; being orders of magnitude cheaper than a copy is.

**It forecloses nothing.** Options A and C remain available later under their own
names, for containers or call sites where the measurements say the difference
earns the extra shape. This ADR sets the default, not the only permitted form.

**The view is a struct, not an interface.** Returning a concrete type is what
blocks the type assertion back to the container, and it keeps the forwarding
methods inlinable. A view still *satisfies* the read contracts, so it composes
with generic code; it is simply not handed out as one.

`*View` is the naming pattern for these types.

**Every container gets one.** Not only those with a boundary in sight today:
any of them may be handed across one sooner or later, and a container missing a
view is a hole a caller discovers at exactly the wrong moment. A consistent
pattern is also cheaper to teach than a rule about which containers qualify.

The cost is bounded and mechanical — a struct, a free function, a `View` method,
and one forwarding method per read operation — so there is no size at which this
stops being worth it.

## Alternatives considered

Both remain viable and could be added later alongside option B. They are recorded
in full because the measurements that separate them are not obvious.

### Option A — shallow view

A struct holding a concrete pointer, one per container, returned by a method.

```go
type SortedDictView[K cmp.Ordered, V any] struct{ d *SortedDict[K, V] }
func (d *SortedDict[K, V]) View() SortedDictView[K, V]
```

**For**

- **Free.** 8 bytes, `Get` at 4.38 ns against 4.44 ns calling the container
  directly — the forwarding inlines away entirely. Boxing 0.38 ns, zero allocs.
- Simplest possible thing that blocks the type assertion.
- One method, no type arguments at any call site.
- Satisfies the read contracts, so it composes with generic code.

**Against**

- **Does not stop element mutation.** For `V = *Item` a caller mutates freely:
  `shallow view: element mutated through it? true`.
- Type-specific reads need a view type per container regardless, so the method
  count grows with containers × read methods.
- Provides no path to stronger semantics later without adding a second shape.

### Option B — projection carried as a field (**chosen**)

The conversion is a `func` stored in the view, supplied by a free function.

```go
type DictProjView[K cmp.Ordered, V, R any] struct {
	d *SortedDict[K, V]
	f func(V) R
}
func ViewFunc[K cmp.Ordered, V, R any](d *SortedDict[K, V], f func(V) R) DictProjView[K, V, R]
```

**For**

- **Closes the element-mutation hole**, given a caller-supplied read-only type.
- **Ordinary Go.** A closure is a familiar thing to pass; nothing exotic.
- **Converters may hold state**, since the function is a value — a projection can
  close over configuration, policy, or anything else.
- Type inference works from the arguments: `ViewFunc(d, f)` infers `K`, `V`, `R`.

**Against**

- **Not pointer-shaped: 16 bytes, and 12.13 ns plus one allocation every time it
  is passed as a contract.** That is ~32x the boxing cost of the shallow form,
  and it recurs at every boundary crossing.
- `Get` costs 5.94 ns against 4.38 ns shallow.
- Cannot be a method, so construction reads `ViewFunc(d, f)` rather than
  `d.View(f)` — an asymmetry with every other accessor in the package.
- The identity case still pays 16 bytes unless a second, shallow view type is
  kept alongside it.

### Option C — projection carried as a type parameter

The converter is a type, not a value. The view materialises its zero value per
call, so the struct stays a single pointer.

```go
type Converter[V, R any] interface{ Convert(V) R }

type DictView[K cmp.Ordered, V, R any, C Converter[V, R]] struct {
	d *SortedDict[K, V]
}

// generic alias, partially applied — the shallow case
type SortedDictView[K cmp.Ordered, V any] = DictView[K, V, V, idConv[V]]
func (d *SortedDict[K, V]) View() SortedDictView[K, V]

// fully instantiated alias, naming a projecting view once
type ItemDictView = DictView[string, *Item, ItemView, itemConv]
```

**For**

- **Pointer-shaped: 8 bytes, boxing 0.39 ns with zero allocations**, matching the
  shallow form and unlike option B.
- **One type covers both cases.** An identity converter gives the shallow view;
  no second type is needed.
- **Aliases recover the ergonomics.** A generic alias may be returned from a
  method, so `d.View()` takes no type arguments; a fully instantiated alias names
  a projecting view once. Alias and long form are interchangeable in both
  directions, since aliases are identical types.

**Against**

- **Converters must be stateless.** The zero value is materialised per call, so a
  projection can never be configured. This is permanent, not a detail.
- **Four type arguments, and inference cannot help** — `R` and `C` appear in no
  argument. Without an alias every construction site spells all four.
- `Get` costs 5.78 ns, essentially the same as option B: **the witness buys the
  width and the allocation, not the call.** An identity witness costs 5.72 ns, so
  unifying shallow and projecting under one type is not free either — it makes
  the shallow case ~30% slower than option A.
- A converter type per projection is more declaration than a closure literal.
- The idiom is unusual in Go; a reader meeting it for the first time has to work
  out what `var conv C` is doing.

## Side by side

| | A: shallow | B: field | C: witness |
|---|---|---|---|
| size | **8 B** | 16 B | **8 B** |
| `Get` | **4.38 ns** | 5.94 ns | 5.78 ns |
| boxing as a contract | **0.38 ns, 0 alloc** | 12.13 ns, **1 alloc** | **0.39 ns, 0 alloc** |
| blocks container mutation | yes | yes | yes |
| blocks element mutation | **no** | yes | yes |
| stateful converters | — | **yes** | **no** |
| constructed by | method | free function | method (via alias) |
| type args at a call site | none | inferred | four, or none via alias |
| shallow case cost | free | 16 B unless a 2nd type | 5.72 ns |

Direct container call, for reference: 4.44 ns. Zero-cost baseline: 0.29 ns.

## Context from earlier measurements

View costs are small and are best read against what they replace.

| measurement | source |
|---|---|
| defensive copy floor ~10 ns, then ~0.3–1 ns per element | `copycost` |
| a copying accessor called n times is O(n²) — 36 800x a view at n=16 384, 2.1 GB in one pass | `copycost` |
| retained pointer-ful copies re-scanned every GC cycle, 2.4x cycle cost | `copycost` |
| interface dispatch +0.72 ns; generic type parameters indistinguishable | `dispatch` |
| an extra map lookup +2.94 ns, ~4x what dispatch costs | `dispatch` |
| map `Get` 3.2–3.4 ns | `hashdict` |
| a method wrapper over a builtin inlines away entirely | `dispatch`, `hashdict` |

**The scale worth holding onto:** projection costs ~1.4 ns per access; the copying
alternative ADR `0001` rejected costs 10 ns minimum and degrades quadratically.
All three options are cheap relative to the problem they address.

## Consequences

- **Every view costs an allocation when passed as a contract interface** — 12.13 ns
  and one alloc, against 0.38 ns and none for a pointer-shaped view. That is the
  standing price of the choice, and it lands at every boundary crossing. It is
  also ~1/800th of the quadratic hazard it replaces.
- **The shallow case pays for machinery it does not use.** A view over a value
  type still carries a function field and still boxes with an allocation. If that
  becomes a measured problem, option A is the answer and can be added.
- **Views are constructed by free functions, not methods**, because a method
  cannot introduce the projected type. That is an asymmetry with every other
  accessor in this package, and it is forced by the language rather than chosen.
- **The caller supplies the projection.** For `V = *Item` somebody has to write
  `ItemView`. The container offers the slot; it cannot invent the read-only type.
  A view over a mutable element type is shallow unless the caller closes it.
- **A projection is not a snapshot.** It denies writes through the view. The
  underlying element can still change beneath it, exactly as ADR `0001` recorded
  for the container itself.
- **A new container is not finished until it has a view.** This is now part of
  the shape a container must have, alongside ADR `0002`'s rules. `LinkedList`
  (ADR `0010`) will need one when it is implemented; its `All` yields cursors,
  which are handles rather than values, so a view over it hands out cursors that
  are inert without the list they came from.
- **Sealing stays forgettable.** Containers satisfy the read contracts inherently,
  so a provider can always pass the container instead of a view. The view is a
  tool the provider applies at boundaries they care about, not a guarantee the
  type system enforces.
- **Projecting set views do not satisfy `Set[R]`.** `Has` takes the real element
  type while `All` yields the projected one, so a projecting set view satisfies
  `Elems[R]` only. Identity views satisfy the full contract. Dict views are
  unaffected, since only the value side is projected.

## Follow-ups

- An ordered tier for the contracts — `OrderedSet`, `OrderedDict` — which is the
  expressiveness half of the problem and is already a follow-up in ADR `0008`.
- Naming: `View` as the method, and what the view types are called.
