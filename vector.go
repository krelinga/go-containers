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
// the operation again against a slice append. Use AppendAll for more than one.
// ADR 0017 re-measured this: a variadic call costs a fixed ~0.3-0.8ns and no
// allocation, which is 60% of an append and 5% of a map insert.
func (v *Vector[T]) Append(e T) { v.es = append(v.es, e) }

// AppendAll adds every element of vs to the end, in order. Spread a slice to
// bulk-append:
//
//	v.AppendAll(other.ValueSlice()...)
//
// vs is copied and not retained; the caller remains free to modify it.
func (v *Vector[T]) AppendAll(vs ...T) {
	// slices.Grow touches the receiver before the append, so a nil receiver
	// panics even when vs is empty -- ADR 0002's eager-dereference rule.
	es := slices.Grow(v.es, len(vs))
	v.es = append(es, vs...)
}

// Values iterates the elements in order.
//
// The iterator binds the backing slice at call time, so a nil receiver panics
// here rather than at iteration. See ADR 0002.
func (v *Vector[T]) Values() iter.Seq[T] {
	es := v.es
	return func(yield func(T) bool) {
		for _, e := range es {
			if !yield(e) {
				return
			}
		}
	}
}

// All iterates index and element together. A vector is keyed by position
// (ADR 0017), so All yields pairs and Values yields the elements alone.
// This is the shape ADR 0017's problem 1 was about: a type has one All, so the
// pair-shaped read gets it and the element-shaped read is Values, matching the
// stdlib's meaning of both names.
func (v *Vector[T]) All() iter.Seq2[int, T] {
	es := v.es
	return func(yield func(int, T) bool) {
		for i, e := range es {
			if !yield(i, e) {
				return
			}
		}
	}
}

// ValueSlice returns the elements as a new slice, in order.
//
// The result is a full, independent copy (ADR 0017): appending to the Vector
// afterwards is not visible through it, and writing to it is not visible in the
// Vector.
func (v *Vector[T]) ValueSlice() []T { return slices.Clone(v.es) }

// AllSlice returns index/element pairs as a new slice, in order. It is the
// bulk-transfer shape for feeding a dict: NewHashDict(v.AllSlice()...).
func (v *Vector[T]) AllSlice() []KeyValue[int, T] {
	out := make([]KeyValue[int, T], 0, len(v.es))
	for i, e := range v.es {
		out = append(out, KeyValue[int, T]{i, e})
	}
	return out
}

// Clone returns an independent Vector with the same elements. The backing
// slices are separate; the elements themselves are copied as values, so a
// Vector of pointers yields a Vector of the same pointers.
func (v *Vector[T]) Clone() *Vector[T] {
	return &Vector[T]{es: slices.Clone(v.es)}
}
