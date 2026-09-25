package sealing

import "iter"

type Item struct{ Name string }

type MapSet[T comparable] struct{ m map[T]struct{} }

func NewMapSet[T comparable](vs ...T) MapSet[T] {
	s := MapSet[T]{m: make(map[T]struct{}, len(vs))}
	for _, v := range vs {
		s.m[v] = struct{}{}
	}
	return s
}
func (s MapSet[T]) Len() int     { return len(s.m) }
func (s MapSet[T]) Has(v T) bool { _, ok := s.m[v]; return ok }

// Add is the mutator the seal exists to keep out of a read-only handle.
func (s MapSet[T]) Add(v T) { s.m[v] = struct{}{} }

// Keys exists so that MapSet[string] matches IfaceView[string] on EVERY method
// except the seal. Without it the impossible-assertion check below would pass
// for the wrong reason, and would keep passing if the seal were removed.
func (s MapSet[T]) Keys() iter.Seq[T] {
	m := s.m
	return func(yield func(T) bool) {
		for t := range m {
			if !yield(t) {
				return
			}
		}
	}
}

type viewer interface{ ToKeyView(*Item) string }
type stateful struct{ known map[string]*Item }

func (v stateful) ToKeyView(i *Item) string { return i.Name }

// The sealed shape interface every read-only API takes.
type ReadKeys[NT any] interface {
	Len() int
	Keys() iter.Seq[NT]
	viewOnly()
}

// =====================================================================
// SHAPE 1 -- TODAY: a concrete struct wrapping an interface.
//
// The struct CANNOT name the container's T (MapSetView[NT] has no T), so it
// MUST hold an interface. That inner interface is structural, not incidental.
// =====================================================================

type inner[NT any] interface {
	Len() int
	Keys() iter.Seq[NT]
}

// the thing the inner interface erases: container + viewer
type convImpl struct {
	s  MapSet[*Item]
	vw viewer
}

func (c convImpl) Len() int { return c.s.Len() }
func (c convImpl) Keys() iter.Seq[string] {
	s, vw := c.s, c.vw
	return func(yield func(string) bool) {
		for t := range s.m {
			if !yield(vw.ToKeyView(t)) {
				return
			}
		}
	}
}

type convImpl2 struct {
	s  MapSet[*Item]
	vw viewer
}

func (c convImpl2) Len() int               { return c.s.Len() + 0 }
func (c convImpl2) Keys() iter.Seq[string] { return convImpl(c).Keys() }

var useSecond = false

func mkInner(s MapSet[*Item], vw viewer) inner[string] {
	if useSecond {
		return convImpl2{s, vw}
	}
	return convImpl{s, vw}
}

type StructView[NT any] struct{ impl inner[NT] }

func (v StructView[NT]) Len() int {
	if v.impl == nil {
		return 0
	}
	return v.impl.Len()
}
func (v StructView[NT]) Keys() iter.Seq[NT] {
	if v.impl == nil {
		return func(func(NT) bool) {}
	}
	return v.impl.Keys()
}
func (v StructView[NT]) viewOnly() {}

func NewStructView(s MapSet[*Item], vw viewer) StructView[string] {
	return StructView[string]{impl: mkInner(s, vw)}
}

// =====================================================================
// SHAPE 2 -- a bare SEALED INTERFACE. The interface IS the erasure point, so
// there is no inner interface: the implementation holds the container directly.
// =====================================================================

type IfaceView[NT any] interface {
	Len() int
	Keys() iter.Seq[NT]
	viewOnly()
}

// same fields as convImpl, plus the seal -- ONE layer, not two
type ifaceConv struct {
	s  MapSet[*Item]
	vw viewer
}

func (c ifaceConv) Len() int { return c.s.Len() }
func (c ifaceConv) Keys() iter.Seq[string] {
	s, vw := c.s, c.vw
	return func(yield func(string) bool) {
		for t := range s.m {
			if !yield(vw.ToKeyView(t)) {
				return
			}
		}
	}
}
func (c ifaceConv) viewOnly() {}

type ifaceConv2 struct {
	s  MapSet[*Item]
	vw viewer
}

func (c ifaceConv2) Len() int               { return c.s.Len() + 0 }
func (c ifaceConv2) Keys() iter.Seq[string] { return ifaceConv(c).Keys() }
func (c ifaceConv2) viewOnly()              {}

func NewIfaceView(s MapSet[*Item], vw viewer) IfaceView[string] {
	if useSecond {
		return ifaceConv2{s, vw}
	}
	return ifaceConv{s, vw}
}

// =====================================================================
// SHAPE 3 -- a ONE-WORD concrete struct. It must still hold an INTERFACE to
// erase the container's type parameter, and an interface is two words, so the
// state has to be heap-allocated: one word costs an EXTRA allocation at
// construction. (An earlier version of this file hardcoded the container type
// in the state, which removed the erasure and flattered the shape badly.)
// =====================================================================

type ptrState[NT any] struct{ impl inner[NT] }

type PtrView[NT any] struct{ st *ptrState[NT] }

func (v PtrView[NT]) Len() int {
	if v.st == nil {
		return 0
	}
	return v.st.impl.Len()
}

func (v PtrView[NT]) Keys() iter.Seq[NT] {
	if v.st == nil {
		return func(func(NT) bool) {}
	}
	return v.st.impl.Keys()
}
func (v PtrView[NT]) viewOnly() {}

func NewPtrView(s MapSet[*Item], vw viewer) PtrView[string] {
	return PtrView[string]{st: &ptrState[string]{impl: mkInner(s, vw)}}
}

// An IDENTITY impl is ONE WORD (it holds only the container), and a one-word
// value boxes into an interface for free. So View() can return the SAME
// concrete struct view type at zero cost -- no separate IdentityView type.

type identImpl[T comparable] struct{ s MapSet[T] }

func (v identImpl[T]) Len() int { return v.s.Len() }
func (v identImpl[T]) Keys() iter.Seq[T] {
	m := v.s.m
	return func(yield func(T) bool) {
		for t := range m {
			if !yield(t) {
				return
			}
		}
	}
}

type identImpl2[T comparable] struct{ s MapSet[T] }

func (v identImpl2[T]) Len() int          { return v.s.Len() + 0 }
func (v identImpl2[T]) Keys() iter.Seq[T] { return identImpl[T](v).Keys() }

// View returns the ordinary struct view -- uniform with converting views.
func (s MapSet[T]) View() StructView[T] {
	if useSecond {
		return StructView[T]{impl: identImpl2[T]{s}}
	}
	return StructView[T]{impl: identImpl[T]{s}}
}
