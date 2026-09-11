package containers

import (
	"iter"
	"slices"
)

// Vector is an insertion-ordered sequence backed by a slice.
//
// It is a reference type (ADR 0018): a one-word value whose copies share the
// same contents. The backing slice sits behind a pointer precisely so that a
// reallocation is invisible to callers -- which is the whole reason Vector
// exists rather than a plain []T (ADR 0015).
//
// The zero value reads as empty and panics on a write. Appending to a nil
// slice works, which looks like a counterexample until you notice that append
// RETURNS rather than mutates: the builtin counterpart to v.Append(e) is
// m[k] = v, which panics.
//
// Vector is not a replacement for []T. It earns its place at an API boundary,
// where a []T field cannot be handed out read-only; it is worse than a slice
// for local code (ADR 0015).
type Vector[T any] struct {
	st *vectorState[T]
}

type vectorState[T any] struct{ es []T }

// NewVector returns a Vector containing vs. vs is copied and not retained.
func NewVector[T any](vs ...T) Vector[T] {
	return Vector[T]{st: &vectorState[T]{es: slices.Clone(vs)}}
}

// IsZero reports whether v was ever constructed.
func (v Vector[T]) IsZero() bool { return v.st == nil }

// Len reports how many elements the Vector holds.
func (v Vector[T]) Len() int {
	if v.st == nil {
		return 0
	}
	return len(v.st.es)
}

// At returns the element at i. It panics if i is out of range, exactly as
// indexing a slice does -- including on a zero Vector, where every index is out
// of range.
func (v Vector[T]) At(i int) T {
	if v.st == nil {
		panic("containers: Vector index out of range on a zero Vector")
	}
	return v.st.es[i]
}

// Set replaces the element at i. It panics if i is out of range.
func (v Vector[T]) Set(i int, e T) {
	if v.st == nil {
		panic("containers: Vector index out of range on a zero Vector")
	}
	v.st.es[i] = e
}

// Append adds one element to the end. Appending to a zero Vector panics.
//
// It is deliberately not variadic: a variadic call costs a fixed ~0.3-0.8ns and
// no allocation, which is ~60% of an append (ADRs 0015, 0017). AppendAll covers
// bulk.
func (v Vector[T]) Append(e T) { v.st.es = append(v.st.es, e) }

// AppendAll adds every element of vs, in order. vs is copied and not retained.
func (v Vector[T]) AppendAll(vs ...T) {
	if len(vs) == 0 {
		return // writes nothing, so it does not panic
	}
	es := slices.Grow(v.st.es, len(vs))
	v.st.es = append(es, vs...)
}

// Values iterates the elements in order.
//
// The slice header is captured at the call, not at iteration: a later Append is
// not seen, a later Set(i, x) within range IS. See SortedSet.Keys for the full
// rule.
func (v Vector[T]) Values() iter.Seq[T] {
	var es []T
	if v.st != nil {
		es = v.st.es
	}
	return slices.Values(es)
}

// Positions iterates the valid indices in order.
//
// Redundant for a Vector, where it is just 0..Len()-1, and not redundant for
// generic code over PositionValues[P, V] where P may be a cursor (ADR 0018).
func (v Vector[T]) Positions() iter.Seq[int] {
	var es []T
	if v.st != nil {
		es = v.st.es
	}
	return func(yield func(int) bool) {
		for i := range es {
			if !yield(i) {
				return
			}
		}
	}
}

// All iterates index and element together. A Vector is keyed by position, so
// All yields pairs and Values yields the elements alone (ADR 0017).
func (v Vector[T]) All() iter.Seq2[int, T] {
	var es []T
	if v.st != nil {
		es = v.st.es
	}
	return slices.All(es)
}

// ValueSlice returns the elements as a new slice, in order. The result is a
// full, independent copy (ADR 0017).
func (v Vector[T]) ValueSlice() []T {
	if v.st == nil {
		return nil
	}
	return slices.Clone(v.st.es)
}

// PositionSlice returns the valid indices as a new slice.
func (v Vector[T]) PositionSlice() []int {
	if v.st == nil {
		return nil
	}
	out := make([]int, len(v.st.es))
	for i := range out {
		out[i] = i
	}
	return out
}

// AllSlice returns index/element pairs as a new slice, in order. It is the
// bulk-transfer shape for feeding a map: NewSortedMap(v.AllSlice()...).
func (v Vector[T]) AllSlice() []Entry[int, T] {
	if v.st == nil {
		return nil
	}
	out := make([]Entry[int, T], 0, len(v.st.es))
	for i, e := range v.st.es {
		out = append(out, Entry[int, T]{i, e})
	}
	return out
}

// Clone returns an independent copy.
func (v Vector[T]) Clone() Vector[T] {
	if v.st == nil {
		return Vector[T]{}
	}
	return Vector[T]{st: &vectorState[T]{es: slices.Clone(v.st.es)}}
}
