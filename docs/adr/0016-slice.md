# 16. A view over a plain slice, and no `Slice` type

- **Status:** Proposed. Note that this ADR changed subject while being written:
  it set out to propose `Slice[T] []T` and the measurements turned it into a
  proposal to add a view and no type. The rejected adapter is kept in full below.
- **Date:** 2026-09-08
- **Evidence:** `experiments/sliceadapter/` (`RESULTS.md`), with context from
  `experiments/vectorcost/` and `sizedcollect/`.
- **Relates to:** ADR `0015` (`Vector`, whose follow-up proposed this, and whose
  non-variadic `Append` creates the gap this fills), `0009` (`Map`, the model),
  `0007` (the value/pointer asymmetry an adapter accepts), `0006` (sized
  construction, which is the measured win), `0002` (the shape rules an adapter
  deliberately does not follow), `0008` (naming — `Slice` is the exception this
  ADR was pre-authorised to take).

## Context

ADR `0009` makes `Map[K, V]` a defined `map[K]V` and calls it an **adapter**, not
a default. `CLAUDE.md` records when to reach for one: a free conversion from an
existing builtin, builtin syntax, passing the result where the builtin is
expected, and working `encoding/json`.

ADR `0015` recorded the parallel as a follow-up: `Slice` would be to `Vector`
what `Map` is to `HashDict`. ADR `0008` had already pre-authorised the name,
saying a future `Slice[T] []T` would take the same exception `Map` does.

The parallel holds in outline and breaks in one specific place, which shapes
everything below.

## The wall in the analogy

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
length.** That is the whole shape of what follows.

It also fixes nothing:

```
appends off a shared adapter header still alias=true
```

Naming a slice type changes no slice semantics. Every hazard ADR `0015` records
survives. An adapter is an ergonomic convenience, exactly as `0009` says of
`Map`; `Vector` remains the answer where the hazards matter.

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

func (c *Config) Hosts() containers.VectorView[string] {
	return containers.ViewSliceIdentity(&c.hosts)
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

### It costs nothing to use

| indexed sum of 1024 | | vs raw |
|---|---|---|
| `raw[i]` on a `[]int` | 197.5 ns | — |
| `ad[i]` on a `Slice[int]` | 202.5 ns | +2.5% |
| `ad.At(i)` through the accessor | **196.8 ns** | **±0%** |
| `vec.At(i)` on the struct wrapper | 242.9 ns | +23% |

**A defined slice type keeps bounds-check elimination**, through its accessor as
well as through builtin syntax, where `Vector` loses it. The accessor inlines to
an index into the slice the loop bound came from, so the check stays provable.

(The compiler's own BCE report does not separate the two, because an inlined body
keeps its original source position and both accessors show a check. The timings
do, across three groups at ±3%.)

### The sizing win is real, and largest where construction happens

| append 1024 into an **empty** Vector | | allocs |
|---|---|---|
| `AppendAll(Slice[int](s))` | **2.375 µs** | **6** |
| `AppendAllSeq(slices.Values(s))` | 4.070 µs | 15 |

| append 1024 onto one already holding 1024 | | allocs |
|---|---|---|
| `AppendAll(Slice[int](s))` | 4.451 µs | 7 |
| `AppendAllSeq(slices.Values(s))` | 5.250 µs | 6 |

1.71x empty, 1.18x not — and one allocation *more* in the second case, because a
populated vector already has capacity to absorb most of the growth. The headline
number is the empty case, which is the one `Collect`-style construction hits.

### A view over it must hold a pointer

| construct a sequence view | | allocs |
|---|---|---|
| over a `*Vector` | 0.371 ns | 0 |
| over a `Slice` **by value** | **12.25 ns** | **1** |
| over a `*Slice` | 0.377 ns | 0 |

A slice header is three words, so holding one by value is not pointer-shaped and
boxes with an allocation. `Map` has no such problem: its header is one word, so
`ViewMapIdentity` takes a `Map` by value and allocates nothing.

## Decision

### 1. Add `ViewSlice` and `ViewSliceIdentity` over `*[]T`. Add no new type.

```go
func ViewSlice[T, NT any](s *[]T, viewer CanViewVector[T, NT]) VectorView[NT]
func ViewSliceIdentity[T any](s *[]T) VectorView[T]
```

That is the whole change. A `[]T` field gains the one thing it could not have —
being handed out read-only — and gains it without becoming a different type:

```go
type Config struct{ hosts []string } // unchanged

func (c *Config) Hosts() containers.VectorView[string] {
	return containers.ViewSliceIdentity(&c.hosts)
}
```

`append`, `range`, indexing and `encoding/json` all keep working on the field,
because it is still a slice.

### 2. `*[]T`, not `[]T`

A slice header is three words, so a view holding one by value is not
pointer-shaped and costs an allocation on every construction; holding the
address costs none. Measured at 12.25 ns and one allocation against 0.375 ns and
zero.

The address is also the right semantics rather than merely the cheap one: the
view tracks the *variable*, so the owner's appends — including reallocating
ones — are visible through it. Holding a copy of the header would show a
snapshot that silently goes stale. This matches `ViewVectorIdentity`, which holds
`*Vector`.

The cost is that `ViewSliceIdentity` needs an addressable slice, so it cannot be
applied directly to a function's return value. One local variable fixes it, and
the same is true of `Vector`.

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

- **To give a slice a view.** The view needs no adapter. Measured over a bare
  `*[]T` it is identical on construction, indexing and iteration, all
  allocation-free, and it leaves the caller's field a plain slice instead of
  requiring a type change.
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

### Take the slice by value in the view

`ViewSliceIdentity(s []T)` reads better than `ViewSliceIdentity(&s)` and works on
a function's return value directly. Rejected on two grounds: it costs 12.25 ns
and an allocation per construction against 0.375 ns and none, and it captures a
*snapshot* of the header, so the view silently goes stale the moment the owner
appends.

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
- **The view tracks the variable, not a snapshot.** `hosts = append(hosts, x)` is
  visible through a view taken beforehand, including across reallocations. That
  is the correct behaviour for a field accessor and a surprise for anyone
  expecting a copy; it is the same rule every other view in the library follows.
- **`ViewSliceIdentity` needs an addressable slice**, so it cannot be applied to
  a function's return value without a local. `Vector` has the same constraint.
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
