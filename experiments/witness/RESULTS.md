# The carried witness: static conversion for views

ADR `0011` measured a type-parameter witness as the cheapest view shape and
priced `Get`. ADR `0012` closed it off, because a witness must be stateless and
`FromKeyView` generally needs a registry. ADR `0013` then found that iterating a
view allocates per call — an erratum neither had in view.

This harness prices the witness on that operation, and separates two variants
nobody had separated:

- **pure** — `struct{ c C }` plus a type parameter `VW`, its zero value
  materialised per call. One word. **Stateless viewers only** — this is what
  ADR `0012` rejected.
- **carried** — `struct{ vw VW; c C }` with `VW` a *concrete* type parameter and
  a field. The call is still static, because `VW` is known at compile time, and
  the viewer may hold state.

See `bench.txt` for raw output and provenance.

## 1. The witness removes ADR 0013's erratum

Iterating a converting view over 64 elements:

| | | allocs |
|---|---|---|
| the container, no view at all | 350.2 ns | **0** |
| view holding a **shape interface** (today) | 552.8 ns | **6** |
| view with a **pure** witness | 411.2 ns | **0** |
| view with a **carried** witness, stateless viewer | 423.1 ns | **0** |
| view with a **carried** witness, **stateful** viewer | 401.3 ns | **0** |

**Zero allocations, including with a stateful viewer.** An `iter.Seq` returned
through a dynamic call cannot be stack-allocated at the range site; a static
call has nothing to box. The remaining ~60 ns over the bare container is the
per-element conversion, which is work the view exists to do.

## 2. ADR 0012's objection applies to one variant only

A **carried** witness keeps the viewer in a field, so a registry-holding viewer
works and still dispatches statically — 401.3 ns and no allocations above,
against 552.8 ns and six for the interface form.

That is the finding: *"a witness must be stateless"* is true of the pure form
and false of the carried one. ADR `0011` measured the pure form; ADR `0012`
rejected the idea on the pure form's constraint; the carried form appears never
to have been considered.

## 3. Point reads

| | |
|---|---|
| view holding a shape interface | 1.290 ns |
| pure witness | 0.3641 ns |
| carried witness | 0.3749 ns |

**3.4x**, and the same shape as every other dispatch result in this repo: a
fixed indirect-call cost, large as a share of something cheap.

## 4. Field order decides whether a stateless view is one word

A zero-size field at the END of a struct is padded, so a pointer to it cannot
run past the allocation. At the FRONT it costs nothing.

| | width | boxing into a shape interface |
|---|---|---|
| carried, viewer **last**, stateless | 16 B | 12.27 ns, **1 alloc** |
| carried, viewer **first**, stateless | **8 B** | **0.3105 ns, 0 allocs** |
| carried, viewer first, **stateful** | 16 B | 12.23 ns, 1 alloc |
| interface view (today) | 16 B | 12.23 ns, 1 alloc |

With the viewer declared first, a **stateless** converting view is one word and
boxes free — better than today on every axis. A **stateful** one is two words
and boxes exactly as today does. Declare the fields in the wrong order and the
first row is what you get, silently.

## 5. The parameterised container: half the erratum, and a span merge

Putting the witness on the CONTAINER and keeping the view as an interface:

| iterating 64 elements | | allocs |
|---|---|---|
| view holds an interface (today) | 552.6 ns | **6** |
| witness on the container, view is an interface | **440.3 ns** | **3** |
| carried witness, concrete view | 408.4 ns | **0** |
| the same container used concretely | 399.1 ns | **0** |
| boxing the container into the view interface | 0.3154 ns | **0** |

Today has two dynamic hops; this shape makes the per-element conversion static
and leaves one. Half the allocations, not none — one dynamic call is enough to
defeat the fusion described below.

## 6. Where the three allocations are, and when they matter

| | allocs |
|---|---|
| obtain the iterator only, through the interface | **1** |
| obtain the iterator only, concretely | 1 |
| obtain and range, through the interface | **3** |
| obtain and range, concretely | **0** |
| range a pre-obtained iterator | **2** |

One is the iterator closure; two are the range machinery — escape analysis names
the loop variable, the range-over-func state, and the yield closure.

**The concrete pair is the instructive one.** Obtaining a closure concretely also
costs one allocation alone, yet the full concrete loop costs zero: ranged
immediately, the compiler inlines the method, proves the closure never outlives
the loop, and fuses both into a plain loop. Through an interface it cannot know
which method will run, so nothing fuses.

**Scaling.** The overhead is a flat ~34.4 ns; the walk is ~6.81 ns per element.

| n | concrete | via view | overhead |
|---|---|---|---|
| 0 | 3.3 ns | 35.9 ns | **990%** |
| 8 | 43.5 ns | 75.9 ns | 74% |
| 32 | 220 ns | 256 ns | 16% |
| 64 | 435 ns | 458 ns | 5.4% |
| 128 | 816 ns | 831 ns | 1.8% |
| 1024 | 6.90 µs | 6.76 µs | −2.1% |
| 4096 | 27.6 µs | 26.0 µs | −5.6% |

Below 10% at n≈50, below 5% at n≈100, below 1% at n≈500; negative past a few
hundred, which is jitter rather than signal. The cost is per call, not per
element, and the **empty container is the worst case** at 990%.

For scale, on the same machine: a map lookup is 2.7 ns, an uncontended mutex
pair 7.7 ns, a 64 B allocation 14.1 ns, `fmt.Sprintf("%d")` 27.0 ns,
`time.Now()` 35.3 ns, `json.Marshal` of two fields 58.6 ns, a goroutine spawn
222 ns, `os.Stat` 617 ns. **The overhead is one `time.Now()`.**

## 7. An `Each` escape hatch removes the overhead entirely

`Each(f func(T) bool)` — the container drives the loop and calls `f` per
element, `false` stops it, the same contract as `sync.Map.Range`. No `iter.Seq`
is returned, so there is no closure to hand back and nothing for a range
statement to bind.

Iterating a converting view, callback **not capturing** (or hoisted out of the
hot loop):

| n | container (no view) | view, `range Keys()` | view, `Each` |
|---|---|---|---|
| 0 | 3.5 ns / 0 | 75.8 ns / **6** | **3.8 ns / 0** |
| 1 | 23.9 ns / 0 | 100.1 ns / **6** | **24.8 ns / 0** |
| 8 | 36.1 ns / 0 | 129.1 ns / **6** | **43.1 ns / 0** |
| 64 | 345 ns / 0 | 565 ns / **6** | **404 ns / 0** |
| 1024 | 5.73 µs / 0 | 7.92 µs / **6** | **6.75 µs / 0** |

**The fixed per-call overhead goes from ~72 ns to ~0.6 ns — two orders of
magnitude — and the allocations go to zero.** On the parameterised container
(finding 5) the same hatch takes 3 allocations to 0 and ~33 ns to ~1.1 ns.

What remains at larger n is per-element, not per-call: the view's `Keys` yields
through a nested chain (`convertedKeys` yielding over the container's own
`iter.Seq`), and each link costs a state check per element. `Each` collapses the
chain into ordinary inlined calls. That is why the gap at n=1024 (17%) exceeds
the fixed cost.

### The caller's own closure is the only thing left that allocates

| view, n=64 | | allocs |
|---|---|---|
| `Each`, callback literal written at the call site | 430 ns | **2** |
| `Each`, same callback hoisted out of the loop | 408 ns | **0** |
| `Each`, package-level func value | 404 ns | **0** |

A callback literal capturing a local costs two allocations — the closure and the
captured variable — because passing it through a dynamic call makes it escape.
That is the **caller's** allocation, not the container's, and hoisting the
closure removes it. Worth a doc line: the hatch is only a hatch if the callback
does not allocate.

### How far the callback has to move: one level, not global scope

Every form below iterates the same 64-element view. What changes is only where
the callback is written.

| | n=64 | allocs | |
|---|---|---|---|
| A. literal at the call site | 421 ns | **2** | closure + captured local both escape |
| B. same closure, same capture, **one level out** | 414 ns | **0** | |
| C. closure over a pointer to a local accumulator | 411 ns | **0** | |
| D. method value `acc.add`, taken at the call site | 427 ns | **1** | a method value *is* a closure |
| E. same method value, bound one level out | 410 ns | **0** | |
| F. package-level func over package-level state | 405 ns | **0** | |
| — `range Keys()`, for comparison | 577 ns | 6 | |

**B is the answer: move the func value out of the hot loop and stop.** It still
captures a local, and the local still escapes — once, not per call, which is
what `allocs/op` is measuring. Package scope (F) buys nothing over B, and costs
the caller shared mutable state. The accumulator can stay a local; only the
**binding** has to be hoisted.

D is the one that surprises: `acc.add` is not a free reference to a method, it
is a closure over `&acc` built at the point the method value is taken — so it
allocates at the call site and is free one level out, exactly like a literal.

**The trap is a helper function.** Hoisting one level is enough only when the
loop is right there:

```go
func countVia(v View) int {
	n := 0
	v.Each(func(string) bool { n++; return true })   // rebuilt per CALL
	return n
}
```

That is 434 ns and **2 allocations per call to `countVia`**, no matter how far
outside `countVia` the loop sits. Taking the callback as a parameter and letting
the caller own it gives 405 ns and **0**. So the rule is not "one lexical level"
but *"outside whatever loop is hot"* — and a function called in a hot loop is a
hot loop.

### How much the caller's closure actually costs

Two allocations regardless of n, and a fixed ~16-20 ns of time:

| n | literal at call site | hoisted one level |
|---|---|---|
| 0 | 20.0 ns / 2 | **4.1 ns / 0** |
| 1 | 42.5 ns / 2 | **25.3 ns / 0** |
| 8 | 64.2 ns / 2 | **44.1 ns / 0** |
| 64 | 430 ns / 2 | **409 ns / 0** |
| 1024 | 6.66 µs / 2 | 6.53 µs / 0 |

So hoisting matters exactly where the hatch itself matters, and nowhere else:
**4.9x on an empty container**, ~5% at n=64, and **nothing at n=1024**, where the
two sample ranges overlap and the gap is not resolvable. Across two full runs the
n=64 gap read 10% once and 5% once, so treat it as "a few percent" rather than a
figure. The literal is consistently the noisier of the two, which is the
allocator showing through and is a reason to hoist that the means understate.

**And the naive un-hoisted form still beats range-over-func by 3.8x at n=0**
(20.0 ns / 2 allocs against 75.8 ns / 6) — hoisting collects the remainder, it is
not the price of entry.

### On a concrete container none of this applies

A literal written at the call site of `set.Each` costs **0 allocations** (310 ns,
against 414 ns for the best view form):
escape analysis can see the callee, proves the closure does not outlive the
call, and keeps it on the stack. Only the **dynamic** call forces it to the
heap — the same cause as the three-to-six allocations the hatch exists to avoid,
reappearing one level up in the caller's own code.

### It beats every hatch callers already have

Four things a caller can try with no change to the library. At n=64, through a
view:

| | | allocs | bytes |
|---|---|---|---|
| `range Keys()`, per call | 589 ns | 6 | 192 B |
| `range Keys()`, `Seq` **hoisted** out of the loop | 565 ns | **5** | 144 B |
| `Keys()(f)` — invoking the sequence by hand | 536 ns | **4** | 152 B |
| `KeySlice()`, then range the slice | 525 ns | 1 | **1152 B** |
| `iter.Pull(Keys())` | **3831 ns** | **10** | 360 B |
| `Each` | **430 ns** | **0** | **0 B** |

**`Keys()(f)` is the interesting one**, because `iter.Seq[T]` *is*
`func(yield func(T) bool)` — so a caller can invoke the sequence directly rather
than ranging it, and `c.Each(f)` and `c.Keys()(f)` are the same operation
written two ways. Invoking by hand skips the range statement and saves two
allocations of six, which is the best a caller can do unaided. It is still **14x
the hatch's overhead at n=0** (56.5 ns / 4 allocs against 3.9 ns / 0), because
`Keys()` still returns a closure through a dynamic call and the nested walk
inside that closure still allocates its own machinery.

**`iter.Pull` is far worse than the problem it would solve** — 10 allocations,
and at n=1024 **57.7 µs against 6.71 µs**, an 8.6x, because every element
crosses a coroutine switch. It is the right tool for interleaving two sequences
and the wrong one for going faster.

Two further things that are easy to assume and are false:

- **Hoisting the `iter.Seq` out of the loop saves one allocation of six.** The
  range machinery — loop state, loop variable, yield closure — is per *range
  statement*, not per `Keys()` call, so re-ranging a hoisted `Seq` pays five of
  the six every time. The zero-API-cost workaround does not work.
- **Materialising is not a cheap out.** `KeySlice` trades six fixed allocations
  for one sized by n: 1152 B at n=64 and 18 KiB at n=1024, and it is slower than
  `Each` at every size measured (8.08 µs against 6.59 µs at n=1024), because it
  copies every element.

### Early exit is where it is worth the most

Breaking out after the first of 1024 elements, through a view:

| | | allocs |
|---|---|---|
| `range Keys()` + `break` | 101.3 ns | 6 |
| `Each` + `return false` | **22.3 ns** | **0** |

**4.5x.** The fixed cost is the whole cost when the walk stops immediately, so a
short-circuiting search — `Any`, `Find`, `First` — is the profile that gains
most, and it is a common one.

### What it costs

Not measurable, but real: `Each` gives up what range-over-func was introduced to
provide. A callback cannot `break` to a label, `continue` an outer loop,
`return` from the enclosing function, or `defer` into its frame; stopping early
means threading a sentinel back through the `bool`. It also does not compose
with `slices.Collect`, `maps.Insert` or anything else in the stdlib that
consumes an `iter.Seq`. It is an escape hatch and reads like one — which is the
argument for having it *alongside* `Keys`, and against it replacing anything.

## 8. Routing the range-shaped hatch through a span costs one allocation

ADR `0021` needs a hatch for `Range`/`RangeKeys`, and ADR `0019`'s spans look
like they supply it free: if `Range` returns a span, the span carries `EachKey`
and no new method name is needed. Measured with `Range` called on a view
**interface** — the shape ADR `0020`'s favoured design produces, where a span
*is* a view — over a 64-element container:

| | | allocs |
|---|---|---|
| `Range(lo,hi).Keys()`, ranged | 524 ns | 4 |
| `Range(lo,hi).EachKey(f)` | 503 ns | **1** |
| `EachKeyInRange(lo, hi, f)` | **467 ns** | **0** |

**The one allocation is the span being boxed.** A span is a container plus two
bounds — wider than one word, so it cannot box free — and the allocation lands
on exactly the call the hatch exists to make free. A direct method has no span
value to box and reaches zero.

So "spans supply the range hatch for free" is false: the trade is **one
allocation and no new names, against zero allocations and four new names.**

### Warning: a monomorphic interface reads as free

Every row above measures 0 allocations if the interface has one visible
implementation assigned to a local variable — the compiler devirtualizes, proves
the dynamic type, inlines through it, and the call is not dynamic at all. The
harness uses two implementations behind a runtime-selected branch to prevent
that.

**This is the third time in this line of work that a monomorphic interface has
flattered a design** (ADR `0020` records the first two). Any measurement in this
repo that puts a concrete type into an interface and then calls through it
locally should be assumed wrong until a second implementation is added.

## Durable / perishable

**Durable.** A static conversion removes the per-call allocations that a
dynamic one forces on a returned iterator, whether or not the viewer holds
state. A carried witness dispatches statically *and* carries state; only the
pure form is constrained to stateless viewers. A zero-size struct field is free
at the front of a struct and padded at the back, which decides pointer-shapedness
and therefore whether boxing allocates.

A dynamic call defeats the compiler's fusion of an iterator constructor with the
range statement that consumes it, which is what makes the three allocations
appear; the fixed cost is per call rather than per element, so it is dominated
by call frequency and not by container size.

A callback-driven `Each` sidesteps the whole mechanism, because nothing is
returned for a range statement to bind — so it costs zero allocations through a
dynamic call, independent of view shape. The range machinery is per range
*statement*, not per iterator call, so hoisting an `iter.Seq` out of a loop
saves only the constructor's own allocation. A callback literal that captures a
local escapes through a dynamic call and allocates twice; hoisting the closure
removes that -- and hoisting means "out of whatever loop is hot", which is one
lexical level when the loop is adjacent and further when the call sits inside a
helper; package scope is never required. A method value on a pointer receiver is
itself a closure, so it obeys the same rule as a literal. Under a concrete
(non-dynamic) call the literal never escapes at all, so none of this applies.
Callback iteration cannot `break`, `continue`, `return` or `defer`
through its caller's frame, and does not compose with the stdlib's `iter.Seq`
consumers — the cost that makes it a hatch rather than a default.

An `iter.Seq` **is** a push function, so invoking it by hand (`Keys()(f)`) is the
same operation as a hatch method: it saves the range machinery but not the
closure returned through the dynamic call. `iter.Pull` converts push to pull at a
coroutine switch per element, which is an order of magnitude dearer than the cost
it would avoid. A value wider than one word cannot be boxed into an interface
without allocating, so returning a span through an interface costs one allocation
however cheap the span's own iteration is.

A monomorphic interface — one visible implementation, assigned to a local — is
devirtualized and reads as free. A measurement of dynamic-dispatch cost is
invalid without a second implementation behind a runtime-selected branch.

**Perishable.** Every absolute number, and the ~60 ns conversion overhead, which
tracks the viewer's own cost.
