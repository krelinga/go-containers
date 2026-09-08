# 15. Vector[T]: a container that owns its slice

- **Status:** Accepted. Not yet implemented, so this constrains the change
  rather than describing it.
- **Date:** 2026-09-08
- **Evidence:** `experiments/vectorcost/` (`RESULTS.md`), plus the call-site
  sketches below. Context from `experiments/copycost/` (what an accessor that
  copies costs) and `linkedlist/` (what iterator abstraction costs).
- **Relates to:** ADR `0002` (the container shape this adopts wholesale), `0001`
  (views versus defensive copies — the strongest argument here), `0011`/`0013`
  (views, which a slice cannot have and a Vector can), `0004` (variadic bulk
  mutators, which this ADR proposes diverging from), `0008` (naming, which does
  not obviously accommodate this type), `0010` (`LinkedList`, the other sequence).

## Context

A slice header is a **value**. Length and backing pointer are copied on every
assignment and every call, which produces three distinct hazards, all measured
in `experiments/vectorcost`:

```
slice: two appends off one header -> x[1]=2 y[1]=2  aliased=true
slice: callee appended, caller sees Len=0  (the append is lost)
```

Appends off a shared header alias when capacity allows; a callee's append is
invisible unless it returns the slice and the caller reassigns; and holders that
shared a backing array before a reallocation stop sharing after it. This is
frustration number two of the three this repository was started over.

A `Vector[T]` owning its slice behind a pointer removes all three at once: every
holder reaches the same header, so **a reallocation stops being observable.**

There is a second motivation, and on the evidence it is the stronger one. ADR
`0001` established that an accessor returning an internal collection must either
copy — ~10 ns floor, growing per element, and **O(n²)** when called in a
caller's loop, 36 800x at n = 16 384 — or hand out a mutable interior. Every
container in this library has escaped that through a view. **A `[]T` field
cannot have a view.** A `Vector[T]` field can.

## Call sites

Phase 1 sketches per the call-site convention: the same task written against the
stdlib and against the API being proposed. These do not compile.

### Task A — a type that owns a growing sequence and exposes it

Today, and the choice is between two bad options:

```go
// sketch
type AuditLog struct{ entries []Entry }

func (l *AuditLog) Record(e Entry) { l.entries = append(l.entries, e) }

// Either copy on every call -- O(n) each time, O(n²) in a caller's loop...
func (l *AuditLog) Entries() []Entry { return slices.Clone(l.entries) }

// ...or hand out the interior, and the caller can write l.Entries()[0] = x
// straight into the log.
func (l *AuditLog) Entries() []Entry { return l.entries }
```

Proposed:

```go
// sketch
type AuditLog struct{ entries containers.Vector[Entry] }

func (l *AuditLog) Record(e Entry) { l.entries.Append(e) }

func (l *AuditLog) Entries() containers.VectorView[Entry] {
	return containers.ViewVectorIdentity(&l.entries)
}
```

Not shorter. But O(1) instead of O(n), no allocation, and the caller cannot
write through it. **This is the win, and it is the same win ADR `0001` bought
every other container.** No amount of `slices` helps here, because the problem is
not expressiveness.

### Task B — accumulating across a function boundary

```go
// sketch -- today, two spellings, both with a footgun
func addDefaults(out []string) []string { return append(out, "a", "b") }
out = addDefaults(out) // forgetting the reassignment silently does nothing

func addDefaults(out *[]string) { *out = append(*out, "a", "b") } // *[]T
```

Proposed:

```go
// sketch
func addDefaults(out *containers.Vector[string]) { out.Append("a", "b") }
addDefaults(&out) // nothing to forget
```

A mild win: it removes a silent-failure mode and the `*[]T` double indirection.
Not on its own enough to justify a container.

### Task C — building a slice locally. **Not a win.**

```go
// sketch
func evens(in []int) []int {
	out := make([]int, 0, len(in))
	for _, x := range in {
		if x%2 == 0 {
			out = append(out, x)
		}
	}
	return out
}
```

A `Vector` version is the same length, introduces a type the caller must convert
back to use with `slices.*`, and the aliasing hazards never arise because the
slice never leaves. **Recorded so that local slice building is not later cited as
motivation.**

### Task D — reading a sequence you already hold. **Not a win.**

```go
// sketch
func total(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}
```

Through a Vector this costs 24% via indexed access or **6x** via `All()` (see
findings). A slice parameter you only read is already correct, already fast, and
already idiomatic. **A Vector is not a replacement for `[]T`.**

## Findings

From `experiments/vectorcost/`. Sums are over 1024 elements.

### Single reads are free; indexed loops are not

| | |
|---|---|
| `s[i]` | 0.374 ns |
| `v.At(i)` | 0.347 ns |
| indexed loop over `s` | 196.7 ns |
| indexed loop over `v` | **244.8 ns (+24%)** |

The forwarding method inlines, so one read is free. What is lost is
**bounds-check elimination** across repeated indexing. The compiler confirms it
directly, under `-gcflags='-d=ssa/check_bce/debug=1'`: the indexed slice loop
reports nothing, while the check inside the accessor survives.

```
./shapes.go:22:50: Found IsInBounds     // func (v *Vector[T]) At(i int) T
```

`i < len(s)` proves the index safe; `i < v.Len()` indexing `v.es` does not.
**This is the one overhead a sequence wrapper carries that a map wrapper does
not** — a map has no bounds check to lose.

### The iterator costs 6x, and the container is 3% of it

| sum of 1024 | | vs raw range |
|---|---|---|
| `for _, x := range s` | 200.9 ns | — |
| `for x := range slices.Values(s)` | 1.156 µs | **5.8x** |
| `for x := range v.All()` | 1.194 µs | 5.9x |

The expensive thing is `iter.Seq`, and it is the stdlib's price: `slices.Values`
already pays 5.8x with no container involved. `Vector.All` adds 3.3%. The library
committed to `iter.Seq` as its interchange in ADR `0006` and `0008`, so **a
Vector inherits this cost rather than creating it** — but the proportion is much
worse for a sequence than a map, because raw slice iteration is so cheap.

### The variadic signature costs more than the container

| append one element, presized | | |
|---|---|---|
| `s = append(s, 1)` | 0.5715 ns | — |
| `v.AppendOne(1)` | 0.6570 ns | +15% |
| `v.Append(1)` with `...T` | 0.9279 ns | **+62%** |

The container is 15%; `...T` is another 41 points. ADR `0002` made mutators
variadic, and on a set that is invisible — `Add` is a ~5 ns map insert, so the
overhead is noise. Append is 0.57 ns, so **the same convention lands very
differently here.**

## Decision

### 1. Build `Vector[T any]`, in ADR `0002`'s shape

`noCopy` declared first, uniform pointer receivers, usable zero value, no
nil-receiver or nil-argument special cases, and the eager-dereference rule
applied to every method — including `All`, which reads `v.es` into a local before
returning its closure.

`T` is unconstrained: a sequence needs neither `comparable` nor `cmp.Ordered`.

### 2. Justified by the accessor problem, not by convenience

Task A is the case that earns it. Tasks C and D are explicitly **not** wins and
are recorded as such. A Vector is not a general replacement for `[]T`, and the
documentation should say so where a reader will meet it.

### 3. `Append` is not variadic; bulk insertion follows ADR `0004`

```go
func (v *Vector[T]) Append(e T)
func (v *Vector[T]) AppendAll(src Elems[T])
func (v *Vector[T]) AppendAllSeq(seq iter.Seq[T])
```

This diverges from `HashSet.Add(...T)` deliberately, on the measurement above:
41% of an append is a real cost where the same overhead on a map insert is noise.
`AppendAll` taking `Elems[T]` preserves ADR `0006`'s sizing win, since `Len` is
available as a capacity hint.

### 4. Surface

`Len`, `At(i) T`, `Set(i int, e T)`, `Append`, `AppendAll`, `AppendAllSeq`,
`All() iter.Seq[T]`, `AllIndexed() iter.Seq2[int, T]`, `Clone`.

`At` and `Set` panic out of range, exactly as `s[i]` does. `AllIndexed` cannot be
called `All`, because `Elems[T]` already claims that name for the value-only
sequence and a type cannot have both.

### 5. It satisfies `Elems[T]`, and gets a view

Per ADR `0011`, a container is not finished without a view; per `0013`, that view
is a sealed interface. A sequence fits none of the existing four, so it needs a fifth, named
`VectorView`:

```go
type VectorView[NT any] interface {
	Elems[NT]
	At(int) NT
	AllIndexed() iter.Seq2[int, NT]
	sealedView()
}
```

Note this names the implementation where `SetView` and `DictView` name the
concept — `Vector` is the only sequence, so there is no concept/implementation
split to express yet. It is one of the inconsistencies the naming review below
should look at.

### 6. The name is `Vector`, as a deliberate exception to ADR `0008`

`0008` names implementations `<Ordering><Concept>` and reserves bare concept
names for contracts. A sequence has no ordering axis — it is insertion-ordered by
definition — so there is no ordering to put in front, and `Vector` lands in an
implementation slot with a contract-shaped name.

Taken anyway, because every alternative is worse: `ArrayList` and `SliceList` add
a word that carries no information, `List` collides with `LinkedList`'s concept,
and `Slice` is wanted for the adapter that will likely follow. `Vector` has the
C++ and Rust precedent, and `Map` is already an exception to `0008` on the same
kind of grounds — it keeps the builtin's name because renaming it would cost more
than the inconsistency does.

This is the second exception to a scheme with five entries. That is a signal, and
it is recorded as a follow-up rather than pretended away.

### 7. `At` panics; there is no comma-ok sibling

`At(i)` panics out of range, like `s[i]`, and there is no `Get(i) (T, bool)`.
Matching the builtin is the point of an index accessor, and a caller who needs to
test has `Len()`.

Recorded as revisitable at low cost: adding `Get` later is purely additive and
breaks nothing, so deciding "no" now forecloses nothing.

## Rejected alternatives

### Keep using `[]T` and document the hazards

Costs nothing and stays idiomatic. Rejected because it leaves Task A unsolved,
and Task A is the one this library exists for: an accessor over a slice field
must copy or leak, and copying is O(n²) in a caller's loop. No documentation
fixes that.

### Return `[]T` from a `Vector` accessor rather than a view

Simpler, and free. Rejected because it reintroduces exactly the hazard the
container removes — the returned header is a value, so the caller can append off
it, and can write through it into the container. It would make `Vector` a
wrapper that buys nothing.

### Make `At` return a pointer, so `Set` is unnecessary

`func (v *Vector[T]) At(i int) *T` allows in-place mutation without a `Set`.
Rejected: it hands out an interior pointer that survives a reallocation, at which
point it silently points into the old backing array. The exact class of bug this
container exists to remove.

## Consequences

- **A Vector is not a `[]T` replacement.** Two of the four call-site tasks are
  explicitly not wins, and reading a sequence you already hold is 24% to 6x
  worse. The type earns its place at API boundaries, not in local code.
- **Bounds-check elimination is lost across indexed loops**, which no other
  container in this library pays. Callers who need the last 24% should iterate,
  not index — which is also what the contracts prefer.
- **`Append` diverges from `Add`.** The package will have a variadic mutator on
  sets and a single-element one on sequences, justified by measurement rather
  than by symmetry. This will look inconsistent to a reader who has not read the
  numbers, so it needs a comment where it is defined.
- **A fifth view interface**, and a fifth pair of view constructors, per ADR
  `0013`'s rule that every container has one.
- **Serialization remains unsettled and gets worse.** A slice is the one Go type
  a reader most expects to round-trip through `encoding/json`, and `Vector` will
  marshal as `{}` like every other container here. The library-wide ADR that
  `0002` called for is now more overdue, not less.

## Follow-ups

### Make bulk mutators consistent across the map-backed containers

Proposing `Append`/`AppendAll`/`AppendAllSeq` exposed that the existing
containers already disagree with each other:

| container | single | from `Elems` | from `iter.Seq` | variadic |
|---|---|---|---|---|
| `HashSet` | — | **missing** | **missing** | `Add(...T)`, `Remove(...T)` |
| `SortedSet` | — | `AddAll` | `AddAllSeq` | `Add(...T)`, `Remove(...T)` |
| `HashDict` | `Set(K, V)` | `SetAll` | `SetAllSeq` | — |
| `SortedDict` | `Set(K, V)` | `SetAll` | `SetAllSeq` | — |
| `Map` | `Set(K, V)` | **missing** | **missing** | — |
| `Vector` (proposed) | `Append(e)` | `AppendAll` | `AppendAllSeq` | — |

The dicts already do what this ADR proposes for `Vector`. **The sets are the
outliers**: variadic-primary, and `HashSet` has no bulk forms at all while
`SortedSet` has both. `Map` lacks them too, which may be correct — ADR `0009`
makes it a deliberately thin adapter.

Worth being precise about the reason, because it is not the one that drove
decision 3. The 41% measured there is a real cost on a 0.57 ns append and **noise
on a ~5 ns map insert**, so performance does not argue for changing the sets.
Consistency does, and that is a weaker argument that should be made on its own
terms rather than borrowed.

At minimum `HashSet` should gain `AddAll`/`AddAllSeq` to match `SortedSet`, since
that gap is an oversight rather than a decision.

### Review the naming scheme as a whole

ADR `0008` has now taken two exceptions out of five container names — `Map`, and
`Vector` per decision 6 — and the view interfaces mix concept names (`SetView`,
`DictView`) with implementation names (`SortedDictView`, and `VectorView` per
decision 5). Individually each is defensible; together they suggest the scheme
does not fit what the library turned out to be. Worth one pass over every
exported name at once, rather than another exception per ADR.

### A `Slice[T] []T` adapter will likely follow

The parallel is exact: `Slice` would be to `Vector` what `Map` is to `HashDict` —
free conversion from `[]T`, builtin syntax, working `encoding/json`, at the cost
of the shape rules. Deferred to its own ADR rather than bundled here.

### A sequence contract, once there is a second sequence

`MutableSeq[T]` with `At`, `Set` and `Append` would complete ADR `0008`'s
pattern, but one implementation cannot show what the contract should say.
Revisit when `LinkedList` (ADR `0010`) lands, and not before.
