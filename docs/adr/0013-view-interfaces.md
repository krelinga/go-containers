# 13. Interfaces for view types

- **Status:** Accepted. Not yet implemented, so this constrains the change
  rather than describing it.
- **Date:** 2026-09-08
- **Evidence:** `experiments/viewiface/` (`RESULTS.md`); background numbers from
  `experiments/views/` and `experiments/dispatch/`.
- **Relates to:** ADR `0011` (which chose concrete structs, and recorded
  "sealing stays forgettable" as an unresolved limitation this would close),
  `0012` (whose four-type-argument views are what prompted this), `0008`
  (naming: contracts take the bare concept, implementations `<Ordering><Concept>`).
- **Supersedes:** ADR `0011`'s decision that a view is handed out as a concrete
  struct, and ADR `0008`'s read-only contract tier. See decisions 1 and 2. ADR
  `0011`'s rule that **every container must have a view**, and ADR `0012`'s
  viewer vocabulary, both survive unchanged.

## Context

ADR `0012` left every view a concrete struct carrying its viewer in a field. A
consumer's signature therefore reads:

```go
// sketch
func render(v containers.HashDictView[*Item, *Item, string, ItemView]) error
```

Four type arguments, and **two of them name types the consumer never mentions
again**. `*Item, *Item` is the container's own key and value; `render` speaks
only `string` and `ItemView`. They appear because the view holds a
`*HashDict[K, V]`, and a struct cannot hide the type arguments of its own field.

Two costs follow, one cosmetic and one not.

**Spelling.** The type is long, and the long part is the part that carries no
information for the reader.

**Coupling.** The signature names `HashDict`. A provider that switches to
`SortedDict` breaks every consumer, even though nothing a consumer can observe
changed. The implementation is in the consumer's API by construction.

There is a third thing, which `0011` wrote down and did not solve:

> **Sealing stays forgettable.** Containers satisfy the read contracts
> inherently, so a provider can always pass the container instead of a view.

A concrete view blocks a type assertion, but nothing stops the provider handing
over `*HashDict` in the first place, since it satisfies `Dict[K, V]` structurally.
The seal engages only where someone remembered it.

## Findings that apply to every option

From `experiments/viewiface/`.

### A sealed interface is a stronger boundary than a concrete struct

`0011` rejected exposing views as *bare contracts*. A **sealed** interface — one
carrying an unexported method that only view structs have — is a different
proposition, and it closes the hole one stage earlier than a concrete struct
does. The container cannot be stored in it at all:

```
cannot use NewDict[*Item, *Item]() (value of type *Dict[*Item, *Item]) as
DictView[*Item, *Item] value in variable declaration: *Dict[*Item, *Item]
does not implement DictView[*Item, *Item] (missing method sealedView)
```

That is a compile error where today there is a convention. Measured against the
bare contract, which accepts the container and hands back a mutable pointer:

```
container satisfies a bare contract:      true
bare contract asserts back to container:  true
container unchanged after the assert:     false
container satisfies the SEALED interface: false
sealed view asserts back to container:    false
```

The scope of the improvement is worth stating precisely: **forgetting becomes a
compile error only where the boundary is declared with the sealed type.** A
function still declared `func render(d containers.Dict[K, V])` accepts the
container as happily as ever. This does not make views unforgettable; it moves
the remembering from every call site to one signature, where it is checked.

### Boxing is linear in boundary crossings today, and constant behind an interface

A view built once and passed across *n* boundaries:

| crossings | concrete struct (today) | sealed interface | pointer-shaped struct |
|---|---|---|---|
| 1 | 32.05 ns, **1 alloc** | 32.04 ns, 1 alloc | 29.98 ns, 1 alloc |
| 2 | 62.99 ns, **2 allocs** | 46.33 ns, 1 alloc | 44.09 ns, 1 alloc |
| 4 | 125.5 ns, **4 allocs** | 75.69 ns, 1 alloc | 72.63 ns, 1 alloc |
| 8 | 248.2 ns, **8 allocs** | 140.1 ns, 1 alloc | 135.3 ns, 1 alloc |

`0012`'s view is 24 bytes — a container pointer plus a two-word viewer — so it
is not pointer-shaped and re-boxes at every crossing. An interface boxes once.

Marginally, one more crossing costs **15.4 ns** through an interface and
**30.9 ns** through the concrete struct: **boxing roughly doubles the cost of a
boundary crossing.** Crossover is at two crossings; at one they are
indistinguishable, because both pay exactly one box.

So the answer to "how much would an interface cost in boxing" is **negative**.
Dispatch is ~1 ns; the allocation it removes is ~16 ns. At a boundary the
interface is never slower than what `0012` has today.

### Escape analysis rescues the concrete form only where a view is pointless

| | |
|---|---|
| consumer not inlinable | 29.17 ns, **1 alloc** |
| consumer inlinable | 11.76 ns, **0 allocs** |
| direct call, no interface at all | 11.85 ns, 0 allocs |

When the consumer inlines, the compiler devirtualises the call and needs no box.
That rescue is gone as soon as the callee is in another package and too large to
inline — which describes an API boundary.

The mechanism is narrower than "it escaped", and the next finding pins it down:
non-inlinability alone does not allocate. **The concrete view is free exactly
where nobody needs a view, and allocates exactly where someone does.**

### Passing is free at every width; dispatching through a box is what costs

Width is not the expense. With every value built once and the callee ignoring
its argument, one word, two words and three words are indistinguishable from a
call taking no argument at all — they ride in registers:

| argument, callee ignores it | | allocs |
|---|---|---|
| nothing (the floor) | 0.899 ns | 0 |
| 1-word struct, concrete parameter | 0.935 ns | 0 |
| 2-word interface, interface parameter | 0.905 ns | 0 |
| 3-word struct, concrete parameter | 0.908 ns | 0 |
| 3-word struct **into an interface parameter** | 1.072 ns | **0** |
| interface into a *different* interface parameter | 1.345 ns | 0 |

Boxing a 3-word struct allocates nothing here. Now have the callee call one
method through the parameter, which is what a boundary does:

| argument, callee calls `Get` once | | allocs |
|---|---|---|
| 1-word struct, concrete parameter | 12.33 ns | 0 |
| 3-word struct, concrete parameter | 12.67 ns | 0 |
| 1-word struct into an interface parameter | 13.02 ns | 0 |
| 2-word interface, interface parameter | 13.61 ns | 0 |
| interface into a *different* interface parameter | 13.83 ns | 0 |
| **3-word struct into an interface parameter** | **29.05 ns** | **1** |

So passing an interface costs ~1.3 ns over passing a concrete struct — that is
dispatch, and it is the whole of it. The 16 ns is allocating a non-pointer-shaped
receiver so a method can be dispatched through it: the itab takes the receiver as
one pointer, the dynamic target is opaque, so a 3-word value must be copied to
the heap. A 1-word receiver needs no copy, because it *is* the pointer.

Interface-to-interface conversion — a `DictView` passed to a parameter typed
`Dict` — costs ~0.45 ns and no allocation.

### Only an interface can amortise the box

A view built once and reused, against one rebuilt per call:

| | | allocs |
|---|---|---|
| interface, rebuilt each call | 30.33 ns | 1 |
| **interface, built once and reused** | **13.42 ns** | **0** |
| 3-word struct, rebuilt each call | 30.01 ns | 1 |
| **3-word struct, built once and reused** | **28.99 ns** | **1** |

An interface-returning constructor boxes on every *call to the constructor*, not
on every use. A view held in a field and read repeatedly — a server holding a
view of its own state — boxes once, ever. Today's struct cannot do this: it
re-boxes at every pass, so it allocates **per call, forever**, and 2.2x is the
standing cost rather than a start-up one.

### What an interface-returning constructor costs depends on the view's shape

| construct + one `Get`, no boundary crossed | | allocs |
|---|---|---|
| concrete struct | **12.13 ns** | 0 |
| pointer-shaped struct | 11.79 ns | 0 |
| sealed interface, 3-word view | **30.46 ns** | **1** |
| sealed interface, **1-word view** | **0.32 ns** | **0** |

This is the one place the interface loses, and it loses by 2.5x — but only for a
view that carries something. A view holding just a container pointer is one word,
and boxing it is free, so an interface costs it nothing at all. `SortedSetView`
is that case: ADR `0012` gives it no viewer, because `cmp.Ordered` keys need no
protection.

For the rest, the 2.5x lands on a view used locally — built and read in the same
function, never handed anywhere — which pays for a boundary it never crosses.

The corollary matters more than the cost. Behind an interface, whether the
concrete view is three words boxed once or one word pointing at a heap body is
**the same single allocation and invisible to callers**. The pointer-shaped
option is therefore subsumed rather than rejected, and the representation stays
changeable after release.

### The type-parameter witness is no longer available

`experiments/views/` found a shape that is pointer-shaped *and* free to
construct: carry the converter as a type parameter and materialise its zero
value per call. `0011` recorded it as the cheapest option.

`0012` closed it off. A witness must be **stateless**, and
`FromKeyView(NK) (K, bool)` generally cannot be — turning `"alpha"` back into the
`*Item` it names needs a registry, which is state. The library's own
`views_test.go` viewer holds one. This is a real cost of `0012` that was not
priced at the time, and it should be recorded whether or not this ADR is adopted.

## Decision

**Constructors return sealed, per-container interfaces. The view structs become
unexported, and the `Set` and `Dict` contracts are removed.**

### 1. One sealed `<Container>View` interface per container

```go
type HashDictView[NK, NV any] interface {
	Elems2[NK, NV]
	Get(NK) (NV, bool)
	sealedView()
}

type hashDictView[K comparable, V, NK, NV any] struct {   // unexported
	d      *HashDict[K, V]
	viewer CanViewHashDict[K, NK, V, NV]
}

func ViewHashDict[K comparable, V, NK, NV any](
	d *HashDict[K, V], vw CanViewHashDict[K, NK, V, NV],
) HashDictView[NK, NV]

func ViewHashDictIdentity[K comparable, V any](d *HashDict[K, V]) HashDictView[K, V]
```

Five interfaces: `HashSetView[NT]`, `SortedSetView[T]`, `HashDictView[NK, NV]`,
`SortedDictView[K, NV]`, `MapView[NK, NV]`. The exported name is the one the
struct has today; the struct takes the lowercase form. `viewer` keeps its ADR
`0012` meaning — the thing supplying conversions — and is not reused for the view
itself.

Per-container is what keeps the ordered operations. `SortedDictView` declares
`Range`, `Min`, `Max`, `Floor` and `Ceil` alongside the universal methods, which
one interface spanning every container could not. Whether the five share a base
that the ordered two extend is open question 1 — that is a question about
hierarchy, not about whether each container has its own interface.

Sealing is one unexported method on each struct. `*HashDict` has `Len`, `All` and
`Get`, but not `sealedView`, so **handing over the container where a view is
expected does not compile.** Prototyped and verified, both directions:

```
cannot use &HashDict[string, int]{} ... : *HashDict[string, int] does not
implement DictView[string, int] (missing method sealedView)

cannot use fake{} ... : fake does not implement proto.DictView[string, int]
(missing method sealedView)
```

The second is a type outside the package supplying every exported method. The
seal blocks the container from being passed *as* a view, and blocks anyone else
from claiming to be one.

### 2. `Set` and `Dict` are removed

ADR `0008`'s read-only tier goes away. The view interfaces subsume it, and
keeping both would leave two spellings of "read-only dict" where one is a decoy:
a container satisfies `Dict[K, V]` structurally, which is precisely the hole ADR
`0011` recorded as unclosable and this ADR closes.

### 3. `Elems` and `Elems2` stay, and stay unsealed

The sized constructors take them, so they are a capability check rather than a
boundary — `CollectHashSet(anythingIterable)` has to keep working. They were
never load-bearing for read-only-ness, and are not asked to be.

### 4. `MutableSet` and `MutableDict` absorb their read methods

They embed `Set` and `Dict` today. With those gone they declare `Has` and `Get`
themselves. ADR `0008`'s three layers become two — universal and mutation — with
read-only expressed per container as a view rather than as a contract tier.

### 5. Views satisfy their interface by value

The concrete struct keeps value receivers, so the representation can change to a
pointer, to a body pointer, or to a type-parameter witness later without touching
a caller.

## Consequences

- **One allocation per view, at construction, amortised over every use.** Not one
  per boundary crossing, which is what `0012` pays today and cannot amortise: a
  view held in a field costs 13.42 ns and 0 allocations per call against today's
  28.99 ns and an allocation *per call*.
- **A view that converts nothing costs nothing.** `SortedSetView` is one word, so
  it constructs behind its interface in 0.32 ns with no allocation.
- **A view used only locally pays 2.5x** and gains nothing. This is the accepted
  cost. A view exists to cross a boundary; one that never crosses one did not
  need to be built.
- **Forwarding methods stop inlining**, costing ~1 ns of dispatch per call.
  Against a map `Get` at 3.2 ns and a viewer conversion at ~1.4 ns, this is the
  smallest term in the expression.
- **The concrete representation becomes private, which is the point.** Option D
  is subsumed as an invisible implementation detail, and ADR `0011`'s option C —
  the type-parameter witness — becomes available *per container* wherever the
  viewer happens to be stateless, with no API change. Neither is foreclosed by
  shipping the field-carrying struct first.
- **Option E stays available and gets better.** A caller-side alias now names an
  interface with two type parameters instead of a struct with four.
- **Consumers decouple from the implementation only within a concept.** A
  consumer taking `HashDictView[string, ItemView]` does not name `HashDict`'s key
  and value types — but it does still name *hash*. See the open questions.
- **ADR `0008`'s contract table changes**, and `CLAUDE.md` describes the old one.
- **`views_test.go` and `callsites_test.go` are rewritten**, and the identity
  constructors change return type. `TestEveryContainerHasAView` becomes a check
  that each container has a sealed interface and both constructors.

## Rejected alternatives

Option B was chosen; see the Decision above.

### A. Status quo — concrete structs only

Views stay as `0012` built them. A boundary can be declared two ways, and the
dilemma is that neither is good:

```go
// sketch
v := containers.ViewHashDict(d, viewer)   // concrete, free, 0 allocs

render(v)   // 0 allocs -- the parameter is the concrete type
audit(v)    // 1 alloc  -- boxes into the contract
audit(v)    // 1 alloc  -- again, and on every call, forever

// Spelled with the view type: no boxing, but *Item appears twice and the
// signature names HashDict.
func render(v containers.HashDictView[*Item, *Item, string, ItemView]) error

// Spelled with the read contract: short, but it boxes on every call, and
// *HashDict satisfies it too -- so handing over the container still compiles.
func audit(v containers.Dict[string, ItemView]) error
```

- Free to construct, free to use locally, no new vocabulary.
- Four type arguments; consumers name the implementation; forgetting the view
  stays a silent, compiling mistake. An allocation at every boundary crossing
  that **cannot be amortised** — a view held in a field and read repeatedly
  allocates on every call, not once.

### C. Sealed interfaces alongside the concrete structs

Constructors keep returning concrete structs. The library also declares sealed
interfaces, intended as the type a *boundary* is declared with.

```go
// sketch
v := containers.ViewHashDict(d, viewer)   // concrete, free, 0 allocs
render(v)                                 // boxes once, here -- 1 alloc
audit(v)                                  // boxes again -- 1 alloc

func render(v containers.DictView[string, ItemView]) error
func audit(v containers.DictView[string, ItemView]) error

// Naming the interface once gets the chosen design's single box, per frame:
var w containers.DictView[string, ItemView] = containers.ViewHashDict(d, viewer)
render(w)   // 0 allocs
audit(w)    // 0 allocs
```

- Local use keeps today's cost exactly, and boundaries get the short spelling,
  the decoupling and the compile-time seal. Both cost profiles stay reachable:
  a caller crossing one boundary pays what it pays today, and one crossing many
  opts into a single box by naming the interface once.
- Two vocabularies for one concept, and a naming collision to resolve (below).
  The choice of when to hold which is now the caller's problem — though this
  package has consistently preferred making callers name a choice over making it
  invisibly (`0012`'s removal of `View()` being the precedent).

### D. Pointer-shaped view structs

Keep concrete types; hand out a pointer, so a view is one word.

```go
// sketch -- the view struct is unchanged from 0012; only the constructor's
// return type moves from a value to a pointer.
func ViewHashDict[K comparable, V, NK, NV any](
	d *HashDict[K, V], vw CanViewHashDict[K, NK, V, NV],
) *HashDictView[K, V, NK, NV]

v := containers.ViewHashDict(d, viewer)   // allocates once, elided when v does not escape
render(v)                                 // 0 allocs
audit(v)                                  // 0 allocs -- a pointer is pointer-shaped, boxing is free

// Signatures are as they were under A, plus a star. Only the cost changed.
func render(v *containers.HashDictView[*Item, *Item, string, ItemView]) error
func audit(v containers.Dict[string, ItemView]) error   // still unsealed
```

A one-word *wrapper* — a struct whose only field is a pointer to a body — is the
same option with an extra named type, and measures identical on every axis:
construct 11.83 against 11.79 ns, call 12.62 against 12.37 ns, boxed 13.73
against 13.89 ns, same allocations throughout. The compiler flattens a
single-field struct, so `v.b.d` costs no more than `v.d`.

What the wrapper buys is narrow: the constructor keeps returning a **value**, so
signatures stay star-free and `nil` is not a spellable argument — the only way to
write a broken view is `HashDictView{}`, which panics like any zero view. Against
that, the bare pointer adds no type and matches how containers themselves are
passed (`*HashDict`). Neither is a performance question.

- Fixes the whole allocation story — free to box, amortises like the interface
  (13.02 ns and 0 allocations into an interface parameter), and escape analysis
  elides the body allocation when the view does not escape. Slightly *faster*
  than the interface everywhere measured, since it skips dispatch when the callee
  takes the concrete type. Keeps concrete methods and inlining.
- **Fixes nothing this ADR is about.** Still four type arguments, still names the
  implementation, still forgettable. It answers the cost question while leaving
  the ergonomic one untouched.

### E. Caller-side generic aliases, no library change

```go
// sketch, in the CONSUMER's package -- the library is unchanged
type ItemDictView = containers.HashDictView[*Item, *Item, string, ItemView]

v := containers.ViewHashDict(d, viewer)   // concrete, free, 0 allocs
render(v)                                 // 0 allocs
audit(v)                                  // 1 alloc, every call -- unchanged from A

func render(v ItemDictView) error                       // short, in this package only
func audit(v containers.Dict[string, ItemView]) error   // unchanged from A

// No help here: a generic consumer cannot drop the container's parameters,
// because an alias has to fix them.
func each[K comparable, V, NK, NV any](v containers.HashDictView[K, V, NK, NV]) error
```

- Free, available today, and `experiments/views/` verified aliases are identical
  types rather than distinct ones.
- Shortens the spelling only at the point of instantiation. A *generic* consumer
  still cannot drop the container's parameters, the coupling is unchanged, and
  every consumer repeats the alias.

## Open questions

All three are about sorted containers, and they are the same question at
different depths: **how much shared vocabulary survives the removal of `Dict` and
`Set`?**

### 1. Is there a shared base interface, or are the five unrelated?

Five mutually unrelated interfaces is the strictest reading of "one per
container". It has a cost this ADR should not pretend away: a consumer cannot be
written against "any dict view", and a provider switching `HashDict` for
`SortedDict` breaks every consumer — which is half of what decoupling was
supposed to buy.

The alternative reintroduces `Dict` and `Set` under new names, sealed:

```go
// sketch
type DictView[NK, NV any] interface {
	Elems2[NK, NV]
	Get(NK) (NV, bool)
	sealedView()
}

type SortedDictView[K cmp.Ordered, NV any] interface {
	DictView[K, NV]                       // embeds cleanly -- see below
	Range(lo, hi K) iter.Seq2[K, NV]
	Min() (K, NV, bool)
	Max() (K, NV, bool)
	Floor(K) (K, NV, bool)
	Ceil(K) (K, NV, bool)
}

type HashDictView[NK, NV any] = DictView[NK, NV]   // or no separate name at all
```

That is not obviously wrong — a sealed `DictView` is a different object from the
structural `Dict` this ADR deletes, because a container cannot satisfy it. But it
is worth deciding deliberately rather than arriving at by embedding.

### 2. The embedding works, which is a consequence of ADR `0012`

`SortedDictView[K, NV]` embedding `DictView[K, NV]` type-checks only because ADR
`0012` decided sorted views convert **values only**. Its `Get` takes `K`, its
`All` yields `iter.Seq2[K, NV]` — exactly `DictView` instantiated at `[K, NV]`.
Had sorted keys been converted, the two would not line up and the hierarchy would
be impossible.

The same holds for sets: `SortedSetView[T]` converts nothing, so it is
`SetView[T]` plus the ordered methods.

This is worth recording because it is load-bearing and accidental: ADR `0012`
made that decision to sidestep order preservation, not to enable a hierarchy.
**ADR `0003`'s `SortedDictFunc` follow-up would reopen both at once.**

### 3. Should `Range` return a view rather than an iterator?

ADR `0011` listed this as a capability gap. Under this decision it becomes cheap
to express — `Range(lo, hi K) SortedDictView[K, NV]` returns an interface like
everything else — where against a concrete struct it meant naming a fourth type.
Not decided here, but the cost of doing it later just fell.

## Why B rather than C

The measurements pointed at C. C takes the boundary win where it exists and pays
nothing where it does not, and while this ADR was proposed that is what it
recommended. The decision goes the other way for a reason the measurements cannot
see.

**C exports the struct, so the representation is permanent API.** Every caller
that names `HashDictView[*Item, *Item, string, ItemView]` is naming a four-word
decision the library has made exactly once, on evidence gathered this week, about
a question that is still moving — three words against a body pointer against a
witness. B exports only the interface, so that choice stays revisable after
release.

That is what makes the future options real rather than rhetorical. **Option E**
(caller-side aliases) applies to an interface with two type parameters instead of
a struct with four, so it gets shorter. **ADR `0011`'s option C** — the
type-parameter witness, the cheapest shape measured there — is closed today only
because `0012`'s `FromKeyView` is generally stateful; behind an interface the
library can adopt it per container wherever the viewer *is* stateless, and
callers never learn. Under option C of this ADR, both would be breaking changes.

The cost of buying that is precise and bounded: 2.5x on views that never cross a
boundary, nothing on views that carry no viewer, and a cheaper profile than
today's everywhere else. Being exact about the cheaper part, because "interfaces
cost dispatch" is true and beside the point: passing an interface is free and
dispatch is ~1 ns. The 16 ns is the allocation forced by dispatching through a
non-pointer-shaped receiver, and today's view is non-pointer-shaped. The
interface does not add that cost — it removes it, and removes it permanently,
since a view held once stops paying while today's struct re-boxes forever.

## Follow-ups

- **`LinkedList` (ADR `0010`) inherits this before it is written.** Its `All`
  yields cursors, so `LinkedListView` hands out handles that are inert without
  the list — worth settling when the container lands.
- **The witness shape's loss to stateful viewers belongs in `0012`'s record.**
  ADR `0011` left it available as the cheapest form; `0012` closed it without
  noticing.
- **ADR `0008`'s deferred ordered tier is partly answered here.** If open
  question 1 lands on a shared base, `SortedDictView` *is* the ordered tier, on
  the view side. Whether containers still want one is separate.
- **`CLAUDE.md` needs the new contract table and view convention** once this is
  implemented.
