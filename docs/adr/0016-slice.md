# 16. Slice[T]: a defined slice type, as an adapter

- **Status:** Proposed.
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
// sketch
type Config struct{ hosts containers.Slice[string] } // still a []string underneath

func (c *Config) Hosts() containers.VectorView[string] {
	return containers.ViewSliceIdentity(&c.hosts)
}
```

**Zero copy, O(1), and the caller cannot write.** The field is still a slice —
builtin syntax, `append`, `range`, `encoding/json` all keep working — and it
gained the ability to be handed out read-only, which a `[]T` cannot do at any
price short of copying.

This is ADR `0001`'s accessor problem again, and it is the justification Task L
was supposed to provide and does not. It is a *safety* win, which is what the call-site
convention actually asks for.

### Task K — a sequence field that round-trips through JSON

```go
// sketch -- today
type Config struct {
	Hosts containers.Vector[string] // marshals as {} -- contents silently lost
}

type Config struct {
	Hosts []string // round-trips, but has no library vocabulary at all
}
```

Proposed:

```go
// sketch
type Config struct {
	Hosts containers.Slice[string] // ["a","b"], and has Len, At, All, AllIndexed
}
```

A narrow win, and the only one available: no struct-shaped container in this
library can round-trip, and that is not fixable without the serialization ADR
`0002` has been owing since the beginning.

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

### 1. Build `Slice[T any] []T`, with value receivers and no growth

```go
type Slice[T any] []T

func (s Slice[T]) Len() int
func (s Slice[T]) At(i int) T
func (s Slice[T]) Set(i int, e T)
func (s Slice[T]) All() iter.Seq[T]
func (s Slice[T]) AllIndexed() iter.Seq2[int, T]
func (s Slice[T]) Clone() Slice[T]
```

**There is deliberately no `Append`.** A value receiver cannot grow a slice, and
a pointer receiver would forfeit the value semantics the type exists for. Callers
append with the builtin — `s = append(s, e)` works on a `Slice[T]` unchanged —
or reach for `Vector` when they want that hidden.

`Set` is kept even though it permits mutation through a read-shaped type, because
`s[i] = e` works on a defined slice type regardless. Denying the method would
buy nothing and cost consistency with `Vector`.

### 2. It exists so a slice can be handed out read-only, and can satisfy `Elems[T]`

The first is the justification and belongs in the first line of its doc comment:
a `[]T` field converts for free into something that has a view, which is the one
thing a slice cannot otherwise do without copying.

Satisfying `Elems[T]` is a secondary convenience. It is **not** the reason —
Task L shows a variadic bulk method beats the adapter 2.1x for that job — but it
is still the right way to feed a slice to an operation that has no variadic
form.

### 3. It is an adapter, not a default — `Vector` is the default

Reach for `Slice` for a free conversion from `[]T`, for builtin syntax, to pass
where a `[]T` is expected, or for working `encoding/json`. Everything else uses
`Vector`, which is the only one of the two that hides reallocation.

This is the same division `0009` draws between `Map` and `HashDict`, and it
should be stated the same way.

### 4. It has a view, sharing the sequence view interface, and that view takes a pointer

Every container has one (ADR `0011`), adapters included — `Map` has `ViewMap`.
The need is not theoretical here: handing out read-only access to a slice is
Task J, this ADR's primary justification.

```go
func ViewSlice[T, NT any](s *Slice[T], viewer CanViewVector[T, NT]) VectorView[NT]
func ViewSliceIdentity[T any](s *Slice[T]) VectorView[T]
```

**It shares the interface rather than declaring a `SliceView` of its own.** That
follows ADR `0013` decision 1 exactly: a `Map`'s view is a `DictView`, not a
`MapView`, because it adds nothing to the base. A `Slice`'s view adds nothing to
a `Vector`'s, so a second identical interface would only mean a slice view could
not be passed where a vector view is wanted.

**`*Slice[T]`, not `Slice[T]`.** Measured: by value a view costs 12.25 ns and an
allocation, because a slice header is three words and so not pointer-shaped; by
pointer it is 0.377 ns and none. `Map` avoids this only because a map header is
one word. The cost is that `ViewSliceIdentity` needs an addressable `Slice`, so
it cannot be applied directly to a function's return value.

### 5. No `CollectSlice`

`slices.Collect` already returns a `[]T`, which converts to a `Slice[T]` for
free. A `CollectSlice` would be a rename of a stdlib function, which is not what
the `Collect<Container>` family is for.

## Open questions

### A. What the sequence view interface should be called

Decision 4 shares one interface between `Vector` and `Slice`, which is right. But
it is currently called `VectorView`, and naming it after one of its two
implementations was already flagged in ADR `0015` decision 5 as an inconsistency
— the set and dict views are named for the *concept*. A second implementation
makes it actively misleading.

`SeqView` or `ListView` would be concept-shaped and correct for both. Renaming
touches shipped code, so it is left to the naming review ADR `0015` opened rather
than taken here. **If that review is not happening soon, this is the argument for
doing it now**, since every new caller of `VectorView` makes the rename bigger.

### B. Whether `Vector.AppendAll` is the odd one out

Left open deliberately, pending the bulk-mutator consistency work ADR `0015`
recorded. The finding in Task L is an input to it: a variadic bulk form and an
`Elems` form are complementary, each optimal for its own source, so the answer is
probably "offer both everywhere" rather than picking one.

## Rejected alternatives

### Give `Slice` a pointer receiver so it can grow

`func (s *Slice[T]) Append(e T)` works. Rejected because it forfeits everything
the type is for: a `*Slice[T]` cannot be produced by a free conversion from a
`[]T` value, cannot be passed where a `[]T` is expected, and reintroduces the
mixed-receiver problem ADR `0002` decision 2 rejected. A type that needs a
pointer is `Vector`, which already exists.

### Do not build it

Costs nothing, and Task L no longer argues against it — a variadic bulk method
covers that case better than the adapter does, with no new type.

Rejected on Task J alone. Without `Slice`, a `[]T` field cannot be handed out
read-only at any price short of copying it, and migrating the field to `Vector`
changes how every other line touching it reads — `append`, `range`, indexing and
`encoding/json` all stop working on it. `Slice` is the only way to keep a slice a
slice and still give it a view.

The JSON round-trip in Task K is a second, narrower reason, and would not have
been sufficient alone.

### Make `Slice` the default and drop `Vector`

Rejected outright: an adapter fixes none of the hazards ADR `0015` was accepted
to fix, and `Vector`'s justification — the accessor problem — is untouched by
anything here.

## Consequences

- **Two sequence types, and the difference is not obvious from the names.**
  `Vector` hides reallocation; `Slice` does not. `Slice` exists so an existing
  slice can be given a view without ceasing to be a slice. This needs saying
  wherever either is documented, as `0009` does for `Map` and `HashDict`.
- **The adapter is not the fast path for bulk appends**, despite carrying `Len`.
  A variadic form beats it 2.1x when the source is already a slice. Reaching for
  `Slice[T](s)` to feed a bulk operation is a mistake the doc comment should name
  outright.
- **`Slice` is the only sequence in the library that round-trips through
  `encoding/json`**, which makes the missing serialization ADR more visible, not
  less.
- **Its view is spelled differently from `Map`'s**, taking a pointer where
  `ViewMapIdentity` takes a value. A reader will notice; the doc comment should
  say why.
- **`Slice[T]` has no `Append` while `Vector` does**, which will read as an
  omission until the reason is given. It is the same asymmetry `Map` has with
  nothing, and stems from a language property rather than a choice.

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
