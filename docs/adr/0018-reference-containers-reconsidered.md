# 18. Containers as reference types, reconsidered

- **Status:** **Rejected — on a narrower basis than ADR `0014` gave.** The status
  quo stands. But `0014`'s *decisive* argument is dead, and what replaces it is
  weaker, so this should be read as a closer call than the one it supersedes.
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

- **ADR `0013` decision 2 dies.** An ordered view can no longer be handed to a
  base-typed boundary implicitly — struct embedding is not subtyping. Every
  ordered-to-base call site grows a `.Set()` or `.Dict()`, and a
  `SortedDictView` cannot enter a `[]DictView` at all without one. Free at
  runtime; a constant tax to read and write.
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
Option (a) *adds* a second vocabulary. Option (b) keeps one vocabulary but makes
it a method call — measured identical to `== nil`, so a lateral move rather than
an improvement — and spends ADR `0013` decision 2 to do it — an ordered view can
no longer be handed to a base-typed boundary implicitly, and a `SortedDictView`
cannot enter a `[]DictView` without an explicit conversion at every site.

**That conversion is free at runtime and not free to read.** The measurement says
0.2825 ns. The cost is that every ordered-to-base call site grows a `.Dict()`,
which is the kind of tax that is invisible in a benchmark and constant in a
codebase.

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

The blocker is unchanged in kind and cheaper in degree: **how does a caller ask
"is this handle empty" with one spelling across containers and views, without
giving up ADR `0013`'s substitutability?** Two of the three historical answers to
that are now cheaper than they were, which is why this ADR exists — but none of
them is free, and the status quo answers the question with no method at all.

What would change the answer: a call site where the container/view representation
split actually hurts, or an ADR that removes `HashDict` and finds ADR `0007`'s
asymmetry intolerable in practice rather than in principle. Neither exists yet.
Nothing needs re-running; `experiments/refcontainers/` holds the current numbers.
