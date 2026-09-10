# 18. Containers as reference types, reconsidered

- **Status:** **Rejected — on a narrower basis than ADR `0014` gave, and closer
  than `0014` was.** The status quo stands. `0014`'s *decisive* argument is dead:
  option (b) is now free on every measured path and coherent as a design, so it
  is rejected on the balance of its API costs rather than on cost or feasibility.
  **Option (b) is built out in full below**, including how it would match builtin
  map semantics, because it is the version a future attempt should start from.
- **Date:** 2026-09-10
- **Supersedes:** ADR **`0014`**, which is now historical. Its measurements
  remain valid; its conclusion is reached here by a different route, and two of
  its open questions have been answered by unrelated work.
- **Evidence:** `experiments/refcontainers/` (`RESULTS.md`), which re-measures
  against today's method set and prices the all-interface shape. Context from `experiments/reftypes/` (ADR `0014`'s
  harness, still valid) and `experiments/viewiface/` §9.
- **Relates to:** ADR `0002` (the container shape), `0007` (the value/pointer
  asymmetry), `0009` (`HashDict`), `0013` (views as sealed interfaces),
  `0017` (which removed `Elems`/`Elems2` and changed this calculus without
  meaning to).

## Why this is being reopened

ADR `0014` rejected reference containers with a five-link chain. Three things
have happened since, none of them aimed at this question, and they hit different
links.

**1. `Elems2` is gone, and with it `0014`'s decisive cost.** Link 4 read:
*"Making views structs to match costs ADR `0013`'s hierarchy — struct embedding
is not subtyping — plus an allocation whenever a view is passed to `Elems2`,
which is what every sized constructor takes."* ADR `0017` deleted `Elems` and
`Elems2` and made every bulk operation variadic in its element type. **Nothing
in the library boxes a view into a foreign interface any more** — verified by
grep, not assumed. The allocation half of that link no longer exists.

**2. `Map` and `HashDict` converged on their own.** ADR `0014` left an open
question — "what `Map` must gain before `HashDict` can be removed" — listing
`NewHashDict`, `CollectHashDict`, `CollectHashDictSeq`, `SetAll` and
`SetAllSeq`. ADR `0017` gave `Map` `SetAll` and `DeleteAll`, deleted the whole
`Collect*` family, and refused `NewMap` on the grounds that a composite literal
and `make` already construct one. Their method sets are now **identical except
`NewHashDict`**:

```
HashDict: All AllSlice Clone Delete DeleteAll Get Keys KeySlice Len Set SetAll Values ValueSlice  + NewHashDict
Map:      All AllSlice Clone Delete DeleteAll Get Keys KeySlice Len Set SetAll Values ValueSlice
```

So `HashDict`'s only remaining justification is ADR `0007`'s value/pointer
asymmetry — precisely what reference containers dissolve. **This is a cost of
the status quo that has grown since `0014`.**

**3. The eager-dereference rule is now comprehensively tested.** ADR `0017` added
`TestEmptyBulkCallsStillDereference`, which asserts that every bulk method on
every container panics on a nil receiver even with zero arguments. That is a
safety net reference semantics would remove, and it did not exist when `0014`
was written.

## What was re-measured

From `experiments/refcontainers/`. Full tables in its `RESULTS.md`.

**The representation is still free**, now on today's method set rather than
`0014`'s toy set. `Has` 4.252 ns (pointer) against 4.215 ns (reference); `Keys`
335.2 against 348.9; `KeySlice` — new since `0017`, and on the hot path for every
bulk operation — 431.5 against 446.9; construction identical at 3 allocations
either way. **What reference semantics costs is still paid in rules, not
cycles.**

**The nil check is now at or below the noise floor**, at ~1.7% on a point read
against the **+8.5%** `0014` measured. The branch did not get cheaper; the
baseline got more expensive, so the same fixed cost is a smaller share. The
durable form is the weak one: a predictable nil branch is a fraction of a
nanosecond.

**Struct-shaped views are performance-neutral, where `0014` priced them at 6.8x
on the constructor path:**

| | sealed interface (today) | `struct{iface}` |
|---|---|---|
| construct | 0.3751 ns, 0 allocs | 0.3701 ns, 0 allocs |
| `Has` | 4.871 ns | 4.828 ns |
| `Keys` | 386.7 ns, 3 allocs | 384.6 ns, 3 allocs |
| `KeySlice` | 420.8 ns, 1 alloc | 422.7 ns, 1 alloc |
| substitute ordered → base | 0.6482 ns | **0.2825 ns** |

The explicit conversion that replaces interface embedding is **cheaper than the
embedding it replaces**. `0014` assumed it was a tax; it is not, at runtime.

**One negative result, recorded because it nearly went the other way.** A first
cut of the harness wrapped a *concrete* container, measured 0 allocations on
`Keys` against the interface's 3, and appeared to show that struct views fix ADR
`0013`'s live erratum. That was an inlining artifact of a monomorphic wrapper. A
real `SetView[T]` must view a `HashSet`, a `SortedSet` or a converting view, so
it has to wrap an **interface**, and the inner call stays dynamic — three
allocations per iterator, unchanged. **A polymorphic view cannot wrap a concrete
type, so monomorphic measurements of one are invalid.**

## The chain, link by link

`0014`'s argument was: *there is no way to give containers reference semantics
without ending up with two spellings of "is this handle empty", or a worse hazard
than the one being fixed.*

| link | status |
|---|---|
| 1. A reference container must be a struct wrapping a pointer — methods cannot be declared on a defined pointer type, and a defined slice type needs a pointer receiver to grow | **holds** — a language fact |
| 2. A struct cannot be compared to nil, so the emptiness check must be a method | **holds** |
| 3. Views are sealed interfaces and theirs is `== nil`; declaring `IsNil()` on a view interface does not unify, since calling a method on a nil interface panics | **holds** |
| 4. Making views structs costs the hierarchy **and an allocation on the `Elems2` path** | **half dead** — the allocation is gone, measured neutral; the hierarchy cost remains but is *ergonomic*, not runtime |
| 5. The hybrid keeps two spellings **and** reintroduces the typed-nil trap | **holds** |

So the options are no longer "two spellings, two spellings plus a trap, or one
spelling bought with a lost hierarchy **and a per-call allocation**". The
allocation is gone. It is now:

- **(a) reference containers, interface views** — two spellings: `c.IsZero()` for
  containers, `v == nil` for views.
- **(b) reference containers, struct views** — one spelling, `IsZero()`
  everywhere, at the cost of ADR `0013`'s substitutability. Free at runtime;
  expanded in full below.
- **(c) the status quo** — one spelling, `== nil`, for both, and no method at all.
- **(d) everything is an interface** — containers *and* views. One spelling,
  `== nil`, for both, no method at all, and the container/view representation
  split disappears entirely. Priced below; it is the only option that also
  removes ADR `0002`'s set-algebra ceiling.

## Option (b), in full

The closest of the four to adoptable, and the one whose case is most often
stated backwards. Containers become one-word structs wrapping shared state, with
value receivers; views become one-word structs wrapping the sealed interface.

```go
type HashSet[T comparable] struct{ st *hashSetState[T] }
type SetView[NT any]       struct{ impl setViewImpl[NT] }

func (s HashSet[T]) IsZero() bool { return s.st == nil }
func (v SetView[NT]) IsZero() bool { return v.impl == nil }
```

**What it actually buys — and it is not the nil story.**

ADR `0014` framed this whole question around how many spellings of "is this
handle empty" the library ends up with, and on that metric **option (b) is a
lateral move, not a win**. The status quo already has one spelling: `*HashSet[T]`
is a pointer and `SetView[T]` is an interface, so `== nil` works on both, with no
method. Option (b) also has one spelling, `IsZero()` — measured at 0.1874 ns
against `== nil`'s 0.1890 ns, i.e. identical — but it is a method rather than an
operator. **Nil is what option (b) pays, not what it earns.**

What it earns is three things, none of which are about nil:

- **`noCopy` and the copylocks caveat disappear.** Copies share, so there is
  nothing to catch. This retires the hazard `CLAUDE.md` documents at length —
  that copylocks is not in `go test`'s default vet subset, so a shallow-copy bug
  passes `go test ./...` — by removing the bug class rather than the tooling gap.
- **`HashDict` collapses into `Map`.** ADR `0007`'s value/pointer asymmetry is
  the only thing keeping them apart, and their method sets are now identical.
  A whole type, its two view constructors, its two view structs and a redundant
  `CanViewHashDict` all go.
- **Containers and views become the same kind of thing.** One word, freely
  copied, reference-semantic. A caller stops changing representation at the
  boundary.

**What it costs at runtime: nothing.**

| | status quo | option (b) |
|---|---|---|
| container `Has` | 4.289 ns | 4.382 ns |
| container `KeySlice` | 420.8 ns, 1 alloc | 446.4 ns, 1 alloc |
| view `Has` | 4.819 ns | 4.912 ns |
| view `Keys` | 374.3 ns, 3 allocs | 368.9 ns, 3 allocs |
| view `KeySlice` | 417.6 ns, 1 alloc | 419.0 ns, 1 alloc |
| substitute ordered → base | 0.6482 ns | **0.2825 ns** |
| the emptiness check | 0.1890 ns | 0.1874 ns |

Free on every path, and the explicit conversion that replaces interface
embedding is *cheaper* than the embedding. **Option (b) cannot be rejected on
cost**, which is precisely what has changed since `0014`: it used to carry a
6.8x charge on the `Elems2` path, and that path no longer exists.

**What it costs in API shape, which is where it is actually decided.**

- **ADR `0013` decision 2 becomes explicit rather than implicit.** An ordered
  view can no longer be handed to a base-typed boundary without saying so. This
  is the cost that softens most under a complete design — struct embedding
  promotes the base methods and makes the conversion a *field selector* rather
  than a method call, and it still composes into a `[]SetView`. See the section
  below. What remains is that the selector must be written.
- **Both sides need nil checks, or their zero values disagree.** This is the
  cost most easily missed, and it was found by testing rather than reasoning: a
  zero reference container reads as empty by design, but a zero `struct{iface}`
  view **panics on every method**, because the field is a nil interface with
  nothing to dispatch to. Making them agree means nil-checking every method on
  the view side as well — roughly double the boilerplate of the container-only
  design, spread across every method of every container and every view.
- **ADR `0002`'s usable zero value reverses.** `var s HashSet[int]; s.Add(1)`
  works today via addressability and would panic. That is a deliberate trade —
  the zero value becomes "the empty container you may read but not write", which
  is the semantics a reader already has for maps — but it is a reversal of an
  accepted rule, and it will surface at real call sites.
- **Error detection is lost where ADR `0017` made it matter more.** Reads on an
  unconstructed container return empty instead of panicking, so
  `NewVector(src.KeySlice()...)` on a zero `src` silently yields an empty
  container. The library now has a test asserting every bulk method on every
  container panics on a nil receiver; that net goes.
- **The migration is the whole library.** Every container, every view, every
  constructor, every contract assertion, every test.

**One thing gets simpler, and is worth noting.** A struct whose only field is
unexported cannot be built populated outside the package, so `sealedView()`
becomes unnecessary — the seal is structural rather than a method — and
`v.(*HashDict[K, V])` stops compiling because `v` is not an interface at all.

**A refinement option (b) contributed regardless of its fate.** ADR `0014`
established that a nil check must sit *inside* the closure a method returns.
Necessary, and not sufficient: a checked closure that ranges over the inner
sequence and re-yields builds a **second** closure — 481.9 ns and 5 allocations
against 404.8 ns and 3 for handing `yield` straight through. The full rule is
now recorded in `experiments/refcontainers/`: **the check goes inside the
returned closure, and the closure must not re-yield.**

### What a complete option (b) would look like

The costs above are individually small and collectively a design, so this
section builds the whole thing rather than listing objections. **Consistency is
the point**, and the target is stated precisely: *a container's zero value
behaves exactly as the builtin it is standing in for.*

#### The single rule: reads are total, writes panic

That is nil-map behaviour, and it is one sentence rather than a table of special
cases:

```go
var s HashSet[int]
s.Len()              // 0
s.Has(1)             // false
for range s.Keys() {}  // no iterations
s.KeySlice()         // nil
s.IsZero()           // true
s.Add(1)             // panics
```

Asserted side by side against a nil `map[string]struct{}` rather than by
inspection — `Len`, lookup, iteration, materialisation and write all agree
(`experiments/refcontainers`, `TestZeroValueMatchesNilMap`). **A zero container
is indistinguishable from a nil map on every operation.**

**`Vector` follows the same rule, and the slice analogy does not break it.**
Appending to a nil slice works, so a first reading says `Vector.Append` on a zero
value should too. It should not, and the reason is that `append` is not a
mutation — it *returns* a new slice, and the caller reassigns. The operation
`v.Append(e)` mutates in place, so its builtin counterpart is `m[k] = v`, which
panics. `v.At(0)` panics as `s[0]` on a nil slice does, because index 0 is out of
range for anything empty. One rule covers both backings.

#### Why the zero value cannot also be writable

The obvious mitigation is mixed receivers: value receivers for reads, pointer
receivers for writes, so a write lazily constructs the state and ADR `0002`'s
usable zero value survives intact.

**It reintroduces exactly the bug reference semantics exists to remove.**
Demonstrated rather than argued (`TestLazyInitDiverges`):

```go
var b lazySet[string]
c := b        // copy of a zero value
b.Add("one")  // lazily constructs b's state; c still has none
// b.Len() == 1, c.Len() == 0
```

A copy taken before the first write does not share. That is the divergence
`noCopy` exists to catch, and it would be back with the guard removed. **So pure
value receivers, and a read-only zero value, is not a preference — it is forced.**

#### Recovering the view hierarchy with embedding, not a conversion method

The version of option (b) priced earlier gave ordered views a `.Set()` method.
Struct **embedding** is better on every axis:

```go
type SortedSetView[T any] struct {
	SetView SetView[T]
}
```

- **Base methods are promoted**, so `sv.Has(x)` and `sv.Len()` work directly with
  nothing written — measured at 5.602 ns against 5.551 ns calling the base
  directly, and 5.505 ns through today's interface embedding. Identical.
- **The conversion is a field selector**, `sv.SetView`, not a method call. It
  reads as what it is, and at 0.2812 ns it is **2.3x cheaper than the interface
  embedding it replaces** (0.6395 ns).
- **It composes.** `[]SetView[T]{sv.SetView, other}` compiles, which was the
  concrete thing ADR `0013` decision 2 bought and the thing a `.Set()` method
  made awkward.

Verified in `TestEmbeddingRecoversSubstitution`. What remains is that the
selector must be *written*: substitution is explicit rather than implicit. That
is the whole of the hierarchy cost, and it is now a field access rather than a
method call.

#### `IsZero` is provided and rarely needed

Because reads are total, **the question a caller actually has is `Len() == 0`**,
and that works uniformly on every container and every view without knowing
whether either was constructed. `IsZero()` answers a narrower question — *was
this ever constructed* — which is exactly as rare as `m == nil` is for maps, and
exists for the same reason: parity with the builtin, not daily use.

That reframes the "two spellings" argument that ADR `0014` built its rejection
on. Under a complete option (b) there is one *emptiness* spelling (`Len() == 0`),
one *never-constructed* spelling (`IsZero()`), and both work everywhere. The
status quo has `== nil` for never-constructed and `Len() == 0` for empty — the
same two questions, differently spelled. **Option (b) does not add a vocabulary;
it swaps an operator for a method on the rarer of the two.**

#### What replaces `noCopy`

Nothing, for the divergence class: it stops existing. The *opposite* hazard
appears — sharing where a caller expected a copy — and no tool checks it. The
mitigation is the one the library already has: every container carries `Clone()`,
and the doc comment on each type says copies share. This is the same contract
`Map` has carried since ADR `0009` without incident, extended to the rest.

#### The consistency table this buys

| | status quo | complete option (b) |
|---|---|---|
| container handle | `*HashSet[T]` (pointer) | `HashSet[T]` (one-word value) |
| view handle | `SetView[T]` (interface) | `SetView[T]` (one-word value) |
| representation at the boundary | **changes** | same |
| copying a container | forbidden, `go vet` enforced | defined; copies share |
| copylocks caveat | live, and outside `go test` | **gone** |
| zero container, reads | **panics** | 0 / false / empty, as a nil map |
| zero container, writes | works (addressable) | **panics**, as a nil map |
| zero view, reads | panics | 0 / false / empty |
| "is it empty" | `Len() == 0` | `Len() == 0` |
| "was it constructed" | `== nil` | `IsZero()` |
| ordered → base view | implicit (embedding) | `sv.SetView` (field selector) |
| `HashDict` vs `Map` | two types, ADR `0007` asymmetry | **one type** |
| set algebra on a contract | impossible | still impossible |

#### What it would take

Ordered by how much judgement each needs, not by size:

1. **Reverse ADR `0002`'s usable-zero-value rule and narrow its
   eager-dereference rule** to writes only. Both were written for pointer
   receivers, where a nil receiver is unambiguously a bug; under reference
   semantics the zero value becomes a meaningful state.
2. **Accept the loss of error detection on unconstructed sources.**
   `NewVector(src.KeySlice()...)` on a zero `src` yields an empty container
   instead of panicking. This is `maps.Collect(nilMap)`'s behaviour and it is the
   price of the rule in §1; ADR `0017` made the bulk path common enough that it
   should be stated rather than discovered.
3. **Delete `HashDict`, promote `Map`.** Their method sets are already identical
   but for the constructor. Under option (b) the asymmetry that justified two
   types is gone, so this stops being a separate question and becomes a
   consequence.
4. **Rewrite every container as `struct{ st *state }` with value receivers**, and
   every view as `struct{ impl sealedImpl }`, with the nil check on every read
   — inside the returned closure, handing `yield` through, per the refined
   placement rule.
5. **Re-embed the ordered views** and add the field selector at every
   ordered-to-base call site.
6. **Delete `noCopy`, `container_layout_test.go`, and the copylocks note in
   `CLAUDE.md`.** Replace with a documented copies-share contract per type.
7. **Rewrite `contracts.go`'s assertions** from `(*HashSet[int])(nil)` to
   `HashSet[int]{}`.

#### The honest summary

**Option (b) is free at runtime, coherent as a design, and achievable.** What it
costs is: one accepted ADR reversed (`0002`'s usable zero value), one accepted
decision softened from implicit to explicit (`0013`'s substitution), a class of
error detection traded for builtin-matching semantics, and a whole-library
migration.

What it buys is the thing this ADR keeps circling: **the library stops having two
kinds of handle.** Whether that is worth the four costs above is a judgement
about how much the container/view split actually hurts in practice — and the
honest answer is that no call site in this repository currently demonstrates that
it does. **That, not cost, is what option (b) is still waiting on.**

## Option (d): the entire public API as interfaces

The one shape that answers the nil question completely. `HashSet[T]` stops being
a struct and becomes a sealed interface, as `SetView[T]` already is:

```go
type HashSet[T comparable] interface {
	Len() int
	Has(T) bool
	Keys() iter.Seq[T]
	KeySlice() []T
	Add(T)
	AddAll(...T)
	Union(HashSet[T]) HashSet[T]
	sealedContainer()
}

func NewHashSet[T comparable](vs ...T) HashSet[T]
```

**What it gets right, and nothing else does.**

- **One nil spelling, with no method.** `var s HashSet[int]` is a nil interface,
  so `s == nil` compiles and works — the same spelling views already use. No
  `IsZero`, no `IsNil`, no second vocabulary. Verified in `nil_test.go`.
- **The container/view split disappears.** Both are one-word sealed interfaces.
  A caller stops changing representation at the boundary, which is the thing ADR
  `0014` wanted and could not reach.
- **The seal survives in both directions.** A container carries
  `sealedContainer()` and a view `sealedView()`, so neither satisfies the other:
  `cannot use s … as ifaceSetView[string]: missing method sealedView`. Verified,
  not assumed.
- **ADR `0002`'s set-algebra ceiling dissolves.** `Union(HashSet[T]) HashSet[T]`
  satisfies itself, where `Union(*HashSet[T]) *HashSet[T]` cannot satisfy
  `Union(SetAlgebra[T]) SetAlgebra[T]` for want of covariant returns. **This is
  the only option that removes it**, and it has stood since `0002`.
- **`noCopy` and the copylocks caveat go**, as under any reference shape.
- **`HashDict` and `Map` both satisfy one `Dict[K, V]`**, which is a cleaner
  resolution of ADR `0007` than either reference option offers.

**What it costs**, from `experiments/refcontainers/` finding 4:

| 64 elements | pointer | interface | |
|---|---|---|---|
| `KeySlice` — ADR `0017`'s bulk path | 410.9 ns, 1 alloc | 421.7 ns, 1 alloc | **+2.6%** |
| `Has` | 4.292 ns | 4.948 ns | +15% |
| `Keys` | 319.6 ns, **0 allocs** | 384.1 ns, **3 allocs** | +20% |
| construct | 742.0 ns, 3 allocs | 796.2 ns, **5 allocs** | +7% |
| `Union` | 1.546 µs, 3 allocs | 1.741 µs, **6 allocs** | +13% |
| `Len` | 0.3734 ns | 1.181 ns | **+216%** |
| `At` | 0.4914 ns | 1.130 ns | **+130%** |
| **indexed loop over 64** | 16.11 ns | 68.92 ns | **+328%** |

**ADR `0017` did soften this, in exactly one place.** The bulk path is
**effectively free** through an interface, because a slice returned through a
dynamic call carries none of the per-call allocations an `iter.Seq` does. Since
`0017` made slices the currency of every bulk operation, the shape of the library
most exposed to dispatch is now the shape least affected by it. `0014` could not
have known that.

**Everything else got no better, because dispatch is a fixed cost and its share
tracks how cheap the operation is.** Against a map probe it is noise. Against
`Len` it is 216%. Against an index it is 130%, and against an *indexed loop* it
is **4.3x**, because `At` cannot inline and bounds-check elimination is
impossible through dispatch.

**Two costs are decisive, and they are not the average ones.**

**1. `Vector` becomes unusable for the thing it is for.** ADR `0015` accepted
`Vector` knowing an indexed loop costs ~24% against a raw slice, on the argument
that it earns its place at an API boundary and is explicitly *not* a replacement
for `[]T` locally. At 4.3x that argument stops working: `0015`'s call-site
evidence was gathered against a concrete type, and this shape invalidates it.
The container whose entire justification is handing out a slice safely would
become the one it is most expensive to read.

**2. The zero value stops being readable, not merely unwritable.** This is where
option (d) fails on its own terms. Every method on a nil container interface
panics — **including reads** — because a nil interface has no dynamic type to
dispatch to. A nil builtin `map` reads fine: `len` is 0, a lookup returns the
zero value, a range runs zero times. So (d) buys one nil *spelling* at the cost
of nil *semantics* that are strictly worse than the builtin it is imitating, and
worse than the reference-struct options, which at least get total reads. ADR
`0014` made this point about interface containers and it survives intact:
**it fails the stated goal.**

There is also a smaller structural cost worth naming: **every constructor becomes
the only way to obtain a container.** `var s HashSet[int]` is not a usable empty
set under (d) — it is a nil that panics — so ADR `0002`'s usable zero value is
not bent as it would be under (a) or (b), it is gone. Every declaration site
becomes a constructor call.

**Verdict on (d): rejected, and it is the furthest from adoptable of the four.**
It answers the nil-*spelling* question perfectly and the nil-*semantics* question
worst. It is the only option that removes the set-algebra ceiling, which is a
genuine and long-standing win — but paying 4.3x on indexed access and losing the
usable zero value to get it is not a trade this library should make. If the
set-algebra ceiling is ever worth attacking, it should be attacked directly,
without changing how every container is represented.

## Decision

**Rejected. The status quo stands.** But the reasoning is now narrower than
`0014`'s, and worth stating precisely, because the old reason no longer applies.

**The status quo already has one nil spelling.** `*HashSet[T]` is a pointer and
`SetView[T]` is an interface; `== nil` works on both, and neither needs a method.
Option (a) *adds* a second vocabulary, and is dominated by (b) — there is no
reason to take reference containers without also taking the view side, once the
view side is free.

**Option (b) is the live alternative, and it is closer than this status
suggests.** Built out in full above, it is free on every measured path, it
matches nil-map semantics exactly on the zero value, and the hierarchy cost
reduces to writing `sv.SetView` at ordered-to-base sites — a field selector that
is itself 2.3x cheaper than the interface embedding it replaces. It is not
rejected on cost or on coherence.

**It is rejected on the balance of four API costs against one benefit.** Option
(b) would reverse ADR `0002`'s usable zero value, make ADR `0013`'s substitution
explicit, trade a class of error detection for builtin-matching semantics, and
migrate the whole library. Against that it buys one thing: the library stops
having two kinds of handle.

**Nothing in this repository currently demonstrates that the two kinds of handle
hurt.** Every call site in `callsites_test.go` crosses the container/view
boundary without difficulty, because the crossing is a constructor call that was
going to be written anyway. That is the missing evidence, and it is the same
standard this library applies to every container it builds: **write the call site
that hurts, then change the design.**

**And ADR `0017` raised the price of total reads.** Under decision 3 of `0014`,
reads on a zero container succeed and return empty. That is exactly
`maps.Collect(nilMap)`'s behaviour and defensible on its own — but the library
now has a test asserting that *every* bulk method on *every* container panics on
a nil receiver, and the bulk path is `NewVector(src.KeySlice()...)`. Under
reference semantics an unconstructed source yields an empty slice and the
constructor silently produces an empty container. **The error detection that
ADR `0002`'s rule buys is worth more now that there are more places to lose it.**

The container-side benefits are real, are not disputed, and are larger than they
were: the representation is free, `noCopy` and the copylocks caveat would go, the
copy-divergence hazard would be dissolved rather than policed, and `HashDict`
would collapse into `Map`. They are still not worth a second nil vocabulary or a
lost hierarchy.

**And option (d) is rejected separately**, on grounds that have nothing to do
with nil spelling: it costs 4.3x on an indexed loop, which invalidates ADR
`0015`'s call-site evidence for `Vector`, and it makes the zero value panic on
reads where a builtin map returns empty. It is the only option that removes ADR
`0002`'s set-algebra ceiling; that is worth attacking directly if it is worth
attacking at all.

## What is now separately actionable

**`HashDict` and `Map` have converged, and that does not need reference
containers to resolve.** This is the one genuinely new thing on the table, and it
deserves its own ADR rather than being buried here.

Their method sets are identical but for the constructor. ADR `0009` built
`HashDict` to resolve ADR `0007`'s value/pointer asymmetry; ADR `0017` closed the
API gap without meaning to. Three options, none of which require this ADR to be
accepted:

- **Keep both.** The asymmetry stays: `Map` satisfies `MutableDict` as a value,
  `HashDict` as a pointer. This is the status quo and costs a whole type, its two
  view constructors, its two view structs, and a `CanViewHashDict` interface that
  is already identical to `CanViewMap`.
- **Delete `HashDict`, make `Map` the default.** Recovers ADR `0007`'s asymmetry
  in exchange for removing a type. Worth asking how much that asymmetry actually
  hurts now: both are reference-ish in practice, and `Map` additionally
  round-trips through `encoding/json` where `HashDict` is silently lossy.
- **Delete `Map`, keep `HashDict`.** Gives up builtin syntax, free conversion from
  `map[K]V`, and working serialization — the four reasons ADR `0009` kept it.

**Recorded, not decided.** It wants its own call-site evidence.

## What survives from ADR 0014

- **`experiments/reftypes/` is still valid** and is not superseded; this ADR adds
  a second harness rather than replacing the first.
- **`0014`'s "Choice B" is answered** — `Map` needs nothing further. See above.
- **`0014`'s "Choice A"** (`IsZero` vs `IsNil`) is still open in principle and
  still moot in practice.
- **`0014`'s "Choice C"** — whether anything replaces copylocks as a guard —
  remains unasked, because the guard remains.

## If this is revisited again

**The blocker has moved from cost to evidence.** ADR `0014` could reject this on
measurement; this one cannot. Option (b) as built out above is free, coherent,
and matches the builtin semantics it imitates. What it lacks is a call site that
demonstrates the problem it solves.

So the trigger is specific: **a realistic call site where the container/view
representation split costs something a reader can see** — a function that must
take both and cannot, a conversion written repeatedly, a bug caused by the
boundary. Write that first, in `callsites_test.go`, exactly as this library
requires before building any container. If it cannot be written, option (b) is
solving a problem this library does not have.

Two smaller triggers, either of which would move the balance:

- **An ADR that removes `HashDict`** and finds ADR `0007`'s asymmetry intolerable
  in practice rather than in principle. That work is independent and is recorded
  above as separately actionable.
- **A second copy-divergence bug** getting past `go test` because copylocks is
  not in its default vet subset. One such class of bug is what `noCopy` exists
  for; a second occurrence would be evidence that policing it is worse than
  dissolving it.

Nothing needs re-running. `experiments/refcontainers/` holds the current numbers,
and `optionb.go` holds a working sketch of the design.
