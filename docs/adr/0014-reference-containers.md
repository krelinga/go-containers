# 14. Containers as reference types

- **Status:** **Rejected.** The status quo stands: ADR `0002`'s container shape
  and ADR `0009`'s `HashDict` are unchanged. See "Why this was rejected". The
  document is kept in full because the measurements and the dead ends are the
  valuable part.
- **Date:** 2026-09-08
- **Evidence:** `experiments/reftypes/` (`RESULTS.md`), with context from
  `experiments/copycost/`, `dispatch/`, `hashdict/` and `viewiface/`.
- **Relates to:** ADR `0002` (the container shape this replaces), `0007` (the
  value/pointer asymmetry this dissolves), `0009` (`HashDict`, which this
  removes), `0013` (views as sealed interfaces, which this aligns containers
  with), `0004` and `0006` (bulk insert and sized construction, which the
  removal of `HashDict` disturbs).
- **Would have superseded:** ADR `0002`'s `noCopy` field, uniform pointer
  receivers, usable-zero-value rule and prohibition on nil special cases; ADR
  `0009` entirely. **None of that happened — those ADRs remain binding.**

## Context

ADR `0002` fixed the shape of a container: a struct with `noCopy` declared
first, uniform pointer receivers, held and passed as `*HashSet[T]`. Copying is
forbidden and `go vet`'s copylocks enforces it. The rule exists for one reason:
a slice header must be *replaced* when it grows, so a container holding one
inline diverges when copied. That is the aliasing surprise this library was
begun over.

`Map[K, V]` is the exception. Being a defined `map[K]V`, it is already a
reference type — copied freely, copies sharing — and ADR `0007` recorded the
asymmetry that produced: `Map` satisfies the contracts *as a value*, every other
container *as a pointer*. ADR `0009` resolved that by building `HashDict`, a
shape-conforming hash dict, and demoting `Map` to an adapter.

Two things have changed since.

**Views became sealed interfaces** in ADR `0013` — passed by value, copied
freely, copies referring to the same container. So a caller now crosses a
representation boundary going from container to view: `*HashSet[T]` in,
`SetView[T]` out. The two halves of the library disagree about how a handle is
held.

**The reason for `noCopy` can be removed rather than policed.** If the state
lives behind a pointer, a copy *is* the same container, and there is no
divergence to catch.

This ADR proposes making every container a reference type, resolving ADR `0007`'s
asymmetry from the other end — everything satisfies as a value, `Map` stops being
the exception, and the container built to displace it has no remaining job.

## Findings

From `experiments/reftypes/`.

### The representation is free

| | `Has` | `Len` | `Add` | `All` (64) | construct |
|---|---|---|---|---|---|
| `*PtrSet` — ADR `0002`'s shape | 2.933 ns | 0.366 ns | 73.6 ns | 360.0 ns | 26.4 ns |
| struct wrapping a pointer | 2.993 ns | 0.374 ns | 72.7 ns | 325.9 ns | 26.2 ns |

Indistinguishable, allocations included. The extra hop is not extra: today a
method reads `s.m` having already dereferenced `s`; a reference struct reads
`s.st.m` where `s` is one word in a register. One load either way. **What
reference semantics costs is paid in rules, not cycles.**

### Builtin-map nil semantics is affordable, and the check has a correct place

A nil check on reads costs **+8.5% on a point read** (2.993 → 3.247 ns) and
nothing measurable on iteration, where it runs once per `All` rather than once
per element. Iterating a zero container costs 0.88 ns and no allocations.

But placement matters more than the branch does:

| `All` over 64 elements | | allocs |
|---|---|---|
| check **before** the closure, early-returning an empty one | 360.4 ns | **2** |
| check **inside** the closure | 331.6 ns | **0** |

Two return statements yielding two different closures puts the closure on the
heap. One closure, one return, keeps it on the stack.

### Only a struct can reproduce the semantics; only an interface gets the syntax

| | `x == nil` | `Len` when zero | read when zero | range when zero | write when zero |
|---|---|---|---|---|---|
| builtin `map` | ✓ | 0 | zero value | 0 iterations | **panics** |
| reference struct, checked | ✗ | **0** | **zero value** | **0 iterations** | **panics** |
| sealed interface | **✓** | panics | panics | panics | panics |

A nil interface has no dynamic type, so *every* method call panics; there is no
way to make a read on it succeed. Only `map` itself gets both, which is why
`Map[K, V]` can be a defined `map[K]V` and nothing else can follow it.

Two representations that would have squared this do not exist: methods cannot be
declared on a defined pointer type (`invalid receiver type`), and a defined slice
type needs a pointer receiver to grow, which forfeits reference semantics.

### The interface form costs 23%, and allocates on every `All`

| | `Has` | `Len` | `All` | `All` allocs |
|---|---|---|---|---|
| reference struct | 2.993 ns | 0.374 ns | 325.9 ns | 0 |
| sealed interface | 3.672 ns (**+23%**) | 1.025 ns (**+174%**) | 396.7 ns | **3, 64 B** |

Dispatch is ~0.7 ns as `dispatch` measured, but as a *share* it is large, because
container operations are cheap — `Len` is the extreme, being nothing but the
load. The allocations are the heavier cost: an `iter.Seq` returned through a
dynamic call cannot be stack-allocated at the range site, so iteration allocates
three times **per call**, and that does not amortise.

### The one thing the interface form wins

ADR `0002` recorded that set algebra cannot go on a contract, for want of
covariant returns. Verified:

```
*PtrSet[int] does not implement SetAlgebra[int] (wrong type for method Union)
        have Union(*PtrSet[int]) *PtrSet[int]
        want Union(SetAlgebra[int]) SetAlgebra[int]
```

If the container **is** the interface the two types coincide and it compiles. A
ceiling standing since `0002` would disappear — and it is the only thing on that
side of the ledger.

## What was proposed

Rejected — recorded as proposed, not as decided. Nothing below is binding.


### 1. Every container is a reference type

A small struct wrapping a pointer to shared state, with **value receivers
throughout**:

```go
type HashSet[T comparable] struct{ st *hashSetState[T] }

func NewHashSet[T comparable](vs ...T) HashSet[T]

func (s HashSet[T]) Has(t T) bool
func (s HashSet[T]) Add(vs ...T)
```

Copies share. `s2 := s1` is defined behaviour, not a bug, and needs no vet rule.

### 2. `noCopy` is removed

There is nothing left to catch. This also retires the hazard `CLAUDE.md` records
at length — that copylocks is not in `go test`'s default vet subset, so a
shallow-copy bug passes `go test ./...` — because the bug class stops existing.

### 3. Zero values follow builtin maps: reads are total, writes panic

```go
var s HashSet[int]
s.Len()          // 0
s.Has(1)         // false
for range s.All() {}  // no iterations
s.Add(1)         // panics
```

Reads carry a nil check; **the check goes inside the closure** for any method
returning an iterator, per the finding above.

This reverses two accepted rules, deliberately. ADR `0002` required a *usable*
zero value — `var s HashSet[int]; s.Add(10)` works today via addressability — and
banned nil special cases. Both were written for pointer receivers, where a nil
receiver is a bug. Under reference semantics the zero value stops being a bug and
becomes a meaningful state: the empty container you may read but not write. That
is an established Go idiom rather than an accident, and it is the semantics a
reader already has for maps.

The eager-dereference rule narrows accordingly: it still governs **writes**,
which must panic at the call, and no longer governs reads, which have nothing to
panic about.

### 4. Every container has `IsZero() bool`

```go
func (s HashSet[T]) IsZero() bool
```

A struct cannot be compared to nil, so the check needs a spelling. Uniform across
every container including `Map`, whose implementation is `return m == nil` and
which additionally supports `== nil` because it is a map.

The name is **not settled** — see "Choices still to make".

### 5. `Map` is the hash dict; `HashDict` is removed

`Map` already has the target semantics, and adds builtin syntax, free conversion
from `map[K]V`, and working `encoding/json`. ADR `0009` built `HashDict` to
resolve ADR `0007`'s value/pointer asymmetry; decision 1 resolves it for every
container at once, so `HashDict` has no remaining job.

`Map` is missing API that `HashDict` has, which is a migration cost, not a
detail — see "Choices still to make".

### 6. Containers keep concrete return types

`Clone`, `Union`, `Intersect` and `Difference` return `HashSet[T]`, not an
interface. ADR `0002`'s ceiling therefore stands: set algebra still cannot go on
a contract. Decision 7 explains why that is accepted.

### 7. Containers are structs, not interfaces

The interface form is rejected. It costs 23% on point reads and three
allocations per `All`, on operations a container performs constantly, in exchange
for nil-comparability and the set-algebra ceiling. ADR `0013` accepted dispatch
for *views* because a view is a boundary object; a container is not, and the same
trade does not transfer.

## Consequences that would have followed

- **The library becomes uniform in how a handle is held.** Containers and views
  are both one-word, freely-copied, reference-semantic values. A caller stops
  changing representation at the container/view boundary.
- **ADR `0007`'s asymmetry is gone.** Every container satisfies every contract
  as a value. `Map` is no longer special-cased.
- **Contract declarations need no change**, though their satisfaction does:
  `Elems`, `Elems2`, `MutableSet` and `MutableDict` are satisfied by values
  instead of pointers, so `contracts.go`'s assertion block is rewritten from
  `(*HashSet[int])(nil)` to `HashSet[int]{}` throughout.
- **Views change less than they look, but they do change.** A view holds
  `HashSet[T]` instead of `*HashSet[T]` — both one word — and the seal is
  unaffected, since a container still lacks `sealedView`. But decision 5 deletes
  `ViewHashDict`, `ViewHashDictIdentity` and their two structs outright, and
  makes `CanViewHashDict` redundant with `CanViewMap`, which are identical
  interfaces today. Nine view structs become seven, and the viewer vocabulary
  loses a name.
- **`var d HashDict[K, V]; d.SetAll(src)` stops working.** Today `setAll` lazily
  creates the backing map. Under decision 3 the equivalent panics, because a zero
  container is not writable. This is the usable-zero-value reversal made
  concrete, and it will appear at real call sites.
- **An unconstructed container passed as a source now yields empty instead of
  panicking.** `CollectSortedSet(zeroSet)` returns an empty set where today a nil
  `*HashSet` panics at the call, because ADR `0002`'s eager-dereference rule made
  it. Under decision 3 reads are total, so there is nothing to panic about and
  the mistake goes undetected. This is exactly `maps.Collect(nilMap)`'s behaviour
  and is the price of matching builtin maps; it is a real loss of error detection
  and should be stated rather than discovered.
- **Struct containers are comparable; `Map` is not.** A one-field struct compares
  by its state pointer, so `s1 == s2` is identity comparison and `HashSet[T]` can
  be a map key. `Map[K, V]` supports neither — `map can only be compared to nil`.
  A residual asymmetry, smaller than the value/pointer one it replaces, but real,
  and it falls out of `Map` being a map rather than a wrapper.
- **Serialization improves for hash dicts and is unchanged elsewhere.** `Map`
  round-trips through `encoding/json` because it is a map; the sorted containers
  remain silently lossy, since they still have only unexported fields. The
  library-wide decision `CLAUDE.md` calls for is still owed.
- **The copy hazard is dissolved rather than policed**, which removes a class of
  bug and a class of tooling caveat together.

## Why this was rejected

**There is no way to give containers reference semantics without ending up with
two spellings of "is this handle empty", or a worse hazard than the one being
fixed.**

The chain is short and every link is measured.

1. A reference container must be a struct wrapping a pointer, because the two
   representations that would be nil-comparable are unavailable: methods cannot
   be declared on a defined pointer type, and a defined slice type needs a
   pointer receiver to grow, which forfeits the reference semantics that
   motivated it.
2. **A struct cannot be compared to nil.** So a container's emptiness check has
   to be a method — `IsZero()` or `IsNil()`.
3. Views are sealed interfaces after ADR `0013`, and theirs is `== nil`. Adding
   `IsNil()` to a view interface does not unify anything, because calling a
   method on a nil interface panics: a caller would need
   `v != nil && !v.IsNil()`, which is worse than either spelling alone.
4. Making views structs to match costs ADR `0013`'s hierarchy — struct embedding
   is not subtyping — plus an allocation whenever a view is passed to `Elems2`,
   which is what every sized constructor takes.
5. The hybrid recovers the hierarchy, because a struct *satisfies* an interface
   even though it cannot subtype one. But it keeps the two spellings, merely
   relocating the split inside the view vocabulary, **and** reintroduces Go's
   typed-nil trap: a zero ordered view reports `IsNil() == true`, yet held as the
   base interface reports `== nil` as **false** while panicking on use.

So the options are: two spellings, two spellings plus a trap, or one spelling
bought with a lost hierarchy and a per-call allocation on the constructor path.
The status quo has exactly one spelling for containers (`*HashSet[T]` is a
pointer; `== nil` works) and one for views (`== nil`), and it needs no method at
all. **The change would have added nil semantics rather than unified them**,
which is the opposite of the reason for making it.

The container-side benefits were real and are not disputed: the representation
is free, `noCopy` and the copylocks caveat would have gone, `HashDict` would have
collapsed into `Map`, and the copy-divergence hazard would have been dissolved
rather than policed. They were not worth buying a second nil vocabulary.

## What survives the rejection

- **`experiments/reftypes/` stands as evidence** and its conclusions are durable
  regardless: a reference struct is free relative to a pointer; a nil check costs
  ~8.5% of a point read; the check must sit inside the closure `All` returns or
  it costs two allocations; a container interface costs 23% on reads and three
  allocations per `All`.
- **ADR `0013` gained an erratum** that has nothing to do with this proposal and
  everything to do with shipped code: iterating a view allocates three times per
  call, because `All` returns a closure through a dynamic call. That was found
  here and is recorded against `0013`.
- **`experiments/viewiface/` §9 now covers the struct-wrapped view shapes**,
  which is the material any future attempt to unify the two vocabularies will
  need.
- **ADR `0002`'s rules are re-affirmed rather than merely left alone.** The
  usable zero value and the ban on nil special cases were examined directly
  against a designed alternative and kept.

## Choices that would have needed making

Moot now. Recorded because they are the questions any future attempt at this
must answer, and two of them were nearly settled.

### A. `IsZero()` or `IsNil()`

Both were argued when this was raised, and the argument is genuinely balanced.

- **`IsZero`** is literally accurate — the value is a zero struct, not a nil
  reference — and matches `time.Time.IsZero`. `IsNil` is `reflect.Value`'s, where
  the receiver really is nil-able.
- **`IsNil`** describes what a reader already understands. The whole design is
  "behaves like a nil map", and a nil reference is a more commonly held idea than
  a zero reference. It also reads consistently with `Map`, where the
  implementation genuinely *is* `m == nil`.

Working assumption while this was proposed: `IsZero`.

A third consideration surfaced in review, which cuts across the choice rather
than settling it: **views would still use `== nil`.** They are sealed interfaces
after ADR `0013`, so a nil view is spellable. Declaring `IsNil()` on the view
*interfaces* does not fix it — calling a method on a nil interface panics, so a
caller would need `v != nil && !v.IsNil()`, which is worse than either spelling
alone. The only way to give views the container's spelling is to make views
structs wrapping a sealed interface.

Measured in `experiments/viewiface` §9. Dispatch is not what that costs:

| | construct | `Get` | `All` | pass to a foreign interface |
|---|---|---|---|---|
| bare interface (today) | 13.17 ns, 1 alloc | 13.22 ns | 569 ns, 4 allocs | **2.210 ns, 0 allocs** |
| `struct{iface}` | 13.21 ns, 1 alloc | 13.58 ns | 572 ns, 4 allocs | **14.93 ns, 1 alloc** |
| `struct{ptr}` | 23.70 ns, **2 allocs** | 13.36 ns | 566 ns, 4 allocs | 2.775 ns, 0 allocs |

`Get` through a wrapper is +2.7%, construction is identical, and iteration is
unchanged. Two real costs:

- **Passing a view to a foreign interface boxes.** `Elems2` is the case that
  matters, since every sized constructor takes it: 6.8x and an allocation,
  because a two-word struct is not pointer-shaped. The one-word `struct{ptr}`
  form avoids it and pays a second allocation at construction instead.
- **ADR `0013`'s hierarchy does not survive.** Struct embedding is not subtyping:
  `cannot use sv (variable of struct type WrapSortedView[...]) as
  WrapIface[...]`. Decision 2 of `0013` — an ordered view usable wherever the
  base is wanted — depends on interface embedding. Substitution becomes an
  explicit `sv.Dict()` conversion, which is **free at 2.74 ns and 0 allocations**
  but must be written at every ordered-to-base call site, and a `SortedDictView`
  cannot enter a `[]DictView` without one.

Something does get simpler: a struct whose only field is unexported cannot be
built populated outside the package, so `sealedView()` becomes unnecessary and
`v.(*HashDict[K, V])` fails to compile because `v` is not an interface.

So the choice is between **two spellings** (`c.IsNil()` for containers, `v == nil`
for views) and **one spelling** bought with an allocation on the `Elems2` path
and an explicit conversion wherever an ordered view feeds a base-typed boundary.

Worth weighing how much the check is actually used. A container's zero value is
reachable by declaration — `var c HashSet[int]` — so its check is load-bearing.
A view only ever comes from a constructor, which never returns nil, so a nil view
means someone declared one and never assigned it. The uniformity is real; the
thing being unified may be rare on the view side.

Go itself splits the same way, which is either a precedent or an excuse:
`time.Time` is a struct and uses `IsZero`, while maps, slices and interfaces use
`== nil`.

**A hybrid was evaluated and is not recommended.** Keeping `SetView` and
`DictView` as pure interfaces while making only `SortedSetView` and
`SortedDictView` structs has one genuine merit: a struct *satisfies* an interface
even though it cannot subtype one by embedding, so ADR `0013`'s substitution
survives implicitly, with no explicit conversion. But it fails on its own terms
and adds a hazard:

- **The split moves rather than closes.** Hash views stay `== nil`, ordered views
  become `IsNil()` — two spellings inside the view vocabulary, where which one
  applies depends on the static type in hand rather than on the value.
- **It reintroduces the typed-nil trap.** A zero ordered view reports
  `IsNil() == true`, but substituted into the base interface it is a non-nil
  interface holding a zero struct: `v == nil` is **false** while calling it
  panics. With both types as interfaces a nil view is nil at every static type.
- Substituting a two-word struct into the base interface costs 30.06 ns and an
  allocation against 13.39 ns and none, every time. A one-word form fixes that
  and doubles construction.

So the choice stands at two: all views stay interfaces, or all views become
structs. The hybrid keeps the two spellings *and* adds a trap neither pure option
has.

### B. What `Map` must gain before `HashDict` can be removed

`HashDict` has API `Map` does not: `NewHashDict`, `CollectHashDict`,
`CollectHashDictSeq`, `SetAll` and `SetAllSeq`. `MutableDict` does not require
the bulk forms, but ADR `0004` and the `CollectX` family are a package-wide
convention, and dropping them for the default hash dict would be a visible hole.

Adding them is mostly mechanical, with one semantic wrinkle: **an existing map
cannot be presized.** `CollectMap(src)` can `make(Map[K, V], src.Len())` and get
ADR `0006`'s win in full, but `m.SetAll(src)` on an already-made map cannot —
which matches what `HashDict.SetAll` does today anyway, since ADR `0004`
rejected rebuilding to presize except at k ≥ 4n.

To decide: which of these `Map` gains, and whether `CollectMap` is worth having
given `maps.Collect` exists.

### C. Whether anything replaces copylocks as a guard

`noCopy` caught a real bug class. Reference semantics removes that class, but it
introduces the opposite hazard — sharing where a caller expected a copy — which
no tool checks. Whether that wants a documented `Clone()` convention, or nothing
at all, is open.

## Alternatives weighed inside the proposal

All moot, since the proposal itself was rejected. Kept because the reasoning
separating them is the part worth re-reading.

### Keep ADR `0002`'s shape

The status quo. Costs nothing at runtime, and keeps the copylocks guard and the
usable zero value. Rejected because it leaves the container/view representation
split in place, keeps `HashDict` alive purely to paper over ADR `0007`, and
retains a hazard — copies that silently diverge — that the alternative removes
outright rather than detecting.

### Containers as sealed interfaces

Would give nil-comparability, unify containers and views as literally the same
kind of thing, and remove the set-algebra ceiling. Rejected on cost: 23% on point
reads, 174% on `Len`, and three allocations per `All` call, none of it
amortisable, on the operations a container exists to perform. It also *fails the
stated goal*, since a nil interface panics on reads where a nil map does not.

### Defined pointer or slice types

Not expressible. `type SortedSet[T any] *state[T]` cannot have methods; `type
SortedSet[T any] []T` needs a pointer receiver to grow, which forfeits the
reference semantics that motivated it.

## Follow-ups

Two of the three survive the rejection, because they were never about this
proposal.

- **ADR `0013` understated the cost of views.** Iterating a view allocates three
  times per call, because `All` returns a closure through a dynamic call;
  `viewiface` measured `Get` and never `All`. Live in the shipped library, and
  now recorded against `0013`.
- **Serialization** remains owed a library-wide ADR. Unchanged by the rejection:
  `Map` round-trips because it is a map, and every other container is still
  silently lossy.
- ~~`LinkedList` (ADR `0010`) inherits this~~ — moot. `LinkedList` follows ADR
  `0002`'s shape like every other container.

## If this is revisited

The blocker is nil semantics, not cost, so a future attempt needs a new answer to
one question: **how does a caller ask "is this handle empty" with one spelling
across containers and views?** Anything that leaves two spellings, or that boxes
a zero struct into an interface, lands back here. `experiments/reftypes/` and
`experiments/viewiface/` §9 hold the measurements; nothing needs re-running.
