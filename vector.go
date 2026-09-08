package containers

import (
	"iter"
	"slices"
)

// A Vector is a sequence that owns its backing slice.
//
// The zero value is an empty Vector ready to use:
//
//	var v containers.Vector[string]
//	v.Append("read")
//
// # What it is for
//
// A slice header is a *value*, so length and backing pointer are copied on
// every assignment and every call. Three hazards follow, and a Vector removes
// all three by owning the slice behind a pointer:
//
//   - **Appends off a shared header alias.** With spare capacity, two appends
//     from one header write the same backing cell and the first result is
//     silently overwritten.
//   - **A callee's append is invisible** unless it returns the slice and the
//     caller reassigns. Nothing at the call site says so.
//   - **Reallocation splits holders.** Two names for one slice share a backing
//     array until it grows, and stop sharing afterwards.
//
// Every holder of a *Vector reaches the same header, so an append is seen by
// all of them and **a reallocation is not observable.**
//
// The stronger reason is ADR 0001's: a []T field cannot be handed out
// read-only, so an accessor over one must copy — O(n) per call, and O(n²) when
// called in a caller's loop — or return a mutable interior. A Vector field can
// have a view instead, at O(1) and no allocation. See IndexedView.
//
// # What it is not
//
// **A Vector is not a replacement for []T.** Building a slice locally gains
// nothing from one, and reading a sequence you already hold is measurably worse:
// an indexed loop costs ~24% because bounds-check elimination does not survive
// At, and ranging All costs ~6x a raw range — though that last figure is the
// price of iter.Seq itself, which slices.Values pays too. A Vector earns its
// place at an API boundary, not in local code. See ADR 0015 and
// experiments/vectorcost.
//
// Every method has a pointer receiver, so calling one on an addressable Vector
// value takes its address automatically. A nil *Vector is not usable: every
// method panics on one, deliberately.
//
// A Vector is not safe for concurrent use.
type Vector[T any] struct {
	_  noCopy
	es []T
}

// NewVector returns a Vector containing vs. It is a convenience for
// construction with initial values; the zero value of Vector is equally usable.
//
// vs is copied, so the caller's slice and the Vector do not share a backing
// array.
func NewVector[T any](vs ...T) *Vector[T] {
	return &Vector[T]{es: slices.Clone(vs)}
}

// CollectVector returns a Vector holding everything src yields, preallocated
// from src.Len. See ADR 0006.
func CollectVector[T any](src Elems[T]) *Vector[T] {
	n := src.Len() // eager, so a nil src panics
	seq := src.All()
	v := &Vector[T]{es: make([]T, 0, max(0, n))}
	v.AppendAllSeq(seq)
	return v
}

// CollectVectorSeq is CollectVector for a bare iterator, which cannot report
// its length.
func CollectVectorSeq[T any](seq iter.Seq[T]) *Vector[T] {
	v := &Vector[T]{}
	v.AppendAllSeq(seq)
	return v
}

// Len reports how many elements the Vector holds.
func (v *Vector[T]) Len() int { return len(v.es) }

// At returns the element at i. It panics if i is out of range, exactly as
// indexing a slice does.
//
// There is deliberately no comma-ok form: matching the builtin is the point of
// an index accessor, and a caller who needs to test has Len.
func (v *Vector[T]) At(i int) T { return v.es[i] }

// Set replaces the element at i. It panics if i is out of range.
func (v *Vector[T]) Set(i int, e T) { v.es[i] = e }

// Append adds one element to the end.
//
// It is deliberately not variadic, unlike HashSet.Add. A variadic signature
// costs about 40% of an append, which is noise against a map insert and half
// the operation again against a slice append. Use AppendAll or AppendAllSeq for
// more than one. See ADR 0015 decision 3.
func (v *Vector[T]) Append(e T) { v.es = append(v.es, e) }

// AppendAll adds everything src yields, in order, preallocating from src.Len.
//
// src is fully consumed as it is applied.
func (v *Vector[T]) AppendAll(src Elems[T]) {
	n := src.Len()                     // eager, so a nil src panics
	es := slices.Grow(v.es, max(0, n)) // eager, so a nil receiver panics
	for e := range src.All() {
		es = append(es, e)
	}
	v.es = es
}

// AppendAllSeq is AppendAll for a bare iterator, which cannot report its
// length.
func (v *Vector[T]) AppendAllSeq(seq iter.Seq[T]) {
	es := v.es // eager, so a nil receiver panics even when seq is empty
	for e := range seq {
		es = append(es, e)
	}
	v.es = es
}

// All iterates the elements in order.
//
// The iterator binds the backing slice at call time, so a nil receiver panics
// here rather than at iteration. See ADR 0002.
func (v *Vector[T]) All() iter.Seq[T] {
	es := v.es
	return func(yield func(T) bool) {
		for _, e := range es {
			if !yield(e) {
				return
			}
		}
	}
}

// AllIndexed iterates index and element together.
//
// It cannot be called All: Elems[T] already claims that name for the
// value-only sequence, and a type cannot have both.
func (v *Vector[T]) AllIndexed() iter.Seq2[int, T] {
	es := v.es
	return func(yield func(int, T) bool) {
		for i, e := range es {
			if !yield(i, e) {
				return
			}
		}
	}
}

// Clone returns an independent Vector with the same elements. The backing
// slices are separate; the elements themselves are copied as values, so a
// Vector of pointers yields a Vector of the same pointers.
func (v *Vector[T]) Clone() *Vector[T] {
	return &Vector[T]{es: slices.Clone(v.es)}
}
