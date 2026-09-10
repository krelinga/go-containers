# Reference containers, re-measured against the library ADR 0017 left behind

ADR `0014` rejected reference-type containers. Its decisive argument was a cost
that no longer exists, so the ledger is re-priced here rather than re-read.
See `bench.txt` for raw output and provenance; `doc.go` for the question.

## 1. The reference representation is still free

| 64 elements | `*ptrSet` (ADR `0002`) | `refSet` (value receivers) |
|---|---|---|
| `Has` | 4.252 ns | 4.215 ns |
| `Len` | 0.4229 ns | 0.3973 ns |
| `Keys` (full walk) | 335.2 ns, 0 allocs | 348.9 ns, 0 allocs |
| `KeySlice` | 431.5 ns, 1 alloc | 446.9 ns, 1 alloc |
| construct | 736.8 ns, 3 allocs | 738.7 ns, 3 allocs |

Indistinguishable, allocations included, on today's method set rather than the
toy set ADR `0014` used. `KeySlice` is new since ADR `0017` and is on the hot
path for every bulk operation; it does not change the answer.

## 2. The nil check is now at or below the noise floor

Builtin-map semantics on a zero value — total reads, panicking writes — costs:

| | `Has` | `Len` | `Keys` | `KeySlice` |
|---|---|---|---|---|
| `refSet` | 4.215 ns | 0.3973 ns | 348.9 ns | 446.9 ns |
| `refSet` + nil check | 4.287 ns | 0.3723 ns | 335.6 ns | 425.9 ns |

**~1.7% on a point read and nothing elsewhere**, against the **+8.5%** ADR
`0014` measured. The difference is the baseline, not the branch: a point read is
4.2 ns here against 2.99 ns there, so the same fixed branch is a smaller share.
The durable claim is the weaker one — *a predictable nil branch costs a fraction
of a nanosecond* — and how much that matters depends entirely on what it is
added to.

The placement finding from ADR `0014` is unchanged and still load-bearing: the
check must sit **inside** the closure a method returns, not before it. Two
returns yielding two different closures puts the closure on the heap.

## 3. Struct-shaped views are performance-neutral, where they used to cost 6.8x

This is the finding that re-opens the question. ADR `0014` charged struct views
with "an allocation whenever a view is passed to `Elems2`, which is what every
sized constructor takes". ADR `0017` deleted `Elems` and `Elems2` and made bulk
operations variadic in the element type, so **nothing in the library boxes a
view into a foreign interface any more.**

| | sealed interface (today) | `struct{iface}` |
|---|---|---|
| construct | 0.3751 ns, 0 allocs | 0.3701 ns, 0 allocs |
| `Has` | 4.871 ns | 4.828 ns |
| `Keys` (full walk) | 386.7 ns, **3 allocs** | 384.6 ns, **3 allocs** |
| `KeySlice` | 420.8 ns, 1 alloc | 422.7 ns, 1 alloc |
| substitute ordered → base | 0.6482 ns, 0 allocs | **0.2825 ns**, 0 allocs |

Neutral on every operation, and the explicit conversion that replaces interface
embedding is **cheaper than the embedding it replaces**, not a tax.

**A wrapper does not fix ADR `0013`'s erratum.** Iterating a view allocates
three times per call either way. An earlier cut of this harness wrapped a
*concrete* container, which is monomorphic, inlines, and showed 0 allocations —
but a real `SetView[T]` must view a `HashSet`, a `SortedSet` or a converting
view, so it has to wrap an interface, and the inner call stays dynamic. **The
monomorphic measurement flatters the design and is not shippable.**

## Durable / perishable

**Durable.** A reference struct is free relative to a pointer, on any method
set. A nil branch is a fraction of a nanosecond and must sit inside the returned
closure. A struct wrapping an interface neither costs nor saves against the bare
interface, because the dynamic call is still there — including the three
allocations per iterator returned through one. Replacing interface embedding
with an explicit conversion costs nothing at runtime. A polymorphic view cannot
wrap a concrete type, so monomorphic measurements of it are invalid.

**Perishable.** Every absolute number, and the *share* the nil check represents,
which moves with whatever the baseline operation costs.
