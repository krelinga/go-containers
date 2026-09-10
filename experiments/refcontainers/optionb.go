package refcontainers

import "iter"

// A complete option (b), built to match builtin map semantics on the zero
// value: reads are total, writes panic, and every container and view spells
// emptiness the same way.

type obState[T comparable] struct{ m map[T]struct{} }

// obSet has VALUE receivers throughout, including the mutators. That is what
// makes a zero value read-only rather than lazily constructible -- see
// TestLazyInitDiverges for why the alternative is worse.
type obSet[T comparable] struct{ st *obState[T] }

func newOBSet[T comparable](vs ...T) obSet[T] {
	st := &obState[T]{m: make(map[T]struct{}, len(vs))}
	for _, v := range vs {
		st.m[v] = struct{}{}
	}
	return obSet[T]{st}
}

func (s obSet[T]) IsZero() bool { return s.st == nil }

// Reads: total on a zero value, exactly as a nil map is.
func (s obSet[T]) Len() int {
	if s.st == nil {
		return 0
	}
	return len(s.st.m)
}

func (s obSet[T]) Has(v T) bool {
	if s.st == nil {
		return false
	}
	_, ok := s.st.m[v]
	return ok
}

func (s obSet[T]) Keys() iter.Seq[T] {
	st := s.st
	return func(yield func(T) bool) {
		if st == nil {
			return
		}
		for v := range st.m {
			if !yield(v) {
				return
			}
		}
	}
}

func (s obSet[T]) KeySlice() []T {
	if s.st == nil {
		return nil
	}
	out := make([]T, 0, len(s.st.m))
	for v := range s.st.m {
		out = append(out, v)
	}
	return out
}

// Writes: panic on a zero value, exactly as writing to a nil map does.
func (s obSet[T]) Add(v T) { s.st.m[v] = struct{}{} }

// lazySet is the rejected alternative: mixed receivers, so a write can lazily
// construct the state and ADR 0002's usable zero value survives. It
// reintroduces the divergence that reference semantics exists to remove.
type lazySet[T comparable] struct{ st *obState[T] }

func (s lazySet[T]) Len() int {
	if s.st == nil {
		return 0
	}
	return len(s.st.m)
}

func (s *lazySet[T]) Add(v T) {
	if s.st == nil {
		s.st = &obState[T]{m: make(map[T]struct{})}
	}
	s.st.m[v] = struct{}{}
}

// ---------------------------------------------------------------------------
// The hierarchy, recovered by struct embedding rather than a conversion method.
// ---------------------------------------------------------------------------

type obSetView[T any] struct{ impl setViewImpl[T] }

func (v obSetView[T]) IsZero() bool { return v.impl == nil }

func (v obSetView[T]) Len() int {
	if v.impl == nil {
		return 0
	}
	return v.impl.Len()
}

func (v obSetView[T]) Has(t T) bool {
	if v.impl == nil {
		return false
	}
	return v.impl.Has(t)
}

func (v obSetView[T]) Keys() iter.Seq[T] {
	impl := v.impl
	return func(yield func(T) bool) {
		if impl == nil {
			return
		}
		impl.Keys()(yield)
	}
}

func (v obSetView[T]) KeySlice() []T {
	if v.impl == nil {
		return nil
	}
	return v.impl.KeySlice()
}

// obSortedSetView EMBEDS the base view. Method promotion gives it every base
// method for free, and the "conversion" to the base is a field selector rather
// than a method call: sv.SetView.
type obSortedSetView[T any] struct {
	SetView obSetView[T]
}

func viewOBSorted[T comparable](s *ptrSet[T]) obSortedSetView[T] {
	return obSortedSetView[T]{SetView: obSetView[T]{ifaceSetViewImpl[T]{s}}}
}

func (v obSortedSetView[T]) Len() int     { return v.SetView.Len() }
func (v obSortedSetView[T]) Has(t T) bool { return v.SetView.Has(t) }
func (v obSortedSetView[T]) Min() (T, bool) {
	var z T
	return z, false
}
