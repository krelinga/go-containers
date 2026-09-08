# 16. A view over a plain slice, and no `Slice` type

- **Status:** Proposed. Note that this ADR changed subject while being written:
  it set out to propose `Slice[T] []T` and the measurements turned it into a
  proposal to add a view and no type. The rejected adapter is kept in full below.
- **Date:** 2026-09-08
- **Evidence:** `experiments/sliceadapter/` (`RESULTS.md`), with context from
  `experiments/vectorcost/`, `copycost/` and `views/`.
- **Relates to:** ADR `0001` (views against defensive copies — the justification
  that survived), `0011` and `0013` (what a view is, and the sealed interfaces
  this adds a producer to), `0015` (`Vector`, whose follow-up proposed the
  adapter, and whose decision 5 named the interface this renames), `0010`
  (`LinkedList`, whose reservation of `List` decides the new name), `0009`
  (`Map`, the model the adapter was drawn from), `0006` and `0008` (sized
  construction and naming, both of which bore on the rejected adapter).
- **Supersedes, if accepted:** ADR `0015` decision 5's name for the sequence view
  interface. `VectorView` becomes `IndexedView`; nothing else in `0015` changes.

## Context

ADR `0009` makes `Map[K, V]` a defined `map[K]V` and calls it an **adapter**, not
a default. `CLAUDE.md` records when to reach for one: a free conversion from an
existing builtin, builtin syntax, passing the result where the builtin is
expected, and working `encoding/json`.

ADR `0015` recorded the parallel as a follow-up: `Slice` would be to `Vector`
what `Map` is to `HashDict`. ADR `0008` had already pre-authorised the name,
saying a future `Slice[T] []T` would take the same exception `Map` does.

The parallel holds in outline and breaks in one specific place. Following that
break, and then measuring what was left, moved this ADR off proposing an adapter
entirely — it proposes a view and no new type. The adapter analysis is kept as
the primary rejected alternative.

## The wall in the analogy, and why the adapter was dropped

**A map is a reference type. A slice is not.**

`Map`'s `Set` and `Delete` work on a value receiver because a map header points
at shared state, so mutating through any copy is visible through all of them. A
slice's **length lives in the header**, which *is* the value. Measured:

```
Set through a value receiver sticks=true; Append through one sticks=false
```

Writing an existing element goes through the shared backing array and sticks.
Appending updates a copy of the header and is discarded. No receiver choice
escapes this: a pointer receiver would work but would forfeit the value
semantics that make an adapter an adapter, and would mean `Slice[T]` no longer
satisfies anything a `[]T` can be passed to.

**So a slice adapter can read and can write in place, but cannot change its own
length.** A `Slice[T]` would have had no `Append` — which is the first sign it
was not the thing `Map` is.

It also fixes nothing:

```
appends off a shared adapter header still alias=true
```

Naming a slice type changes no slice semantics. Every hazard ADR `0015` records
survives. An adapter is an ergonomic convenience, exactly as `0009` says of
`Map`; `Vector` remains the answer where the hazards matter.

What killed it was not either of these, but the measurements below: everything an
adapter was wanted for turned out to be better served without one.

## Call sites

Phase 1 sketches. These do not compile.

### Task J — handing out read-only access to a slice you already have

```go
// sketch -- today
type Config struct{ hosts []string }

// Copy on every call: O(n), and O(n²) in a caller's loop...
func (c *Config) Hosts() []string { return slices.Clone(c.hosts) }

// ...or hand out the interior and let the caller write into it.
func (c *Config) Hosts() []string { return c.hosts }

// Or migrate the field to a Vector, which copies once at construction and
// changes how every other line in the type reads.
```

Proposed:

```go
// sketch -- the field does not change at all
type Config struct{ hosts []string }

func (c *Config) Hosts() containers.IndexedView[string] {
	return containers.ViewSliceIdentity(c.hosts)
}
```

**Zero copy, O(1), and the caller cannot write.** The field is still a `[]string`
— `append`, `range`, indexing and `encoding/json` all keep working — and it
gained the ability to be handed out read-only, which a slice cannot otherwise do
at any price short of copying.

This is ADR `0001`'s accessor problem, and it is the only task here that is a
win. It is a *safety* win, which is what the call-site convention asks for.

An earlier draft of this ADR reached it through a `Slice[T]` adapter, making the
field `containers.Slice[string]`. That works and is measurably identical — and
the adapter turns out to contribute nothing to it, which is what moved this ADR
off proposing one.

### Task K — a sequence field that round-trips through JSON. **Not a win.**

```go
// sketch -- today
type Config struct {
	Hosts containers.Vector[string] // marshals as {} -- contents silently lost
}

type Config struct {
	Hosts []string // round-trips correctly
}
```

A plain slice already does this. The draft that proposed `Slice[T]` counted JSON
as a reason for it, on the grounds that a `Slice[T]` field would round-trip
*and* carry the library's vocabulary. But the vocabulary is only needed to
satisfy `Elems[T]`, Task L shows that is not worth having for a slice, and
decision 1 leaves the field a plain `[]string` anyway — so the round-trip was
never at risk.

**Recorded because it was nearly used as a justification.** What it actually
shows is that `Vector` marshals as `{}`, which is the serialization ADR this
library has owed since `0002`, and is not fixable here.

### Task L — feeding a plain `[]T` to a sized bulk operation. **Not a win.**

This was proposed as the justification for the type. It is not one.

ADR `0015` made `Vector.Append` non-variadic, so bulk-appending a slice today
goes through a bare iterator with no length:

```go
// sketch
v.AppendAllSeq(slices.Values(names)) // no length, so append regrows ~11 times
```

The adapter improves on that — `v.AppendAll(containers.Slice[string](names))` is
1.71x faster with 6 allocations against 15. But a third form beats both, and it
needs no new type at all:

```go
// sketch
v.AppendMany(names...) // 1.124 µs, ONE allocation -- the raw append floor
```

| appending 1024 into an empty Vector | | allocs |
|---|---|---|
| `AppendMany(s...)` | **1.124 µs** | **1** |
| raw `append(es, s...)` | 1.169 µs | 1 |
| `AppendAll(Slice[T](s))` | 2.409 µs | 6 |
| `AppendAllSeq(slices.Values(s))` | 4.266 µs | 15 |

Variadic expansion reaches `append`'s own variadic form — one memmove with a
known length — so it is **2.1x the adapter**. And it is not the cost ADR `0015`
measured: that was `Append(e T)` against `Append(es ...T)` for *one* element,
paid per call, where a bulk method expands once per batch.

`Elems` still wins the other direction. Appending from another *container* is
2.388 µs and 5 allocations via `Elems`, against 4.518 µs and 13 if the caller
must materialise a slice for a variadic. The two are complementary and neither
subsumes the other.

**So this task argues for adding `Vector.AppendMany(es ...T)`, not for adding
`Slice`.** Recorded as a follow-up below and struck from this ADR's
justification.

### Task M — anything `Vector` exists for. **Not a win.**

```go
// sketch
func record(log containers.Slice[string], e string) {
	log = append(log, e) // lost, exactly as with []string
}
```

The adapter does not hide reallocation, does not make a callee's append visible,
and does not stop appends off a shared header aliasing. **Recorded so that
`Slice` is not later mistaken for a cheap `Vector`.**

## Findings

From `experiments/sliceadapter/`.

### A view over a plain slice needs no adapter

Like for like — every row taking an address, so the only variable is what is
underneath:

| | construct | `At` | `All` sum of 1024 |
|---|---|---|---|
| over a bare `*[]int` | **0.375 ns** | 1.10 ns | 1.150 µs |
| over a `*Slice[int]` | 0.370 ns | 1.05 ns | — |
| over a `*Vector[int]` | 0.375 ns | 0.98 ns | 1.144 µs |

Indistinguishable. (Decision 2 takes `[]T` rather than `*[]T`, which costs one
allocation — a separate axis, measured below. It applies equally to all three
rows and so does not change this comparison.)

**The adapter contributes nothing to the
one thing it was most wanted for**, and going without leaves the caller's field a
plain slice.

### Taking a slice by value costs one allocation, and there is no way around it

| construct a view | | allocs |
|---|---|---|
| from `*[]T` | 0.367 ns | 0 |
| from `[]T`, storing the header | **11.80 ns** | **1** |
| from `[]T`, storing its address | 11.68 ns | 1 |

A slice header is three words, so a view holding one is not pointer-shaped and
boxes with an allocation. Taking `[]T` and storing `&s` does not dodge it — the
address of a parameter escapes.

Decision 2 pays that allocation deliberately. The two forms view different
things, and the value form is the one that matches what a slice is.

### Evidence that bore on the adapter, and did not save it

**It would have cost nothing to use.** A defined slice type keeps bounds-check
elimination, through its accessor as well as through builtin syntax:

| indexed sum of 1024 | | vs raw |
|---|---|---|
| `raw[i]` on a `[]int` | 197.5 ns | — |
| `ad.At(i)` through the accessor | **196.8 ns** | **±0%** |
| `vec.At(i)` on the struct wrapper | 242.9 ns | +23% |

(The compiler's BCE report does not separate the two, because an inlined body
keeps its original source position and both accessors show a check. The timings
do, across three groups at ±3%.) Costing nothing is not a reason to exist.

**Its `Len` beat a bare iterator, and lost to a variadic.** `AppendAll(Slice[int](s))`
is 2.375 µs and 6 allocations against `AppendAllSeq(slices.Values(s))` at 4.070 µs
and 15 — but `AppendMany(s...)` is **1.124 µs and one allocation**, the raw
`append` floor. See Task L; this is the finding that removed the adapter's
headline justification.

**It could not serve the index/value case at all.** A type has one `All`, and one
yielding values satisfies `Elems[T]` rather than `Elems2[int, T]`:

```
Slice[string] does not implement Elems2[int, string] (wrong type for method All)
        have All() iter.Seq[string]
        want All() iter.Seq2[int, string]
```

## Decision

### 1. Add `ViewSlice` and `ViewSliceIdentity` over `[]T`. Add no new type.

```go
func ViewSlice[T, NT any](s []T, viewer CanViewSlice[T, NT]) IndexedView[NT]
func ViewSliceIdentity[T any](s []T) IndexedView[T]
```

That is the whole change. A `[]T` field gains the one thing it could not have —
being handed out read-only — and gains it without becoming a different type:

```go
type Config struct{ hosts []string } // unchanged

func (c *Config) Hosts() containers.IndexedView[string] {
	return containers.ViewSliceIdentity(c.hosts)
}
```

`append`, `range`, indexing and `encoding/json` all keep working on the field,
because it is still a slice.

`CanViewSlice[T, NT]` is a new requirement interface, identical in structure to
`CanViewVector` and separate from it for ADR `0012`'s stated reason: these are
named per *constructor*, so that failing to satisfy one names the operation you
cannot perform. `CanViewHashDict` and `CanViewMap` are already identical to each
other on the same grounds.

### 2. `[]T`, not `*[]T` — because a reallocated slice is a new slice

The constructor takes a slice, not the address of one. That costs one allocation
per view — 11.80 ns against 0.367 ns — and it is not an ergonomic concession. It
is the correct signature, and the reason is a semantic question worth writing
down.

**Is a reallocated slice logically the same slice, or a new one?**

It is a new one. Go's `append` returns a value the caller must rebind, and the
spec does not promise the backing array is reused, so a slice has value identity
rather than reference identity. This library has already committed to that
answer: ADR `0015` justifies `Vector` on the grounds that with one, "a
reallocation stops being observable" — which only needs saying because for a bare
slice it *is* observable. **If a reallocated slice were the same slice, `Vector`
would have nothing to fix.** Identity lives in a variable, or in a container that
owns the slice. Never in the slice value.

So the two candidate signatures view two different things:

- `ViewSlice(s *[]T)` views a **variable**, and follows every rebinding.
- `ViewSlice(s []T)` views a **slice value**, which cannot change.

Given that a slice is a value, the second is what "a view of a slice" means. The
behaviour follows and is correct rather than merely tolerable:

```
At(0)="MUTATED"   -- an element write is visible, because the view wraps that element
Len=1             -- an append is not, because it produced a value the view was never given
At(0)="original"  -- after the owner reallocates, the value the view holds is intact
```

That third line is not staleness. It is the view faithfully showing what it was
handed, and it matches what `views.go` already promises: a view is "not a
snapshot", it shows later writes to the elements it wraps, and it denies writes
*through the view*.

An earlier draft of this ADR took `*[]T` and called the value form "half-live,
and the half changes over time". That was measuring real behaviour against a
model in which slices have identity — the model ADR `0015` rejects.

### 3. One shared interface, renamed from `VectorView` to `IndexedView`

A slice's view adds nothing to a `Vector`'s, so they share one interface.
Declaring a second identical one would only mean a slice view could not be passed
where a vector view is wanted — ADR `0013` decision 1's reasoning applied
unchanged, as a `Map`'s view is a `DictView` rather than a `MapView`.

But the shared interface cannot keep the name `VectorView`, because after this
ADR one of its two producers is not a `Vector`. ADR `0015` decision 5 already
recorded naming it after an implementation as an inconsistency; a second producer
makes it an error.

```go
type IndexedView[NT any] interface {
	Elems[NT]
	At(int) NT
	AllIndexed() iter.Seq2[int, NT]
	sealedView()
}
```

**`IndexedView` names the specialisation, and `ListView` is the declared
destination.** A list is a bag of position/value pairs, and the position type
varies — an `int` for contiguous storage, an opaque cursor for a linked list.
The general contract is therefore two-parameter:

```go
// sketch -- not built here
type ListView[P, NT any] interface {
	Elems[NT]
	At(P) NT
	AllPositions() iter.Seq2[P, NT]
	sealedView()
}

type IndexedView[NT any] = ListView[int, NT] // generic alias, Go 1.24+
```

Prototyped and compiling: one interface covers both a slice view at `P = int` and
a linked-list view at `P = LinkedListCursor[V]`.

**It is deliberately not built now, and `ListView` is deliberately not taken
now.** ADR `0010` named itself `LinkedList` precisely so `List` and `MutableList`
stayed free, on the grounds that spending the bare concept word on an
implementation "is what forced ADR `0008`'s whole vocabulary change, when `Map`
blocked `MutableMap`". Claiming `ListView` for an interface that only covers
integer positions would be that mistake one level down.

The migration costs nothing when the general contract does land:
`IndexedView[NT]` becomes `= ListView[int, NT]`, and `experiments/views`
verified that a generic alias and its long form are interchangeable in both
directions. No caller breaks.

## Open questions

None. The naming question this ADR opened is settled by decision 3.

## Rejected alternatives

### `Slice[T] []T` as an adapter — what this ADR set out to propose

ADR `0015` recorded it as a follow-up and ADR `0008` had pre-authorised the name.
It would have been a defined slice type with value receivers, `Len`, `At`, `Set`,
`All`, `AllIndexed` and `Clone`, and deliberately no `Append` — a value receiver
cannot grow a slice, and a pointer receiver would forfeit the value semantics
that make an adapter an adapter.

Every reason for it fell over:

- **To give a slice a view.** The view needs no adapter. Measured like for like,
  a view over a bare slice is identical to one over the adapter on construction,
  indexing and iteration, and it leaves the caller's field a plain slice instead
  of requiring a type change.
- **To let a `[]T` satisfy `Elems[T]` for bulk operations.** Task L: a variadic
  bulk form is 2.1x faster than the adapter for that, and needs no type.
- **To round-trip through `encoding/json`.** Task K: a plain slice already does,
  and decision 1 keeps the field a plain slice.
- **To supply index/value pairs to a dict-shaped constructor.** It cannot. A type
  has one `All`, and one yielding values satisfies `Elems[T]` rather than
  `Elems2[int, T]`:

  ```
  Slice[string] does not implement Elems2[int, string] (wrong type for method All)
          have All() iter.Seq[string]
          want All() iter.Seq2[int, string]
  ```

  Serving that case would need a *second* adapter whose `All` yields pairs, which
  would then not satisfy `Elems[T]`.

What is left is a type that costs nothing to use, fixes nothing, and does nothing
a plain slice plus one constructor does not. It would also have added a second
sequence type whose difference from `Vector` is invisible in the name, and a
fourth exception to ADR `0008`'s naming scheme.

**The analysis is kept because the wall in the `Map` analogy is worth having
written down**: `Map` works as an adapter because a map is a reference type, and
nothing in the slice world can copy that.

### Give the adapter a pointer receiver so it can grow

`func (s *Slice[T]) Append(e T)` works. Rejected with the adapter, and
independently: a `*Slice[T]` cannot be produced by a free conversion from a `[]T`
value, cannot be passed where a `[]T` is expected, and reintroduces the
mixed-receiver problem ADR `0002` decision 2 rejected. A type that needs a
pointer is `Vector`, which already exists.

### Take the address of the slice in the view

`ViewSlice(s *[]T)` is free — 0.367 ns and no allocation against 11.80 ns and
one — and gives a view that follows the caller's variable through every append
and rebinding.

Rejected on decision 2's grounds: it views a *variable*, not a slice, and a slice
is what the caller asked to have viewed. It also costs `&` at every call site and
requires an addressable slice, so it cannot be applied to a function's return
value.

If a view of a variable is ever wanted, it is a distinct concept and deserves a
distinct name rather than the default spelling — and `Vector` already covers most
of what it would be for, at no allocation and with real identity behind it.

### Do nothing

Rejected on Task J alone. A slice field cannot be handed out read-only at any
price short of copying — O(n) per call and O(n²) in a caller's loop, per
`experiments/copycost` — and migrating it to `Vector` changes how every other
line touching it reads.

## Consequences

- **The library gains a view without gaining a container.** This is the first
  time; every previous view came with one. ADR `0011`'s rule is that every
  container has a view, not that every view has a container, so nothing needs
  restating — but `CLAUDE.md` describes views as belonging to containers and will
  need a word.
- **`VectorView` is renamed to `IndexedView`**, which touches shipped code:
  `views.go`, `views_test.go`, `callsites_test.go`, `CLAUDE.md`, and ADR `0015`
  decision 5, which named it. The rename lands with this ADR rather than before
  it, because `VectorView` is not yet wrong — it becomes wrong the moment
  `ViewSlice` exists.
- **A slice field can now be exposed read-only without changing its type**, which
  is a cheaper migration than any other container in this library offers — there
  is nothing to migrate.
- **A slice view costs one allocation**, where every other view in the library
  costs none. That is the price of viewing a value rather than a container, and
  it is unavoidable: a slice header is three words, so it cannot be
  pointer-shaped.
- **A slice view shows element writes but not appends.** Both are correct — it
  wraps the elements it was given and was never given the appended one — but it
  is the first view in the library where the distinction is visible, because it
  is the first view of something without identity. The doc comment must say so
  plainly.
- **`ViewSliceIdentity` works on any slice expression**, including a function's
  return value, since it takes no address.
- **No `Slice[T]`, so ADR `0008`'s pre-authorised exception goes unused**, and the
  naming scheme keeps two exceptions rather than gaining a third.

## Follow-ups

### Add `Vector.AppendMany(es ...T)`

Task L's real conclusion, and it is independent of whether this ADR is accepted.
Appending a plain slice is 2.1x faster and five allocations cheaper through a
variadic bulk method than through any other form available today, and it reaches
the raw `append` floor. ADR `0015`'s measurement does not argue against it: that
was the per-call cost of a variadic *single* append, where a bulk method expands
once per batch.

It belongs in the mutator-consistency work rather than here, because the same
question applies to `SortedSet.AddAll` and every other `*All` in the package.

### The iteration contracts want their own ADR

Several findings across ADRs `0013`, `0015` and this one point at the same place,
and they are collected here so the ADR that takes them on does not have to
rediscover them.

**A type has one `All`, so `Elems[T]` and `Elems2[K, V]` are mutually
exclusive.** This is the root of most of the rest:

- `Vector` satisfies `Elems[T]`, so it cannot also satisfy `Elems2[int, T]`, and
  `AllIndexed` exists under a second name only because `All` is taken.
- A slice adapter could not have served the index/value case for the same reason
  — verified as a compile error in `experiments/sliceadapter`.
- `LinkedList` (ADR `0010`) spends its `All` on cursor/value pairs, so it
  satisfies `Elems2` and not `Elems`. **The two sequence containers disagree about
  what `All` means**, which is what stops `ListView[P, NT]` being built today:
  whichever way it goes, one container ends up with a view whose `All` does not
  mirror its own.

**ADR `0006` records the cause**: "Go has no higher-kinded types: `iter.Seq` and
`iter.Seq2` cannot be unified."

Two cost findings belong with it:

- **Iterating a view allocates three times per call** (ADR `0013` follow-up),
  because `All` returns a closure through a dynamic call. `Get` is unaffected.
- **`iter.Seq` costs ~6x raw ranging over a slice** (`experiments/vectorcost`),
  and ~5.8x of that is the stdlib's own price — `slices.Values` pays it too. For
  `LinkedList` the same abstraction *hides* a 3.9x pointer-chasing penalty
  entirely (`experiments/linkedlist`).

And one soundness finding:

- **A boundary declared `Elems2[K, V]` seals nothing**, since containers satisfy
  it too (ADR `0013` decision 4). That is deliberate, but it is the one hole left
  in the view seal.
