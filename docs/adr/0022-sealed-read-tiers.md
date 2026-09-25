# 22. Delete the mutation tiers; seal the read tiers

- **Status:** **Proposed.** Do not implement without review. Four of the five
  open questions are now **settled** (see *What to decide*): the guarantee is
  worth the plumbing, the token is named `callViewFirst`, `View()` lands with
  this ADR, and `SortedSetView`'s internal exception stands because it does not
  reach the exported API. The fifth — a replacement tripwire for the mutator
  vocabulary — has a recommendation awaiting a ruling.
- **Date:** 2026-09-25
- **Evidence:** `experiments/sealing/` (`RESULTS.md`), which also asserts in
  `run.sh` that the sealed assertion still fails to compile.
- **Relates to:** ADR `0018` (which created the shape interfaces and whose view
  structs this **keeps**), `0012` (whose `View()` ruling this revisits), `0020`
  (**abandoned**; this supersedes it in intent), `0013`/`0021` (the iteration
  cost, fixed by `0021`'s `Each` rather than by view shape), `0002` (whose
  vestigial satisfaction tests this removes).

## The problem

**The read shape interfaces promise a read-only contract they do not keep.**
`contracts.go` is candid about it:

> They are deliberately NOT sealed: containers satisfy them too, which is the
> point. […] a function that needs a read-only GUARANTEE takes a concrete view
> type.

That is honest, and it works — for a reader who knows it. The hazard is the one
that needs no bad intent:

```go
func Publish(ks containers.Keys[string]) { … }   // looks read-only

var k containers.Keys[int] = someMapSet          // a CONTAINER goes in
w, _ := any(k).(containers.MapSet[int])          // succeeds
w.Add(97)                                        // writes through
```

Measured, not hypothesised: `experiments/sealing` writes through a shape
interface this way. A caller reaching for `Keys[K]` at an API boundary gets a
type whose name says reads and whose behaviour permits writes, and **nothing in
the compiler, the tests, or the review diff flags it.** The failure is silent:
the cast compiles, tests pass, and the symptom appears later as a mutation
through a handle that was supposed to be read-only.

The library's own answer — "name the concrete view type" — is correct and cheap
(**1.86 ns, 0 allocations**; see below). It is also the thing a caller has to
remember at exactly the moment they are least likely to.

## The decision this ADR recommends

**Two changes, in this order.** The first stands on its own merits; the second
is what the first makes affordable.

1. **Delete `MutableKeys[K]` and `MutableKeyValues[K, V]`.** Functions that
   mutate generically declare the two or three methods they actually use, at the
   point of use.
2. **Seal the read tiers** — `Values`, `Keys`, `KeyValues`, `SortedKeys`,
   `SortedKeyValues`, `Positions`, `PositionValues` — with **one unexported
   token shared by the whole package**, declared by every read tier, implemented
   by every view, and **never by a container**. A container passed where reads
   are promised becomes a compile error; the caller writes `.View()`.

And one companion decision, separable if the reviewer prefers:

3. **Add `View()` to every container**, returning the identity view. It is free
   (0.39 ns, 0 allocations), and sealing makes it near-mandatory ergonomically.

`0018`'s **view structs stay exactly as they are.** This ADR replaces ADR
`0020`'s attempt to change them.

## Why the mutation tiers have to go first

`MutableKeys[K]` **embeds** `Keys[K]` (`contracts.go:93`). Sealing `Keys` forces a
three-way choice, and all three were built and compiled:

| | result |
|---|---|
| keep the embed, container supplies the token | compiles — **and the hole is fully reopened**; the write through a sealed tier still succeeds |
| keep the embed, container withholds the token | **compile error**: `MapSet does not implement MutableKeys (missing method viewOnly)` — the container can no longer state its mutation contract |
| **break the embed**, restating 13 signatures | works |

The first branch is the dangerous one: the seal is present, reads as a
guarantee, and buys nothing. The second is a dead end. The third works but
duplicates every read method the mutation tiers currently inherit.

**Deleting them is better than restating them, and better than keeping them.**

## What deleting them costs

Applied to the real tree; it builds, vets and passes the suite.

**The one genuine consumer improves.** `prune` (`map_test.go`) and
`pruneContainer` (`callsites_test.go`) are the only functions that take a
mutation tier as a parameter. They become:

```go
func prune[K comparable, V any](m interface {
	All() iter.Seq2[K, V]
	Delete(K)
}, keep func(V) bool)
```

Two methods instead of a fourteen-method tier, declared where they are consumed
— which is how Go's own minimal interfaces are written. It also documents what
`prune` needs, which the tier did not.

**The view guard gets more direct.** `views_test.go`'s check that a view does
not satisfy the mutation contract becomes an assertion on
`interface{ Set(string, int); Delete(string) }` — the actual hazard rather than a
proxy for it.

**Two tests disappear because they were vestigial.**
`TestContractSatisfaction` and `TestMutableSetSatisfaction` contained *only*
mutation-tier assertions, under comments reading *"the pointer does while the
value does not — ADR `0002`'s uniform pointer receivers"* — a rule ADR `0018`
removed. They were asserting a superseded design.

**What is genuinely lost:** the compile-time statement that every container
carries its mutators. The per-container tests call those mutators directly, so
the assertion was redundant; but it was a cheap tripwire for a new container, and
after this nothing replaces it. If that matters, a per-container ad-hoc
assertion is two lines and keeps the tripwire without the tier.

## Why sealing needs one shared token

An interface-typed value satisfies another sealed interface only if it
**declares** the same unexported method. What its dynamic type has is
irrelevant. With a token per interface, a view handed out as `MapSetView[T]`
would fail to satisfy `Keys[T]`, even though the implementation behind it has
both methods. This was not obvious and cost a rebuild to find.

So: one token, package-wide.

**Name it for the compiler error**, because that clause is the entire diagnostic
a caller receives:

```
cannot use s (variable of struct type containers.MapSet[int]) as
containers.Keys[int] value in argument to Publish:
containers.MapSet[int] does not implement containers.Keys[int]
(missing method viewOnly)
```

`viewOnly` hints; `callViewFirst` instructs. **Recommendation: `callViewFirst`.**
The name appears in godoc's rendering of the interface, so it is read in two
places and should work in both.

## The irreducible cost: an unexported mirror hierarchy

`MapSetView[NT]` is `struct{ impl Keys[NT] }`, and an **identity view passes the
container straight in as that implementation** (`views.go:61`:
`MapSetView[T]{impl: s}`). Sealing `Keys[NT]` forbids exactly that.

So the plumbing needs unsealed copies: `innerValues`, `innerKeys`,
`innerKeyValues`, `innerSortedKeys`, `innerSortedKeyValues`, `innerPositions`,
`innerPositionValues`. Seven unexported interface declarations, roughly 60 lines,
mirroring the public seven.

**The alternative is worse.** Wrapping each container in a one-word adapter that
supplies the token needs roughly 43 forwarding methods across five types —
more code, and forwarding is where transcription bugs live. Interfaces that
restate a method set cannot silently forward the wrong thing.

## What it measures

Line counts, `contracts.go`, each variant built and tested:

| | lines | |
|---|---|---|
| today | 133 | |
| **delete `Mutable*` only** | **109** | −24, a net simplification |
| **delete `Mutable*` + seal** | **173** | +40 over today |
| *seal, keep `Mutable*`* | *225* | *+92 — the branch this ADR avoids* |

Deleting the mutation tiers takes **52 lines off the sealing bill**: the 13
restated signatures, the tier declarations, and four assertions. That is the
majority of the cost, not the margin — the reason the two changes belong in one
ADR.

The performance case for keeping `0018`'s view structs, from
`experiments/sealing` (n=64, same implementation underneath):

| | StructView (today) | bare interface | 1-word struct |
|---|---|---|---|
| **zero value** | **reads empty** | **PANICS** | reads empty |
| `View()`, identity | **0.39 ns / 0** | 0 allocs | — |
| construct, converting | 13.5 ns / 1 | 13.3 ns / 1 | 23.8 ns / **2** |
| point read | 1.40 ns / 0 | 1.06 ns / 0 | 1.08 ns / 0 |
| iterate | 422 ns / 3 | 425 ns / 3 | 423 ns / 3 |
| → **concrete** view param | **1.86 ns / 0** | n/a | — |
| → sealed shape interface | 14.8 ns / 1 | **2.26 ns / 0** | 2.83 ns / 0 |

**Iteration and method calls are a wash**, so the struct wrapper costs nothing
where views are actually used. Its one penalty — boxing into a shape interface —
is 14.8 ns and one allocation, and **applies only to code generic over container
kind**, of which the library currently has none. Naming the concrete view type
avoids it entirely.

## Call sites

**Sketch — the hazard this closes:**

```go
// today: compiles, and Publish can write through ks
func Publish(ks containers.Keys[string]) { … }
Publish(mySet)

// after: a compile error naming the fix
Publish(mySet)          // MapSet[string] does not implement Keys[string]
                        // (missing method callViewFirst)
Publish(mySet.View())   // 0.39 ns, 0 allocations
```

**Sketch — what a read-only API should usually say instead**, and which needs no
seal at all:

```go
// the concrete view type IS the guarantee, and is cheaper: 1.86 ns, 0 allocs
func Publish(v containers.MapSetView[string]) { … }
```

Both belong in `callsites_test.go` as the stdlib-baseline-first diff CLAUDE.md
requires, if this is accepted.

## What this does NOT fix

**The seal protects the structure, never the contents.** Through a sealed,
un-castable view over `MapSet[*Item]`, every `*Item` stays freely mutable —
measured, not reasoned. And it is **not closable at any price**: Go cannot express
"contains no pointers" as a constraint. `comparable` admits pointers, a marker
interface cannot be implemented by builtin types, and type sets cannot describe a
struct's fields.

So deep immutability stays a documented convention, and that is consistent with
Go rather than a concession: `slices.Clone` states its shallowness in the doc
comment and is not named `ShallowClone`.

**The distinction worth stating, because these two look like the same call and
are not:** documentation is the weak choice when a cheap mechanism exists — the
cast has one, costing an unexported method. It is the *only* choice when no
mechanism exists at any price, which is the situation for shallowness.

This has a consequence for `View()`. ADR `0012` refused a bare `View()` on the
grounds that callers must *name the conversion, or name its absence*. `View()`
does name its absence, more concisely than `ViewMapSetIdentity`. But the word
"Identity" was doing less work than `0012` credited it with: it names the absence
of *conversion*, not the presence of *aliasing*, so a caller holding
`Map[K, *V]` was never warned by it either. **The right place for that warning is
a hazard call site in `callsites_test.go`**, which compiles and breaks on drift,
rather than a doc comment that does neither.

## Rejected alternatives

- **Views as sealed interfaces** (ADR `0020`'s shape). A nil interface panics on
  every method, so the zero value cannot read as empty — one of the library's
  three central rules, and the one `0018` was explicitly asked to preserve.
  Abandoned there, rejected here.
- **A one-word pointer-backed view struct.** Keeps the zero value and boxes
  free, but must still hold an interface to erase the container's type
  parameter, so its state is heap-allocated: **two** allocations at construction
  against one. Dominated.
- **Documentation only.** The failure mode is silent — the cast compiles and the
  tests pass — and a mechanism exists that costs ~60 lines of unexported
  interfaces. Coding agents are a specific case of the general problem, not a
  separate argument: the temptation peaks when the writer is stuck, which is when
  a doc comment in another file is least likely to be in view.
- **Seal and keep the mutation tiers.** +92 lines and a broken tier hierarchy,
  against +40. The measurements are above.
- **Sealing without excluding containers.** Measured as fully leaky. Worse than
  not sealing, because it reads as a guarantee.

## What to decide

1. **Whether the guarantee is worth ~60 lines of unexported plumbing** for a case
   with zero current instances. **SETTLED: yes.** Noted that sealing is cheapest
   to adopt *before* cross-container generic read-only code exists, and the mirror
   hierarchy is pure cost until it does — that is accepted.
2. **The token's name.** **SETTLED: `callViewFirst`.** The name is the entire
   diagnostic a caller receives, and instructing beats hinting. It reads oddly in
   godoc's rendering of the interface; that is the accepted price.
3. **Whether `View()` lands with this or separately.** **SETTLED: with this.**
   Sealing makes it near-mandatory — every read-only boundary now needs a view —
   so shipping the seal without it would be shipping the friction without the
   remedy. It is free (0.39 ns, 0 allocations) and returns the ordinary view type.
   This **revisits ADR `0012`**, which refused a bare `View()`: see *What this
   does NOT fix* for why `Identity` was doing less work than `0012` credited, and
   why the aliasing warning belongs in a hazard call site rather than a name.
   `View<Container>Identity` is superseded by it.
4. **Whether a replacement tripwire is wanted** for the mutator vocabulary.
   **OPEN — recommendation: yes.**

   Today four assertions at the bottom of `contracts.go` make `go build` fail if a
   container drifts from ADR `0017`'s vocabulary (`Add`/`AddAll`, `Set`/`SetAll`,
   `Delete`/`DeleteAll`). They catch a new container that forgets `DeleteAll`, a
   rename applied to one container but not its sibling, and a signature drift from
   `...K` to `[]K`. Per-container tests cover each container's own mutators, but
   **nothing else asserts uniformity across containers**, which is what these
   assert. Deleting the tiers deletes the type the assertion needs.

   The replacement keeps them **unexported** and **not embedding the read tiers**:

   ```go
   type mutatesKeys[K any] interface {
   	Add(K); AddAll(...K); Delete(K); DeleteAll(...K)
   }
   type mutatesKeyValues[K, V any] interface {
   	Set(K, V); SetAll(...Entry[K, V]); Delete(K); DeleteAll(...K)
   }
   ```

   Unexported, so no caller can take one as a parameter — the hazard that
   motivated deleting `Mutable*` cannot arise. Non-embedding, so a container never
   needs the seal token — the conflict that *forced* deleting `Mutable*` cannot
   arise either. Verified: it builds, and renaming `MapSet.DeleteAll` breaks
   `go build` with `missing method DeleteAll`. ~10 lines.

   The counter-argument is that it re-adds what this ADR deletes. The answer is
   that both properties that made `Mutable*` a problem — exported, and embedding
   `Keys[K]` — are absent here.
5. **Whether `SortedSetView` is an exception worth keeping.** **SETTLED: keep it,
   because it is invisible.** It holds its container concretely
   (`views.go:135`) rather than through an interface, since its keys never
   convert, so it needs no mirror interface. Its **exported** surface is the same
   shape as every other view — `IsZero`, `Len`, `Has`, `Keys`, `KeySlice`, plus the
   ordered reads it carries for being sorted — and the field type is unexported.
   Uniformity of the exported API is what matters; internal uniformity is not
   worth an interface nothing dispatches through.

## Implementation order, once accepted

1. Delete `MutableKeys`/`MutableKeyValues`; rewrite `prune` and `pruneContainer`
   to declare the two methods they use; rewrite the `views_test.go` guard as an
   ad-hoc mutator assertion; delete `TestContractSatisfaction` and
   `TestMutableSetSatisfaction`.
2. Add the unexported tripwire (pending question 4).
3. Add the mirror hierarchy and repoint the view structs' `impl` fields at it.
4. Seal the read tiers with `callViewFirst`; implement it on the five view types;
   drop the container-side read assertions.
5. Add `View()` to the five containers; supersede `View<Container>Identity`.
6. Add both call-site sketches to `callsites_test.go`, plus the pointer-aliasing
   hazard case, as a stdlib-baseline-first diff.
7. Update CLAUDE.md: the shape-interface paragraph ("**not** sealed") is now
   wrong; the `View()` prohibition under *Shape and naming* is superseded; add
   `callViewFirst` and the aliasing caveat to the three-rules section.
