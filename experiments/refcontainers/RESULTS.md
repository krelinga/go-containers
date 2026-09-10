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

## 4. The all-interface shape: free on the bulk path, 4.3x on an indexed loop

Making the container itself a sealed interface, so containers and views are the
same kind of thing. ADR `0014` priced this against a toy method set; ADR `0017`
changed the method set.

| 64 elements | `*ptrSet` | `ifaceSet` | |
|---|---|---|---|
| `KeySlice` (ADR `0017`'s bulk path) | 410.9 ns, 1 alloc | 421.7 ns, 1 alloc | **+2.6%** |
| `Has` | 4.292 ns | 4.948 ns | +15% |
| `Keys` (full walk) | 319.6 ns, **0 allocs** | 384.1 ns, **3 allocs** | +20% |
| construct | 742.0 ns, 3 allocs | 796.2 ns, **5 allocs** | +7% |
| `Union` | 1.546 µs, 3 allocs | 1.741 µs, **6 allocs** | +13% |
| `Len` | 0.3734 ns | 1.181 ns | **+216%** |
| `At` (indexed) | 0.4914 ns | 1.130 ns | **+130%** |
| **indexed loop over 64** | 16.11 ns | 68.92 ns | **+328%** |

**The bulk path is free through an interface.** A slice returned through a
dynamic call carries no per-call allocation the way an `iter.Seq` does, so ADR
`0017`'s currency is indifferent to dispatch. This is the one place the
all-interface shape got cheaper since ADR `0014`.

**Everything proportional to how cheap the operation is got no better.** The cost
is a fixed indirect call, so it is invisible against a map probe and enormous
against an index. An indexed loop is **4.3x**, because `At` cannot inline and
bounds-check elimination is impossible through dispatch.

**Nil semantics, verified rather than argued** (`nil_test.go`): a zero container
interface `== nil` — one spelling for containers and views, no method needed —
and **every method panics on it, reads included**. A nil builtin map reads fine;
a nil interface has no dynamic type to dispatch to. The seal survives: a
container carries `sealedContainer()` and a view `sealedView()`, so neither
satisfies the other (`missing method sealedView`).

**Set algebra on the contract compiles**, which it cannot for concrete
containers: `Union(ifaceSet[T]) ifaceSet[T]` satisfies itself where
`Union(*ptrSet[T]) *ptrSet[T]` cannot satisfy `Union(SetAlgebra) SetAlgebra`.
That ceiling has stood since ADR `0002` and this is the only shape that removes
it.

## 5. Option (b) end to end, and a refinement to the placement rule

Option (b) is a reference container plus a struct-shaped view, both nil-checked
so their zero values agree. Measured as the complete shape rather than half of
it:

| | status quo | option (b) |
|---|---|---|
| container `Has` | 4.289 ns | 4.382 ns |
| container `KeySlice` | 420.8 ns, 1 alloc | 446.4 ns, 1 alloc |
| view `Has` | 4.819 ns | 4.912 ns |
| view `Keys` | 374.3 ns, 3 allocs | 368.9 ns, 3 allocs |
| view `KeySlice` | 417.6 ns, 1 alloc | 419.0 ns, 1 alloc |
| the emptiness check | 0.1890 ns (`== nil`) | 0.1874 ns (`IsZero()`) |

**Free on every path, including the emptiness check itself.** `IsZero()` and
`== nil` are indistinguishable, which is worth stating plainly: option (b) does
not make the check cheaper or dearer, so its nil story is a lateral move rather
than a win.

**Both sides need the check, or they disagree.** A zero reference container reads
as empty by design (ADR `0014` decision 3). A zero `struct{iface}` view **panics**
on every method, because the field is a nil interface with nothing to dispatch
to. Verified in `nil_test.go` across all three states. Making them agree means
nil-checking every method on the view side too — roughly double the boilerplate
of the container-only design, on every method of every container and every view.

**The placement rule from ADR `0014` needs one more clause.** `0014` established
that the check must sit *inside* the closure a method returns. That is necessary
and not sufficient: how the checked closure reaches the inner sequence matters
just as much.

| nil-checked view iterator | | allocs |
|---|---|---|
| no check (bare interface) | 406.6 ns | 3 |
| check, then **range and re-yield** | 481.9 ns | **5** |
| check, then **hand `yield` through** | 404.8 ns | **3** |

```go
// costs a second closure on the heap
for t := range impl.Keys() { if !yield(t) { return } }

// free
impl.Keys()(yield)
```

Ranging over the inner sequence and re-yielding builds a second closure; passing
`yield` straight to it does not. **The full rule: the check goes inside the
returned closure, and the closure must not re-yield.**

## Durable / perishable

**Durable.** A reference struct is free relative to a pointer, on any method
set. Dispatch is a fixed cost, so its *share* tracks how cheap the operation is:
negligible on a map probe, dominant on an index — and free on a path whose
result is a slice. A nil branch is a fraction of a nanosecond and must sit inside the returned
closure. A struct wrapping an interface neither costs nor saves against the bare
interface, because the dynamic call is still there — including the three
allocations per iterator returned through one. Replacing interface embedding
with an explicit conversion costs nothing at runtime. A polymorphic view cannot
wrap a concrete type, so monomorphic measurements of it are invalid.

**Perishable.** Every absolute number, and the *share* the nil check represents,
which moves with whatever the baseline operation costs.
