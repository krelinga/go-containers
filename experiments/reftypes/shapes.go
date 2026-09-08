package reftypes

import "iter"

// The shared backing, identical for every representation so the comparison is
// about the handle and nothing else.
type setState[T comparable] struct{ m map[T]struct{} }

func newState[T comparable]() *setState[T] { return &setState[T]{m: map[T]struct{}{}} }

// ---------------------------------------------------------------------------
// (1) Today's shape: pointer to a non-copyable struct. ADR 0002.
// ---------------------------------------------------------------------------

// noCopy is vet's copylocks marker, reproduced so the shape is faithful.
type noCopy struct{}

func (noCopy) Lock()   {}
func (noCopy) Unlock() {}

type PtrSet[T comparable] struct {
	noCopy
	m map[T]struct{}
}

func NewPtrSet[T comparable]() *PtrSet[T] { return &PtrSet[T]{m: map[T]struct{}{}} }

func (s *PtrSet[T]) Len() int     { return len(s.m) }
func (s *PtrSet[T]) Has(t T) bool { _, ok := s.m[t]; return ok }
func (s *PtrSet[T]) Add(t T)      { s.m[t] = struct{}{} }
func (s *PtrSet[T]) All() iter.Seq[T] {
	m := s.m // eager, per ADR 0002
	return func(yield func(T) bool) {
		for t := range m {
			if !yield(t) {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// (2) Reference struct, no nil handling. Panics on a zero value like today.
// ---------------------------------------------------------------------------

type RefSet[T comparable] struct{ st *setState[T] }

func NewRefSet[T comparable]() RefSet[T] { return RefSet[T]{newState[T]()} }

func (s RefSet[T]) Len() int     { return len(s.st.m) }
func (s RefSet[T]) Has(t T) bool { _, ok := s.st.m[t]; return ok }
func (s RefSet[T]) Add(t T)      { s.st.m[t] = struct{}{} }
func (s RefSet[T]) All() iter.Seq[T] {
	m := s.st.m
	return func(yield func(T) bool) {
		for t := range m {
			if !yield(t) {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// (3) Reference struct reproducing builtin-map semantics: a zero value reads
// clean and panics on write. Every read carries the nil check ADR 0002 bans.
// ---------------------------------------------------------------------------

type NilSafeSet[T comparable] struct{ st *setState[T] }

func NewNilSafeSet[T comparable]() NilSafeSet[T] { return NilSafeSet[T]{newState[T]()} }

func (s NilSafeSet[T]) IsZero() bool { return s.st == nil }

func (s NilSafeSet[T]) Len() int {
	if s.st == nil {
		return 0
	}
	return len(s.st.m)
}
func (s NilSafeSet[T]) Has(t T) bool {
	if s.st == nil {
		return false
	}
	_, ok := s.st.m[t]
	return ok
}

// Add does NOT check: writing to a zero container panics, as it does for a
// builtin map.
func (s NilSafeSet[T]) Add(t T) { s.st.m[t] = struct{}{} }

func (s NilSafeSet[T]) All() iter.Seq[T] {
	if s.st == nil {
		return func(yield func(T) bool) {}
	}
	m := s.st.m
	return func(yield func(T) bool) {
		for t := range m {
			if !yield(t) {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// (4) Sealed interface. Nil-comparable, and set algebra type-checks because the
// container is the interface -- the ceiling ADR 0002 recorded.
// ---------------------------------------------------------------------------

type IfaceSet[T comparable] interface {
	Len() int
	Has(T) bool
	Add(T)
	All() iter.Seq[T]
	Union(IfaceSet[T]) IfaceSet[T]
	sealedContainer()
}

type ifaceSet[T comparable] struct{ st *setState[T] }

func NewIfaceSet[T comparable]() IfaceSet[T] { return ifaceSet[T]{newState[T]()} }

func (s ifaceSet[T]) sealedContainer() {}
func (s ifaceSet[T]) Len() int         { return len(s.st.m) }
func (s ifaceSet[T]) Has(t T) bool     { _, ok := s.st.m[t]; return ok }
func (s ifaceSet[T]) Add(t T)          { s.st.m[t] = struct{}{} }
func (s ifaceSet[T]) All() iter.Seq[T] {
	m := s.st.m
	return func(yield func(T) bool) {
		for t := range m {
			if !yield(t) {
				return
			}
		}
	}
}

// Union is the point: a method returning the container type satisfies an
// interface method returning the interface, because they are the same type.
// Under ADR 0002's shape, *HashSet.Union cannot satisfy Set.Union.
func (s ifaceSet[T]) Union(o IfaceSet[T]) IfaceSet[T] {
	out := NewIfaceSet[T]()
	for t := range s.All() {
		out.Add(t)
	}
	for t := range o.All() {
		out.Add(t)
	}
	return out
}

// ---------------------------------------------------------------------------
// A slice-backed pair, to show what reference semantics fixes. A slice header
// must be REPLACED when it grows, which is why the indirection is not optional.
// ---------------------------------------------------------------------------

type listState[T any] struct{ es []T }

// ValueList copies its header, so appends to a copy diverge -- the aliasing
// surprise this repository was started over.
type ValueList[T any] struct{ es []T }

func (l *ValueList[T]) Append(v T) { l.es = append(l.es, v) }
func (l ValueList[T]) Len() int    { return len(l.es) }

// RefList shares its state, so appends through a copy are visible in the
// original, like a map.
type RefList[T any] struct{ st *listState[T] }

func NewRefList[T any](cap int) RefList[T] {
	return RefList[T]{&listState[T]{es: make([]T, 0, cap)}}
}
func (l RefList[T]) Append(v T) { l.st.es = append(l.st.es, v) }
func (l RefList[T]) Len() int {
	if l.st == nil {
		return 0
	}
	return len(l.st.es)
}

// AllInner does the nil check INSIDE the closure rather than before it.
//
// The eager form has two return statements returning two different closures,
// which is enough to put the closure on the heap. Moving the check inside gives
// one closure and one return.
//
// It also inverts ADR 0002's eager-dereference rule, which exists so a nil
// container panics at the call rather than at iteration. Under builtin-map
// semantics there is nothing to panic about: ranging a nil map yields nothing,
// so a read is total and the rule has no work to do. It still applies to writes.
func (s NilSafeSet[T]) AllInner() iter.Seq[T] {
	return func(yield func(T) bool) {
		if s.st == nil {
			return
		}
		for t := range s.st.m {
			if !yield(t) {
				return
			}
		}
	}
}
