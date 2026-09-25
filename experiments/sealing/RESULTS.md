# Sealing, and what the view wrapper costs

Two questions, one harness.

1. **Is the concrete struct wrapper slower than the interface it wraps?** ADR
   `0018` ships `MapSetView[NT] struct{ impl Keys[NT] }`; ADR `0020` proposed
   replacing it with a bare interface or a carried witness. Nobody had priced
   the wrapper itself.
2. **What does it cost to make "read-only" a compile-time guarantee?** The shape
   interfaces in `contracts.go` are deliberately unsealed, so a container placed
   in one can be asserted back out and written through.

See `bench.txt` for raw output and provenance. `run.sh` also **asserts that the
sealed assertion still fails to compile**, so a change that reopens the hole
breaks the experiment rather than passing quietly.

## 1. The wrapper is not the expensive part

Same converting implementation underneath all three; n=64.

| | **StructView** (2 words, today) | **IfaceView** (sealed interface) | **PtrView** (1 word) |
|---|---|---|---|
| width | 16 B | 16 B | **8 B** |
| **zero value** | **reads empty** | **PANICS** | **reads empty** |
| `View()`, identity | **0.39 ns / 0** | 0 allocs | — |
| construct, converting | 13.5 ns / **1** | 13.3 ns / **1** | 23.8 ns / **2** |
| point read | 1.40 ns / 0 | 1.06 ns / 0 | 1.08 ns / 0 |
| iterate | 422 ns / 3 | 425 ns / 3 | 423 ns / 3 |
| → **concrete** view parameter | **1.86 ns / 0** | n/a | — |
| → sealed **shape interface** | 14.8 ns / **1** | **2.26 ns / 0** | 2.83 ns / 0 |

**Method calls and iteration are a wash.** Iteration is 422 / 425 / 423 ns —
indistinguishable. The wrapper's extra hop inlines away, and there is still
exactly one dynamic call in every shape. Point reads differ by ~0.3 ns, which is
a third of an interface call and not worth a design decision.

**The wrapper's only real cost is boxing into a shape interface**: 14.8 ns and
one allocation, against ~2.3 ns and none. A two-word struct cannot be
pointer-shaped, so entering an interface must copy it to the heap.

**And that cost is avoidable.** A read-only API that names the *concrete view
type* has no interface to box into: **1.86 ns, 0 allocations**. So the boxing
only lands on code generic over *container kind* — a function that must work on
both a `MapSet` and a `SortedSet`.

**`View()` is free with the wrapper.** An identity implementation holds only the
container, so it is one word, and a one-word value boxes into the inner
interface for free: `s.View()` costs **0.39 ns and 0 allocations** and returns
the ordinary view type, with the zero value still reading empty. No separate
`…IdentityView` type is needed to make it cheap.

**`PtrView` is dominated.** One word buys free boxing, but the state must still
hold an interface to erase the container's type parameter, and an interface is
two words — so the state is heap-allocated and construction costs **two**
allocations instead of one. It is no better than the interface at a boundary and
worse everywhere else.

> An earlier version of this harness hardcoded the container type inside
> `PtrView`'s state. That removed the type-parameter erasure the real design
> cannot avoid, and made point reads read 0.37 ns with no dynamic call at all.
> Any measurement of a view shape that does not erase a type parameter is
> measuring something the library cannot ship.

## 2. Sealing: what closes the hole, and what only looks like it does

The mechanism is an unexported method on the interface **which the container does
not implement**. Only an unexported wrapper does. Every route out:

| route | outcome |
|---|---|
| `v.(MapSet[string])` | **compile error** — `impossible type assertion … (missing method viewOnly)` |
| `any(v).(MapSet[string])` | fails at runtime |
| `any(v).(interface{ Add(string) })` | fails at runtime |
| `any(v).(interface{ Len() int })` | succeeds — harmless |
| embed the sealed interface | satisfies the seal, promotes **no** mutator |
| `reflect` | `CanSet=false`, `CanInterface=false`, `Interface()` panics |
| `unsafe` | gets through, as it does through every Go guarantee |

The compile error is the load-bearing part: Go rejects an assertion to a
concrete type that *cannot* implement the interface. `run.sh` asserts that
error still appears, and the harness makes `MapSet[string]` match
`IfaceView[string]` on every other method so `viewOnly` is the **only** thing
named — otherwise the check would pass on an accidental signature mismatch and
keep passing with the seal removed.

Embedding is worth knowing about rather than fearing: it satisfies the seal, so
test doubles remain writable, and it promotes only read methods so the guarantee
survives.

### Sealing an interface the container also satisfies is theater

`TestSealingAloneIsTheater`: a sealed interface whose *container* supplies the
token is exactly as leaky as an unsealed one — the assertion back to the
container succeeds, because the container genuinely is the dynamic type.

**A seal only helps if the container is excluded from it.** Which means a
container can no longer be passed where reads are promised, and the caller must
write `.View()`.

### One shared token is required, not one per interface

An interface-typed value satisfies another sealed interface only if it
**declares** the same unexported method — what its dynamic type has is
irrelevant. With a token per interface, a view handed out as `MapSetView[T]`
would not fit `ReadKeys[T]` even though its implementation has both methods. So
the whole package needs a single token.

### The token's name is the compiler's error message

```
cannot use s (variable of struct type MapSet[int]) as ReadKeys[int] value
in argument to readOnly[int]: MapSet[int] does not implement ReadKeys[int]
(missing method viewOnly)
```

That last clause is the entire diagnostic a caller gets. Naming the token for
what the caller should do (`callViewFirst`) turns the error into instructions.

### The seal never protects the contents

`TestSealDoesNotFreezeContents`: through a sealed, un-castable view over
`MapSet[*Item]`, every `*Item` remains freely mutable. **The structure is
sealed; the reachable object graph is not.**

Go cannot express "contains no pointers" as a constraint — `comparable` admits
pointers, a marker interface cannot be implemented by builtin types, and type
sets cannot describe a struct's fields. So unlike the cast, this is **not
closable at any price**, and is the one part of "read-only" that has to remain a
documented convention. Go's own precedent is the same: `slices.Clone` states its
shallowness in the doc comment and is not named `ShallowClone`.

## 3. Composing views: where the source parameter should point

ADR `0023`'s question. Today a converting constructor takes the **concrete
container**, so a view cannot be narrowed or re-converted by whoever holds it —
and the seal makes that final, since the holder cannot reach the container. Two
source-parameter shapes fix it.

| n=64 | construct | iterate | per layer |
|---|---|---|---|
| baseline — source is the **container** (today, not composable) | **1** alloc | 537 ns / 6 | — |
| **A** — source is the **concrete view type** | **1** alloc | 539 ns / 6 | — |
| **B** — source is the **sealed shape interface** | **2** allocs | 538 ns / 6 | — |
| A, composed 2 deep | 1 alloc | 697 ns / 9 | +160 ns, +3 |
| B, composed 2 deep | 2 allocs | 695 ns / 9 | +156 ns, +3 |
| A, composed 3 deep | 1 alloc | 874 ns / 12 | +177 ns, +3 |

**Taking a view as the source costs nothing per element.** 537 / 539 / 538 ns is
one number three times. The extra dynamic call a view source introduces is paid
**once per `Keys()` call**, obtaining the iterator — not once per element — so at
n=64 it is already invisible. This is the same per-call-not-per-element shape as
finding 2's allocations.

**A dominates B.** Same walk cost, but B must box a two-word view to enter the
sealed interface, so it construct-allocates twice where A allocates once. B's
compensation is that it accepts *any* key-exposing view whatever container it came
from; A accepts only its own view type. That is a design question about whether
cross-kind re-viewing is wanted, not a performance one.

**Each composition layer costs ~160 ns and 3 allocations, per call.** Linear, and
it is the nested range-over-func machinery: one closure plus the range state and
yield closure for each additional `for range` in the chain. Depth is therefore
cheap to add and cheap to walk, but a composed view is a worse candidate for a
hot loop than a flat one — and `Each` (ADR `0021`) removes the per-layer
allocations for the same reason it removes the flat ones.

### A slice adapter is the one container whose `View()` allocates

These numbers were taken for a `Slice[T] []T` adapter proposed inside ADR `0023`
and **withdrawn** — they are part of why. They stay here because the question is
deferred to a later ADR, and because the boxing rule they demonstrate is general.

Two things follow from a slice header being three words where every other
container is one:

| | width | `View()` |
|---|---|---|
| a one-word container (`Map`, `Vector`, `MapSet`) | 8 B | **0 allocs** |
| a defined `[]T` | **24 B** | **1 alloc** |

A one-word value boxes into a view's interface field for free; three words cannot.
This is not a regression — `ViewSliceIdentity([]T)` already costs one allocation
today for the same reason — but it is an exception to ADR `0022`'s rule that
`View()` is free, and the rule's stated reason ("the container is one word") is
exactly why.

Two other facts the ADR needed:

- **A defined `[]T` satisfies the whole position-keyed inner tier on value
  receivers** — `Len`, `At`, `Positions`, `PositionSlice`, `Values`, `ValueSlice`,
  `All`, `AllSlice`, and `Set`. Only `Append` is impossible, because the length
  lives in the header, which *is* the value.
- **A conversion cannot infer its type argument, and a function call can.**
  `Slice(s)` is `cannot use generic type Slice without instantiation`;
  `CastSlice(s)` infers `T` and costs **0 allocations**. This is the finding that
  outlived the withdrawn adapter: ADR `0023` keeps `AsMap` for `Map`, which has
  the same gap.

## Durable / perishable

**Durable.** A concrete struct wrapping an interface adds no measurable cost to
method calls or iteration over the interface alone; its only penalty is that a
two-word value cannot be pointer-shaped, so entering another interface
allocates. Naming a concrete type as a parameter avoids that entirely. A
one-word view must still hold an interface to erase a type parameter, so it
trades one allocation per boundary for one per construction and loses.

A sealed interface closes every route back to a writable type except `unsafe`,
**provided the container does not implement the token**; sealing an interface the
container satisfies changes nothing. Sealed interfaces compose only through a
single shared token, because satisfaction is decided by an interface's declared
method set and not by its dynamic type. The unexported method's name is the only
diagnostic a caller receives. Embedding satisfies a seal without granting
mutators. A seal constrains structure only; Go has no way to express deep
immutability, so contents cannot be protected by the type system.

A one-word container boxes into a view's interface field for free; a three-word
slice header cannot, so a slice adapter's view construction allocates where every
other container's does not. A defined slice type satisfies a position-keyed read
contract on value receivers but can never grow, because its length lives in the
value. A conversion cannot infer its type arguments; a function wrapping one can,
for free.

A view used as a converting constructor's SOURCE adds one dynamic call per
iterator construction, not per element, so composition depth is free to walk and
costs only at the call. Boxing a two-word view into a sealed interface costs the
allocation that taking the concrete view type avoids. Each nested range-over-func
layer costs its own closure plus range state and yield closure.

**Perishable.** Every absolute number, and the ~1 ns point-read spreads in
particular — they are at the resolution limit and ordered differently between
runs.
