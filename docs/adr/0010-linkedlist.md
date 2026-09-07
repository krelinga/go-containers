# 10. LinkedList[V]: a doubly-linked list keyed by cursor

- **Status:** Proposed
- **Date:** 2026-09-06
- **Evidence:** `experiments/linkedlist/` (`RESULTS.md`)
- **Relates to:** ADR `0002` (shape rules), `0006` (`Elems2`), `0008` (naming and
  the contract layers).

The name is `LinkedList`, not `List`, so that `List` and `MutableList` stay
available as contract names. Using the bare concept word for an implementation
is what forced ADR `0008`'s whole vocabulary change, when `Map` blocked
`MutableMap`; this avoids repeating it. `LinkedList` also fits the existing
`<Backing><Concept>` shape beside `HashSet` and `SortedDict`.

The handle is `LinkedListCursor` for the same reason. In a flat package, a bare
`Cursor` would claim the general concept, and a future container with positional
handles — a tree, say — would then have nowhere to go. The name is verbose in
signatures; that is the price of not spending the general word on the first type
that needs it.

## Context

Every container here so far is keyed by a value. A doubly-linked list is the
first that is not: its natural handle is a **position**, and the operation it
exists for — removing an element in O(1) — needs one.

`experiments/linkedlist/` establishes that the container earns its place. Removing the
middle element and splicing it back is flat at 1.6 ns against a slice's O(n),
from 6x at 64 elements to nearly 19 000x at 262 144. The crossover sits below 64,
so there is effectively no size at which a slice wins that operation.

The usual objection is cache behaviour, and it needed measuring twice:

| n = 262 144 | list | slice | |
|---|---|---|---|
| raw loop | 193 855 ns | 50 201 ns | list 3.9x slower |
| through `iter.Seq` | 294 875 ns | 295 687 ns | **identical** |

Pointer chasing genuinely costs 3.9x, and an `iter.Seq` closure hides all of it —
the closure costs more than the cache misses. **Every container in this package
is iterated through `All()`**, so the penalty a caller can observe is zero. A
benchmark pairing a list's iterator against a raw slice range would have reported
~5.9x and blamed the list; that comparison measures the iterator.

## Call sites

Today, dropping elements from a sequence while keeping references to the rest:

```go
// []T -- every removal shifts the tail, and any index held elsewhere is now wrong
for i := len(s) - 1; i >= 0; i-- {
	if !keep(s[i]) {
		s = slices.Delete(s, i, i+1)
	}
}

// container/list -- O(1), but pre-generics: every value comes back as `any`
for e := l.Front(); e != nil; {
	next := e.Next()
	if !keep(e.Value.(Item)) { // type assertion at every use
		l.Remove(e)
	}
	e = next
}
```

Proposed:

```go
var drop []containers.LinkedListCursor[Item]
for c, v := range l.All() {
	if !keep(v) {
		drop = append(drop, c)
	}
}
for _, c := range drop {
	l.Remove(c)
}
```

Typed throughout, O(1) per removal, and the collect-then-remove shape that
`MutableDict` already requires of every container in this package.

## Decision

### 1. `LinkedList[V]` satisfies `Elems2[LinkedListCursor[V], V]`, keyed by position

```go
type LinkedListCursor[V any] struct{ n *node[V] }

func (l *LinkedList[V]) All() iter.Seq2[LinkedListCursor[V], V]
```

`LinkedListCursor` is opaque: an unexported pointer field, so callers can hold and compare
one but cannot construct or dereference it. It is comparable, since a struct of
comparable fields is, which matters because storing cursors in a map is the
point — an LRU index is exactly `HashDict[K, LinkedListCursor[V]]`.

**`LinkedList` satisfies `Elems2` and deliberately not `Dict`.** `Dict` requires
`Get(K) (V, bool)`, and for a list that bool can never be false: decision 2
panics on an invalid cursor, and a valid one always has a value. Rather than
carry a return value that is always `true`, the accessor is `At(c) V`, which
panics. Dropping `Dict` also removes an oddity: `CollectHashDict(list)` would
otherwise compile and build a dict keyed by opaque cursors that nothing could
look up.

Keying on position rather than value is what makes the removal handle available
from the standard iteration method, so the collect-then-remove pattern works
through the contract rather than through a second, list-specific API.

**This makes the key a reference with a lifetime tied to the container**, which
no other implementation here has. Map and dict keys are values; they cannot
outlive their entry. Decision 2 is what makes that safe rather than silent.

### 2. Nodes carry a back-pointer; stale and foreign cursors panic

Each node holds a pointer to its owning list. `Remove` nils it, along with the
node's links. Any method taking a `LinkedListCursor` checks `c.n.list == l` first and
panics otherwise, consistent with this package's uniform nil-receiver rule.

This catches two distinct mistakes:

- a **stale** cursor, whose element has been removed
- a **foreign** cursor, belonging to a different list

Without it, `experiments/linkedlist/` shows a removed cursor reads successfully and
returns a value no longer in the list — a silent wrong answer, not a crash.

**The cost was measured and is structural, not per-access.** Validation itself is
free — unchecked, tombstoned and back-pointer reads all measure 0.3–0.4 ns,
because the branch inlines and predicts. What the back-pointer costs is:

| | node, V=int | node, V=string | GC cycle, 200k retained | build 65 536 |
|---|---|---|---|---|
| 2 pointers | 24 B | 32 B | 1 414 773 ns | 1 125 084 ns |
| 3 pointers | 32 B **(+33%)** | 40 B **(+25%)** | 1 537 531 ns **(+8.7%)** | +15% |

A cheaper mechanism exists and was rejected — see Rejected alternatives. The
back-pointer is chosen because a foreign cursor is easy to produce once several
lists are in play, fails arbitrarily far from its cause, and this package has
consistently paid small costs for loud failure.

### 3. Surface

```go
type LinkedListCursor[V any] struct{ /* unexported */ }

func NewLinkedList[V any]() *LinkedList[V]
func CollectLinkedList[V any](src Elems[V]) *LinkedList[V]
func CollectLinkedListSeq[V any](seq iter.Seq[V]) *LinkedList[V]

// Reading. Satisfies Elems2[LinkedListCursor[V], V].
func (l *LinkedList[V]) Len() int
func (l *LinkedList[V]) All() iter.Seq2[LinkedListCursor[V], V]
func (l *LinkedList[V]) At(c LinkedListCursor[V]) V
func (l *LinkedList[V]) IsValid(c LinkedListCursor[V]) bool
func (l *LinkedList[V]) Front() (LinkedListCursor[V], bool)
func (l *LinkedList[V]) Back() (LinkedListCursor[V], bool)

// Insertion. Each returns a cursor to the new element.
func (l *LinkedList[V]) PushFront(v V) LinkedListCursor[V]
func (l *LinkedList[V]) PushBack(v V) LinkedListCursor[V]
func (l *LinkedList[V]) InsertBefore(c LinkedListCursor[V], v V) LinkedListCursor[V]
func (l *LinkedList[V]) InsertAfter(c LinkedListCursor[V], v V) LinkedListCursor[V]

// Mutation.
func (l *LinkedList[V]) Set(c LinkedListCursor[V], v V)
func (l *LinkedList[V]) Remove(c LinkedListCursor[V])
func (l *LinkedList[V]) MoveToFront(c LinkedListCursor[V])
func (l *LinkedList[V]) MoveToBack(c LinkedListCursor[V])

func (l *LinkedList[V]) Clone() *LinkedList[V]
```

**Every method taking a `LinkedListCursor` panics on an invalid one**, except `IsValid`,
which is the way to ask without panicking. `IsValid` is a method on the list
rather than on `LinkedListCursor` because validity is relative to a list: the same cursor
is valid for its owner and foreign to any other.

`Front` and `Back` return a bool, false on an empty list, because there is no
cursor to hand back — unlike `At`, where the caller already holds one.

`Set` replaces the value at a cursor in place, keeping the cursor valid. Without
it, changing a value means `InsertBefore` plus `Remove`, which changes the
element's identity and invalidates any cursor held elsewhere.

**`CollectLinkedList` has no performance advantage over `CollectLinkedListSeq`**
and is kept for ergonomics: every other container offers the sized form, and a
caller converting between containers should not have to remember which ones
benefit. Its doc comment must say so, as `HashDict.SetAll`'s does — a size hint
is only worth something when there is a backing array or map to presize, and a
linked list allocates per node. See Rejected alternatives for the slab that
would have made the hint real.

No `Clear`, no `Delete`, and no other `MutableDict` operations: a list is not a dict.

### 4. ADR 0002's shape rules apply

`noCopy` declared first, uniform pointer receivers, a usable zero value, uniform
nil-pointer panics, and the eager-dereference rule. `LinkedListCursor` itself is a plain
comparable value and carries none of this — it is a handle, not a container.

## Consequences

- **A cursor pins its node, and through the back-pointer, the list.** Holding one
  after dropping the list keeps both alive. `Remove` nils the node's links and
  its list pointer, so a removed node stops pinning anything.
- **`LinkedListCursor` is comparable, so callers can use it as a map key.** That is genuinely
  useful — an LRU index is exactly `map[K]LinkedListCursor[V]` — and it is also how a
  foreign cursor gets introduced, which decision 2 catches.
- **The iteration penalty is zero through `All()` and 3.9x if a caller could
  traverse raw.** They cannot, so this is a property of the package's design
  rather than a caveat, but it would stop being true if a raw accessor were ever
  added.
- Nodes are +25–33% larger than they would be unvalidated, and the GC re-scans
  the extra pointer every cycle for as long as the list is retained.
- **There is no `Clear`.** No container here has one, and adding it to this one
  alone would introduce an inconsistency rather than resolve it; see follow-ups.

## Rejected alternatives

- **A tombstone instead of the back-pointer** — on removal, point the node's
  `prev` at itself, which a live node can never satisfy. Measured: it catches
  stale cursors with **no extra field and the same free check**. Rejected because
  it does not catch a foreign cursor, and that is the mistake most likely to
  arise once cursors are stored in a map alongside several lists. The whole cost
  of decision 2 buys exactly that case, which is worth stating plainly.
- **Leaving stale cursors undefined**, as `container/list` does. Rejected because
  the failure is silent: the probe in `experiments/linkedlist/` shows a removed cursor
  returning a real-looking value.
- **Allocating nodes from a slab** — `make([]node[V], n)` linked together, one
  allocation instead of n, which would give `CollectLinkedList`'s size hint a
  real performance justification. Rejected on measurement: Go frees whole heap
  objects, so an interior pointer to a single element retains the entire slab.
  With the slice header dropped and exactly one element held, a 200 000-node slab
  stayed fully resident at 26.0 MB, where individually allocated nodes fell to
  0.0 MB. One surviving element would pin the whole block. The slab's memory
  advantage is small anyway — 26.0 MB against 29.0 MB, about 11%.
- **A non-generic `LinkedListCursor`**, holding `any` under the hood and type-asserting to
  `*node[V]` before the ownership check. It works: boxing a pointer into an
  interface does not allocate, the struct still satisfies `comparable`, and
  cursors of different element types correctly compare unequal. It also catches a
  third mistake — a cursor from a list of a different `V`.

  Rejected on two grounds. The measured cost is 2x on both axes that matter for
  a handle: **16 bytes against 8**, which an LRU index pays per cached entry, and
  **0.57 ns against 0.29 ns** per access. More importantly, the extra mistake it
  catches is one the generic form makes **impossible at compile time** —
  `intList.At(stringLinkedListCursor)` does not compile. Adopting it would convert a
  compile error into a runtime panic, which is the wrong direction, in exchange
  for a heterogeneous cursor collection this library has no use for.

- **Keying on an integer index** rather than a cursor. Rejected because indices
  shift on every insertion and removal, which is precisely what a linked list is
  chosen to avoid, and because index access would be O(n).
- **Satisfying only `Elems[V]`**, with cursors exposed through a separate
  iterator. This is the more honest description of a list — a sequence of values —
  and it would keep reference-lifetime semantics out of the shared contracts.
  Rejected because it splits iteration into two methods and puts the removal
  handle outside the contract every other container uses.

## Follow-ups

- Whether `LinkedListCursor` should expose `Next`/`Prev` for stepping without iterating.
- **When an operation panics versus returns a bool**, decided package-wide. The
  implicit rule today is *invalid handle or nil receiver panics; absent or empty
  returns a bool* — so `At` panics on a bad cursor while `Front`, `Back`,
  `Min`, `Max`, `Floor`, `Ceil` and `Get` all report absence with a bool. That
  line is defensible: a stale cursor is a programmer error, an empty container is
  a state. But it is nowhere written down, and `At` panicking beside `Front`
  returning a bool is exactly the juxtaposition that makes a reader doubt it.
  Worth settling as a stated rule and auditing every method against, rather than
  deciding per container.
- **`Clear` across the library**, which wants its own ADR because it is not
  uniform. For the map- and slice-backed containers it is O(1) — drop the
  backing and let the zero value take over. For `LinkedList` it is **O(n)**: it
  must walk the chain nilling each node's back-pointer, because detaching
  without walking would leave every outstanding cursor looking valid while
  pointing into an orphaned list. A single operation whose complexity differs by
  container is exactly the sort of thing to decide once, deliberately, rather
  than per type.
- `List` and `MutableList` as contract names, once there is a second sequence
  implementation to design them against. The name `LinkedList` was chosen to keep
  them free.
- An LRU cache built from `LinkedList` plus `HashDict[K, LinkedListCursor[V]]` would exercise the
  foreign-cursor case decision 2 exists for, and is the obvious test of whether
  the design holds up.
