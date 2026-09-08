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
| 1 | 30.36 ns, **1 alloc** | 30.20 ns, 1 alloc | 28.53 ns, 1 alloc |
| 2 | 59.25 ns, **2 allocs** | 43.15 ns, 1 alloc | 40.91 ns, 1 alloc |
| 4 | 118.5 ns, **4 allocs** | 69.97 ns, 1 alloc | 67.57 ns, 1 alloc |
| 8 | 234.8 ns, **8 allocs** | 127.4 ns, 1 alloc | 123.9 ns, 1 alloc |

`0012`'s view is 24 bytes — a container pointer plus a two-word viewer — so it
is not pointer-shaped and re-boxes at every crossing. An interface boxes once.

Marginally, one more crossing costs **13.9 ns** through an interface and
**29.2 ns** through the concrete struct: **boxing roughly doubles the cost of a
boundary crossing.** Crossover is at two crossings; at one they are
indistinguishable, because both pay exactly one box.

So the answer to "how much would an interface cost in boxing" is **negative**.
Dispatch is 0.7 ns (`dispatch`); the allocation it removes is 13 ns. At a
boundary the interface is never slower than what `0012` has today.

### Escape analysis rescues the concrete form only where a view is pointless

| | |
|---|---|
| consumer not inlinable | 29.27 ns, **1 alloc** |
| consumer inlinable | 11.72 ns, **0 allocs** |
| direct call, no interface at all | 12.04 ns, 0 allocs |

When the box cannot outlive the frame it goes on the stack and costs nothing.
That rescue is gone as soon as the callee is in another package, is too large to
inline, or retains the value — which describes an API boundary. **The concrete
view is free exactly where nobody needs a view, and allocates exactly where
someone does.**

### An interface-returning constructor always allocates

| construct + one `Get`, no boundary crossed | |
|---|---|
| concrete struct | **11.75 ns, 0 allocs** |
| pointer-shaped struct | 12.05 ns, 0 allocs |
| sealed interface | **30.36 ns, 1 alloc** |

This is the one place the interface loses, and it loses by 2.6x. A view used
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

Views stay as `0012` built them.

- Free to construct, free to use locally, no new vocabulary.
- Four type arguments; consumers name the implementation; allocation at every
  boundary crossing; forgetting the view stays a silent, compiling mistake.

### B. Constructors return sealed interfaces

`ViewHashDict` returns `DictView[NK, NV]`; the structs become unexported.

```go
// sketch
type DictView[K, V any] interface {
	Elems2[K, V]
	Get(K) (V, bool)
	sealedView()
}

func render(v containers.DictView[string, ItemView]) error
```

- One public name per concept. The container's types leave the API entirely.
  Consumers decouple from the implementation. Boxing becomes constant. The
  boundary type is sealed, so passing a container does not compile.
- Every view allocates, including views that never cross a boundary (2.6x on
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
v := containers.ViewHashDict(d, viewer)      // concrete, free, 0 allocs
render(v)                                     // boxes once, here

func render(v containers.DictView[string, ItemView]) error
```

- Local use keeps today's cost exactly. Boundaries get the short spelling, the
  decoupling and the compile-time seal. A caller who crosses many boundaries can
  opt into B's behaviour by naming the interface once
  (`var v containers.DictView[string, ItemView] = containers.ViewHashDict(...)`),
  and then pays one box; a caller who crosses one pays the same as today.
- Two vocabularies for one concept, and a naming collision to resolve (below).
  The choice of when to hold which is now the caller's problem — though this
  package has consistently preferred making callers name a choice over making it
  invisibly (`0012`'s removal of `View()` being the precedent).

### D. Pointer-shaped view structs

Keep concrete types; move the fields behind one pointer so a view is one word.

- Fixes the whole allocation story — free to box, and escape analysis elides the
  body allocation when the view does not escape. Slightly *faster* than the
  interface everywhere measured. Keeps concrete methods and inlining.
- **Fixes nothing this ADR is about.** Still four type arguments, still names the
  implementation, still forgettable. It answers the cost question while leaving
  the ergonomic one untouched.

### E. Caller-side generic aliases, no library change

```go
// sketch, in the consumer's package
type ItemDictView = containers.HashDictView[*Item, *Item, string, ItemView]
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
