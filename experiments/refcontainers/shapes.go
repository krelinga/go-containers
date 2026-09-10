package refcontainers

import (
	"iter"
	"slices"
)

// ---------------------------------------------------------------------------
// Container shapes. Both carry today's method set -- ADR 0017 added KeySlice,
// which ADR 0014's toy set did not have, and which is now on the hot path for
// every bulk operation.
// ---------------------------------------------------------------------------

// ptrSet is ADR 0002's shape: a non-copyable struct held as a pointer.
type ptrSet[T comparable] struct {
	_ noCopy
	m map[T]struct{}
}

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

func newPtrSet[T comparable](vs ...T) *ptrSet[T] {
	s := &ptrSet[T]{m: make(map[T]struct{}, len(vs))}
	for _, v := range vs {
		s.m[v] = struct{}{}
	}
	return s
}

func (s *ptrSet[T]) Has(v T) bool { _, ok := s.m[v]; return ok }
func (s *ptrSet[T]) Len() int     { return len(s.m) }
func (s *ptrSet[T]) Add(v T)      { s.m[v] = struct{}{} }

func (s *ptrSet[T]) Keys() iter.Seq[T] {
	m := s.m
	return func(yield func(T) bool) {
		for v := range m {
			if !yield(v) {
				return
			}
		}
	}
}

func (s *ptrSet[T]) KeySlice() []T {
	out := make([]T, 0, len(s.m))
	for v := range s.m {
		out = append(out, v)
	}
	return out
}

// refSet is the reference shape: a one-word struct with value receivers.
// Copies share. There is nothing for copylocks to catch.
type refSet[T comparable] struct{ st *refSetState[T] }

type refSetState[T comparable] struct{ m map[T]struct{} }

func newRefSet[T comparable](vs ...T) refSet[T] {
	st := &refSetState[T]{m: make(map[T]struct{}, len(vs))}
	for _, v := range vs {
		st.m[v] = struct{}{}
	}
	return refSet[T]{st}
}

func (s refSet[T]) Has(v T) bool { _, ok := s.st.m[v]; return ok }
func (s refSet[T]) Len() int     { return len(s.st.m) }
func (s refSet[T]) Add(v T)      { s.st.m[v] = struct{}{} }

func (s refSet[T]) Keys() iter.Seq[T] {
	m := s.st.m
	return func(yield func(T) bool) {
		for v := range m {
			if !yield(v) {
				return
			}
		}
	}
}

func (s refSet[T]) KeySlice() []T {
	out := make([]T, 0, len(s.st.m))
	for v := range s.st.m {
		out = append(out, v)
	}
	return out
}

// nilRefSet adds the builtin-map nil semantics ADR 0014 decision 3 proposed:
// reads are total on a zero value, writes panic. The check sits INSIDE the
// closure for Keys, which ADR 0014 found is what keeps it off the heap.
type nilRefSet[T comparable] struct{ st *refSetState[T] }

func newNilRefSet[T comparable](vs ...T) nilRefSet[T] {
	st := &refSetState[T]{m: make(map[T]struct{}, len(vs))}
	for _, v := range vs {
		st.m[v] = struct{}{}
	}
	return nilRefSet[T]{st}
}

func (s nilRefSet[T]) Has(v T) bool {
	if s.st == nil {
		return false
	}
	_, ok := s.st.m[v]
	return ok
}

func (s nilRefSet[T]) Len() int {
	if s.st == nil {
		return 0
	}
	return len(s.st.m)
}

func (s nilRefSet[T]) Keys() iter.Seq[T] {
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

func (s nilRefSet[T]) KeySlice() []T {
	if s.st == nil {
		return nil
	}
	out := make([]T, 0, len(s.st.m))
	for v := range s.st.m {
		out = append(out, v)
	}
	return out
}

// ---------------------------------------------------------------------------
// View shapes. The question ADR 0014 could not answer favourably, because
// struct views were charged for boxing into Elems2. That path no longer exists.
// ---------------------------------------------------------------------------

// ifaceSetView is today's shape: a sealed interface (ADR 0013).
type ifaceSetView[T any] interface {
	Len() int
	Has(T) bool
	Keys() iter.Seq[T]
	KeySlice() []T
	sealedView()
}

type ifaceSetViewImpl[T comparable] struct{ s *ptrSet[T] }

func (v ifaceSetViewImpl[T]) sealedView()       {}
func (v ifaceSetViewImpl[T]) Len() int          { return v.s.Len() }
func (v ifaceSetViewImpl[T]) Has(t T) bool      { return v.s.Has(t) }
func (v ifaceSetViewImpl[T]) Keys() iter.Seq[T] { return v.s.Keys() }
func (v ifaceSetViewImpl[T]) KeySlice() []T     { return v.s.KeySlice() }

func viewIface[T comparable](s *ptrSet[T]) ifaceSetView[T] { return ifaceSetViewImpl[T]{s} }

// structSetView is the alternative: a one-word struct with concrete methods.
// It cannot be built populated outside its package, so it needs no seal.
type structSetView[T comparable] struct{ s *ptrSet[T] }

func viewStruct[T comparable](s *ptrSet[T]) structSetView[T] { return structSetView[T]{s} }

func (v structSetView[T]) Len() int          { return v.s.Len() }
func (v structSetView[T]) Has(t T) bool      { return v.s.Has(t) }
func (v structSetView[T]) Keys() iter.Seq[T] { return v.s.Keys() }
func (v structSetView[T]) KeySlice() []T     { return v.s.KeySlice() }

// ---------------------------------------------------------------------------
// Substitution: an ordered view used where the base is wanted.
//
// With interfaces this is embedding, and free. With structs it is an explicit
// conversion, because struct embedding is not subtyping -- ADR 0013 decision 2
// is what is at stake.
// ---------------------------------------------------------------------------

type ifaceSortedSetView[T comparable] interface {
	ifaceSetView[T]
	Min() (T, bool)
}

type ifaceSortedImpl[T comparable] struct{ s *ptrSet[T] }

func (v ifaceSortedImpl[T]) sealedView()       {}
func (v ifaceSortedImpl[T]) Len() int          { return v.s.Len() }
func (v ifaceSortedImpl[T]) Has(t T) bool      { return v.s.Has(t) }
func (v ifaceSortedImpl[T]) Keys() iter.Seq[T] { return v.s.Keys() }
func (v ifaceSortedImpl[T]) KeySlice() []T     { return v.s.KeySlice() }
func (v ifaceSortedImpl[T]) Min() (T, bool)    { var z T; return z, false }

func viewIfaceSorted[T comparable](s *ptrSet[T]) ifaceSortedSetView[T] {
	return ifaceSortedImpl[T]{s}
}

type structSortedSetView[T comparable] struct{ s *ptrSet[T] }

func viewStructSorted[T comparable](s *ptrSet[T]) structSortedSetView[T] {
	return structSortedSetView[T]{s}
}

func (v structSortedSetView[T]) Len() int          { return v.s.Len() }
func (v structSortedSetView[T]) Has(t T) bool      { return v.s.Has(t) }
func (v structSortedSetView[T]) Keys() iter.Seq[T] { return v.s.Keys() }
func (v structSortedSetView[T]) KeySlice() []T     { return v.s.KeySlice() }
func (v structSortedSetView[T]) Min() (T, bool)    { var z T; return z, false }

// Set is the explicit conversion a caller must write at every ordered-to-base
// site if views become structs.
func (v structSortedSetView[T]) Set() structSetView[T] { return structSetView[T]{v.s} }

func fixture(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	return out
}

var _ = slices.Clone[[]string]

// ---------------------------------------------------------------------------
// The struct view the library would actually need.
//
// structSetView above wraps a concrete *ptrSet, so it is monomorphic and
// inlines -- but SetView[T] must view a HashSet, a SortedSet, or a converting
// view, so a polymorphic struct view has to wrap an INTERFACE. Measuring the
// monomorphic form would flatter the design; this is the honest shape.
// ---------------------------------------------------------------------------

type setViewImpl[T any] interface {
	Len() int
	Has(T) bool
	Keys() iter.Seq[T]
	KeySlice() []T
}

type wrapSetView[T any] struct{ impl setViewImpl[T] }

func viewWrap[T comparable](s *ptrSet[T]) wrapSetView[T] {
	return wrapSetView[T]{ifaceSetViewImpl[T]{s}}
}

func (v wrapSetView[T]) Len() int          { return v.impl.Len() }
func (v wrapSetView[T]) Has(t T) bool      { return v.impl.Has(t) }
func (v wrapSetView[T]) Keys() iter.Seq[T] { return v.impl.Keys() }
func (v wrapSetView[T]) KeySlice() []T     { return v.impl.KeySlice() }
func (v wrapSetView[T]) IsZero() bool      { return v.impl == nil }

type wrapSortedSetView[T any] struct{ impl setViewImpl[T] }

func viewWrapSorted[T comparable](s *ptrSet[T]) wrapSortedSetView[T] {
	return wrapSortedSetView[T]{ifaceSortedImpl[T]{s}}
}

func (v wrapSortedSetView[T]) Len() int { return v.impl.Len() }

// Set is the explicit conversion that replaces interface embedding.
func (v wrapSortedSetView[T]) Set() wrapSetView[T] { return wrapSetView[T]{v.impl} }
