package reverse

import (
	"iter"
	"slices"
)

// A sorted set, slice-backed, in the shape ADR 0018 left containers: a one-word
// reference value.
type sortedSet[T ~int] struct{ st *state[T] }

type state[T ~int] struct{ es []T }

func newSortedSet[T ~int](es []T) sortedSet[T] {
	c := slices.Clone(es)
	slices.Sort(c)
	return sortedSet[T]{st: &state[T]{es: c}}
}

func (s sortedSet[T]) window(lo, hi T) []T {
	i, _ := slices.BinarySearch(s.st.es, lo)
	j, _ := slices.BinarySearch(s.st.es, hi)
	if j < i {
		j = i
	}
	return s.st.es[i:j]
}

func forward[T any](es []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, e := range es {
			if !yield(e) {
				return
			}
		}
	}
}

func backward[T any](es []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for i := len(es) - 1; i >= 0; i-- {
			if !yield(es[i]) {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// A -- method pairs. Every read gets a Backward twin, returning a bare iter.Seq.
// ---------------------------------------------------------------------------

func (s sortedSet[T]) A_Keys() iter.Seq[T]         { return forward(s.st.es) }
func (s sortedSet[T]) A_KeysBackward() iter.Seq[T] { return backward(s.st.es) }

func (s sortedSet[T]) A_RangeKeys(lo, hi T) iter.Seq[T] { return forward(s.window(lo, hi)) }
func (s sortedSet[T]) A_RangeKeysBackward(lo, hi T) iter.Seq[T] {
	return backward(s.window(lo, hi))
}

// ---------------------------------------------------------------------------
// B -- a sub-range IS a view, and views gain Backward(). One concept covers
// bounds and direction, at the cost of the view's interface dispatch.
//
// This is the shape ADR 0013 deferred ("should Range return a view?") and
// ADR 0017 deferred again.
// ---------------------------------------------------------------------------

type keysImpl[T any] interface {
	Len() int
	Keys() iter.Seq[T]
}

type setView[T any] struct{ impl keysImpl[T] }

func (v setView[T]) Len() int          { return v.impl.Len() }
func (v setView[T]) Keys() iter.Seq[T] { return v.impl.Keys() }

// Backward returns a view of the same window in the opposite direction. O(1):
// it swaps a flag, it does not materialise anything.
func (v setView[T]) Backward() setView[T] { return setView[T]{impl: reversed[T]{v.impl}} }

type windowImpl[T any] struct {
	es  []T
	rev bool
}

func (w windowImpl[T]) Len() int { return len(w.es) }
func (w windowImpl[T]) Keys() iter.Seq[T] {
	if w.rev {
		return backward(w.es)
	}
	return forward(w.es)
}

type reversed[T any] struct{ inner keysImpl[T] }

func (r reversed[T]) Len() int { return r.inner.Len() }
func (r reversed[T]) Keys() iter.Seq[T] {
	// The inner window knows how to walk backwards; this only flips the flag.
	if w, ok := r.inner.(windowImpl[T]); ok {
		return windowImpl[T]{es: w.es, rev: !w.rev}.Keys()
	}
	panic("unreachable in this harness")
}

func (s sortedSet[T]) B_View() setView[T] {
	return setView[T]{impl: windowImpl[T]{es: s.st.es}}
}

func (s sortedSet[T]) B_Range(lo, hi T) setView[T] {
	return setView[T]{impl: windowImpl[T]{es: s.window(lo, hi)}}
}

// ---------------------------------------------------------------------------
// C -- a concrete Span value. Composes like B, but holds the window directly
// rather than behind an interface, so there is no dispatch.
// ---------------------------------------------------------------------------

type span[T any] struct {
	es  []T
	rev bool
}

func (sp span[T]) Len() int          { return len(sp.es) }
func (sp span[T]) Backward() span[T] { return span[T]{es: sp.es, rev: !sp.rev} }

func (sp span[T]) Keys() iter.Seq[T] {
	if sp.rev {
		return backward(sp.es)
	}
	return forward(sp.es)
}

func (s sortedSet[T]) C_Span() span[T]          { return span[T]{es: s.st.es} }
func (s sortedSet[T]) C_Range(lo, hi T) span[T] { return span[T]{es: s.window(lo, hi)} }

// The thing that must NOT ship: materialising to reverse.
func (s sortedSet[T]) Naive_Backward() iter.Seq[T] {
	c := slices.Clone(s.st.es)
	slices.Reverse(c)
	return forward(c)
}

func (s sortedSet[T]) Naive_RangeBackward(lo, hi T) iter.Seq[T] {
	c := slices.Clone(s.window(lo, hi))
	slices.Reverse(c)
	return forward(c)
}
