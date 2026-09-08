# 13. Interfaces for view types

- **Status:** Proposed.
- **Date:** 2026-09-08
- **Evidence:** `experiments/viewiface/` (`RESULTS.md`); background numbers from
  `experiments/views/` and `experiments/dispatch/`.
- **Relates to:** ADR `0011` (which chose concrete structs, and recorded
  "sealing stays forgettable" as an unresolved limitation this would close),
  `0012` (whose four-type-argument views are what prompted this), `0008`
  (naming: contracts take the bare concept, implementations `<Ordering><Concept>`).

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

### An interface-returning constructor always allocates

| construct + one `Get`, no boundary crossed | |
|---|---|
| concrete struct | **12.13 ns, 0 allocs** |
| pointer-shaped struct | 11.79 ns, 0 allocs |
| sealed interface | **30.46 ns, 1 alloc** |

This is the one place the interface loses, and it loses by 2.5x. A view used
locally — built and read in the same function, never handed anywhere — pays for
a boundary it never crosses.

Note the pointer-shaped struct does not: its constructor allocates in isolation,
but escape analysis elides that when the view does not escape.

### The type-parameter witness is no longer available

`experiments/views/` found a shape that is pointer-shaped *and* free to
construct: carry the converter as a type parameter and materialise its zero
value per call. `0011` recorded it as the cheapest option.

`0012` closed it off. A witness must be **stateless**, and
`FromKeyView(NK) (K, bool)` generally cannot be — turning `"alpha"` back into the
`*Item` it names needs a registry, which is state. The library's own
`views_test.go` viewer holds one. This is a real cost of `0012` that was not
priced at the time, and it should be recorded whether or not this ADR is adopted.

## Alternatives

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

### B. Constructors return sealed interfaces

`ViewHashDict` returns `DictView[NK, NV]`; the structs become unexported.

```go
// sketch
type DictView[K, V any] interface {
	Elems2[K, V]
	Get(K) (V, bool)
	sealedView()
}

v := containers.ViewHashDict(d, viewer)   // returns DictView -- boxes HERE, 1 alloc
render(v)                                 // 0 allocs, already an interface
audit(v)                                  // 0 allocs
v.Get("alpha")                            // 0 allocs, ~1 ns dispatch

func render(v containers.DictView[string, ItemView]) error
func audit(v containers.DictView[string, ItemView]) error

// The box lands even when nothing crosses anything:
n := containers.ViewHashDict(d, viewer).Len()   // still 1 alloc
```

- One public name per concept. The container's types leave the API entirely.
  Consumers decouple from the implementation. Boxing becomes constant. The
  boundary type is sealed, so passing a container does not compile.
- Every view allocates, including views that never cross a boundary (2.5x on
  local use). Direct calls stop inlining. The concrete type's extra methods need
  their own interfaces — `SortedDictView` must carry `Range`, `Min`, `Max`,
  `Floor`, `Ceil`, or ordered views lose them.
- **Regresses `SortedSetView`,** which is 8 bytes today, pointer-shaped, and
  already boxes for free. Behind an interface it would allocate for the first
  time.

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

// Naming the interface once opts into B's behaviour for the rest of the frame:
var w containers.DictView[string, ItemView] = containers.ViewHashDict(d, viewer)
render(w)   // 0 allocs
audit(w)    // 0 allocs
```

- Local use keeps today's cost exactly, and boundaries get the short spelling,
  the decoupling and the compile-time seal. Both cost profiles stay reachable:
  a caller crossing one boundary pays what it pays today, and one crossing many
  opts into B's single box by naming the interface once.
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

1. **Do ordered views get interfaces?** `Range`/`Min`/`Max`/`Floor`/`Ceil` mean
   ordered views need a second, wider interface rather than sharing `DictView`.
   Scoping the first cut to `SetView` and `DictView` avoids that, and avoids
   regressing `SortedSetView`, at the price of the ordered views keeping the long
   spelling.
2. **Naming.** `0008` gives contracts the bare concept name, which suggests
   `SetView[NT]` and `DictView[NK, NV]`. Under option C those coexist with the
   structs `HashDictView` and `SortedDictView`; under B the structs can be
   unexported and the collision disappears. If ordered views get interfaces
   under C, `SortedDictView` is claimed by the struct and something has to give.
3. **Does `Elems`/`Elems2` need sealing too,** or only the view interfaces? The
   sized constructors take `Elems2`, which is exactly why containers satisfy the
   read contracts and why the seal is forgettable in the first place.

## What the evidence supports

The cost question the numbers were run to answer comes back clearly: **an
interface at a boundary is cheaper than what `0012` ships today**, not more
expensive, and the one case where it loses is the case where no boundary is
crossed. That points at C rather than B — it is the option that takes the win
where the win exists and pays nothing where it does not.

It is worth being exact about *what* is cheaper, because "interfaces cost
dispatch" is true and beside the point. Passing an interface is free; dispatch is
~1 ns. The 16 ns is the allocation forced by dispatching through a
non-pointer-shaped receiver, and today's view is non-pointer-shaped. The
interface does not add a cost — it removes one, and removes it permanently,
since a view held once stops paying while today's struct re-boxes on every call
forever.

But the numbers do not decide between the interface and option D. Those two are
within 3% of each other everywhere measured. The real question this ADR turns on
is not cost at all: it is whether dropping the container's type arguments from
consumer signatures, and making a forgotten view fail to compile, is worth a
second vocabulary. That is a judgement about the API, and it should be made as
one.

## Follow-ups

- If B or C is adopted, `LinkedList` (ADR `0010`) inherits the decision before it
  is written.
- The witness shape's loss to stateful viewers (above) belongs in `0012`'s record
  regardless of what happens here.
