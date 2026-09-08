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

### Task J — feeding a plain `[]T` to a sized bulk operation

ADR `0015` made `Vector.Append` non-variadic, so bulk-appending a slice goes
through either a length-carrying `Elems` or a bare iterator with none. Today only
the second is available:

```go
// sketch
v.AppendAllSeq(slices.Values(names)) // no length, so append regrows ~11 times
```

Proposed:

```go
// sketch
v.AppendAll(containers.Slice[string](names)) // Len is available; Grow allocates once
```

**Not shorter — 1.71x faster and 6 allocations against 15.** This is a
performance win rather than an ergonomic one, and the call-site convention asks
about shorter or safer. Recorded as the honest position: this task justifies the
type on ADR `0006`'s grounds, which this library already treats as sufficient,
and not on the convention's usual two.

The same gap exists at `SortedSet.AddAll`, `CollectSortedSet`, `CollectVector`
and `CollectSortedDict` — every operation taking `Elems`. A plain slice cannot
supply one today.

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

### Task L — anything `Vector` exists for. **Not a win.**

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

### 2. It exists to let a plain `[]T` satisfy `Elems[T]`

That is the justification, and it should be the first line of its doc comment. A
free conversion turns any slice into something the sized constructors and bulk
mutators can take with a length in hand.

### 3. It is an adapter, not a default — `Vector` is the default

Reach for `Slice` for a free conversion from `[]T`, for builtin syntax, to pass
where a `[]T` is expected, or for working `encoding/json`. Everything else uses
`Vector`, which is the only one of the two that hides reallocation.

This is the same division `0009` draws between `Map` and `HashDict`, and it
should be stated the same way.

### 4. Its view takes a pointer

```go
func ViewSlice[T, NT any](s *Slice[T], viewer CanViewVector[T, NT]) VectorView[NT]
func ViewSliceIdentity[T any](s *Slice[T]) VectorView[T]
```

`*Slice[T]`, not `Slice[T]`, on the measurement above: by value it costs an
allocation on every construction. This is an asymmetry with `ViewMapIdentity`,
which takes its `Map` by value, and it is forced by the width of a slice header.

## Open questions

### A. Whether `Slice` should have a view at all

ADR `0011`'s rule is that every container has one, and `Map` — also an adapter —
has `ViewMap`. But decision 4's pointer requirement means `ViewSliceIdentity(&s)`
cannot be applied to a function's return value, which is exactly where a caller
holding a `[]T` would want it. The alternative is accepting the allocation and
taking the value.

### B. Whether `CollectSlice` belongs

Every other container has `Collect<Container>`. For a slice, `slices.Collect`
already exists and returns `[]T`, which converts for free. A `CollectSlice`
would be a rename of a stdlib function.

### C. Whether this makes `Vector.AppendAll` the odd one out

If a plain slice can now supply `Elems`, the argument for `Vector.Append` being
non-variadic gets stronger — `AppendAll(Slice[T](s))` covers the bulk case
cleanly. Worth re-reading `0015`'s follow-up on mutator consistency with this in
hand.

## Rejected alternatives

### Give `Slice` a pointer receiver so it can grow

`func (s *Slice[T]) Append(e T)` works. Rejected because it forfeits everything
the type is for: a `*Slice[T]` cannot be produced by a free conversion from a
`[]T` value, cannot be passed where a `[]T` is expected, and reintroduces the
mixed-receiver problem ADR `0002` decision 2 rejected. A type that needs a
pointer is `Vector`, which already exists.

### Do not build it; tell callers to use `slices.Values`

Costs nothing. Rejected on the measurement: it forgoes 1.71x and nine
allocations on every `Collect`-shaped construction from a slice, and leaves the
library with no sequence type that round-trips through `encoding/json`.

### Make `Slice` the default and drop `Vector`

Rejected outright: an adapter fixes none of the hazards ADR `0015` was accepted
to fix, and `Vector`'s justification — the accessor problem — is untouched by
anything here.

## Consequences

- **Two sequence types, and the difference is not obvious from the names.**
  `Vector` hides reallocation; `Slice` does not. This needs saying wherever
  either is documented, as `0009` does for `Map` and `HashDict`.
- **`Slice` is the only sequence in the library that round-trips through
  `encoding/json`**, which makes the missing serialization ADR more visible, not
  less.
- **Its view is spelled differently from `Map`'s**, taking a pointer where
  `ViewMapIdentity` takes a value. A reader will notice; the doc comment should
  say why.
- **`Slice[T]` has no `Append` while `Vector` does**, which will read as an
  omission until the reason is given. It is the same asymmetry `Map` has with
  nothing, and stems from a language property rather than a choice.
