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

// ---------------------------------------------------------------------------
// The case the first cut did not model: a span produced by a VIEW.
//
// A container can hand out a concrete span, because it owns the slice. A view
// holds an interface -- and a CONVERTING view's underlying elements have a
// different type from the ones it presents -- so it cannot reach a concrete
// slice without materialising, which the constraint forbids.
//
// So a view-produced span must hold the interface. The question is whether that
// costs an allocation to construct, and the answer turns on HOW direction is
// expressed: as a wrapper type (which re-boxes) or as a field (which does not).
// ---------------------------------------------------------------------------

type windowed[T any] interface {
	Len() int
	// walk yields the half-open window [i, j), in the given direction.
	walk(i, j int, rev bool) iter.Seq[T]
}

// ifaceSpan holds an interface plus bounds plus a direction FIELD. Reversal
// flips the flag; nothing is wrapped, so nothing is re-boxed.
type ifaceSpan[T any] struct {
	impl windowed[T]
	i, j int
	rev  bool
}

func (sp ifaceSpan[T]) Len() int { return sp.j - sp.i }

func (sp ifaceSpan[T]) Backward() ifaceSpan[T] {
	sp.rev = !sp.rev // a copy, with a flag flipped -- no allocation
	return sp
}

func (sp ifaceSpan[T]) Keys() iter.Seq[T] { return sp.impl.walk(sp.i, sp.j, sp.rev) }

// wrapSpan is the same thing expressing direction as a WRAPPER, which is what
// solution B does, to isolate where B's allocations actually come from.
type wrapSpan[T any] struct {
	impl windowed[T]
	i, j int
}

func (sp wrapSpan[T]) Backward() wrapSpan[T] {
	return wrapSpan[T]{impl: reverseWrapper[T]{sp.impl}, i: sp.i, j: sp.j}
}

func (sp wrapSpan[T]) Keys() iter.Seq[T] { return sp.impl.walk(sp.i, sp.j, false) }

type reverseWrapper[T any] struct{ inner windowed[T] }

func (r reverseWrapper[T]) Len() int { return r.inner.Len() }
func (r reverseWrapper[T]) walk(i, j int, rev bool) iter.Seq[T] {
	return r.inner.walk(i, j, !rev)
}

// sliceWindowed is what a container's own state looks like behind that
// interface.
type sliceWindowed[T any] struct{ es []T }

func (s sliceWindowed[T]) Len() int { return len(s.es) }
func (s sliceWindowed[T]) walk(i, j int, rev bool) iter.Seq[T] {
	if rev {
		return backward(s.es[i:j])
	}
	return forward(s.es[i:j])
}

func (s sortedSet[T]) D_IfaceSpan(lo, hi T) ifaceSpan[T] {
	i, _ := slices.BinarySearch(s.st.es, lo)
	j, _ := slices.BinarySearch(s.st.es, hi)
	if j < i {
		j = i
	}
	return ifaceSpan[T]{impl: sliceWindowed[T]{s.st.es}, i: i, j: j}
}

func (s sortedSet[T]) D_WrapSpan(lo, hi T) wrapSpan[T] {
	i, _ := slices.BinarySearch(s.st.es, lo)
	j, _ := slices.BinarySearch(s.st.es, hi)
	if j < i {
		j = i
	}
	return wrapSpan[T]{impl: sliceWindowed[T]{s.st.es}, i: i, j: j}
}

// Is the allocation inherent to holding an interface, or is it the BOXING of a
// wide value into one? sliceWindowed is 3 words (a slice header), so boxing it
// allocates. A view has already paid that once, at view construction -- so a
// span produced by a VIEW copies an existing interface value rather than
// boxing a new one.

type ptrWindowed[T ~int] struct{ st *state[T] } // one word: pointer-shaped

func (p ptrWindowed[T]) Len() int { return len(p.st.es) }
func (p ptrWindowed[T]) walk(i, j int, rev bool) iter.Seq[T] {
	if rev {
		return backward(p.st.es[i:j])
	}
	return forward(p.st.es[i:j])
}

// view models what ADR 0018 ships: a struct holding an already-boxed interface.
type view[T ~int] struct{ impl windowed[T] }

func (s sortedSet[T]) E_View() view[T] { return view[T]{impl: ptrWindowed[T]{s.st}} }

// Range on a VIEW: copies the interface it already holds. No boxing.
func (v view[T]) E_Range(i, j int) ifaceSpan[T] {
	return ifaceSpan[T]{impl: v.impl, i: i, j: j}
}

// And the same on a container, boxing a POINTER-shaped impl rather than a wide
// one.
func (s sortedSet[T]) E_RangePtr(lo, hi T) ifaceSpan[T] {
	i, _ := slices.BinarySearch(s.st.es, lo)
	j, _ := slices.BinarySearch(s.st.es, hi)
	if j < i {
		j = i
	}
	return ifaceSpan[T]{impl: ptrWindowed[T]{s.st}, i: i, j: j}
}

// Len on a key-bounded span. ADR 0017 established that a sub-range must hold
// KEY bounds, not indices: index bounds are 14x dearer to construct and go
// silently wrong when the container changes. The consequence is that Len is not
// a subtraction -- it is two binary searches.

type keyBoundedSpan[T ~int] struct {
	st     *state[T]
	lo, hi T
	rev    bool
}

func (sp keyBoundedSpan[T]) bounds() (int, int) {
	i, _ := slices.BinarySearch(sp.st.es, sp.lo)
	j, _ := slices.BinarySearch(sp.st.es, sp.hi)
	if j < i {
		j = i
	}
	return i, j
}

func (sp keyBoundedSpan[T]) Len() int { i, j := sp.bounds(); return j - i }

func (sp keyBoundedSpan[T]) Keys() iter.Seq[T] {
	i, j := sp.bounds()
	if sp.rev {
		return backward(sp.st.es[i:j])
	}
	return forward(sp.st.es[i:j])
}

func (s sortedSet[T]) F_KeyBounded(lo, hi T) keyBoundedSpan[T] {
	return keyBoundedSpan[T]{st: s.st, lo: lo, hi: hi}
}

// indexBoundedSpan resolves once, so Len is a subtraction -- and the window
// goes stale the moment the container changes.
type indexBoundedSpan[T ~int] struct {
	st   *state[T]
	i, j int
}

func (sp indexBoundedSpan[T]) Len() int { return sp.j - sp.i }

func (s sortedSet[T]) F_IndexBounded(lo, hi T) indexBoundedSpan[T] {
	i, _ := slices.BinarySearch(s.st.es, lo)
	j, _ := slices.BinarySearch(s.st.es, hi)
	if j < i {
		j = i
	}
	return indexBoundedSpan[T]{st: s.st, i: i, j: j}
}
