package refcontainers

import "iter"

// The two-tier view design: a concrete struct per container for the read-only
// GUARANTEE, plus capability interfaces for generic code that does not care
// whether it was handed a container or a view.
//
//	func audit(s Set[string])          // generic: container or view, reads only
//	func handOut() HashSetView[string] // guarantee: nothing can write through it
//
// The decisive question is what it costs to pass a concrete view into an
// interface parameter, because that is the boundary the design exists to make
// cheap. A one-word struct is pointer-shaped and boxes free; anything wider
// allocates.

// Set is the capability interface. Deliberately NOT sealed: containers satisfy
// it too, which is the whole point.
type Set[T any] interface {
	Len() int
	Has(T) bool
	Keys() iter.Seq[T]
	KeySlice() []T
}

// OrderedSet embeds Set, so the hierarchy survives in the interface layer and
// substitution stays implicit -- the thing struct embedding of views gave up.
type OrderedSet[T any] interface {
	Set[T]
	Min() (T, bool)
}

// --- One-word identity view: the common case, no viewer to carry ---

type identityView[T comparable] struct{ c *ptrSet[T] }

func viewIdentity[T comparable](c *ptrSet[T]) identityView[T] { return identityView[T]{c} }

func (v identityView[T]) Len() int {
	if v.c == nil {
		return 0
	}
	return v.c.Len()
}

func (v identityView[T]) Has(t T) bool {
	if v.c == nil {
		return false
	}
	return v.c.Has(t)
}

func (v identityView[T]) Keys() iter.Seq[T] {
	c := v.c
	return func(yield func(T) bool) {
		if c == nil {
			return
		}
		c.Keys()(yield)
	}
}

func (v identityView[T]) KeySlice() []T {
	if v.c == nil {
		return nil
	}
	return v.c.KeySlice()
}

func (v identityView[T]) IsZero() bool { return v.c == nil }

// --- Converting view, two shapes ---

type viewer[T, NT any] interface{ To(T) NT }

type upper struct{}

func (upper) To(s string) string { return s + "!" }

// wideView carries the viewer inline: pointer + interface = three words, so it
// is NOT pointer-shaped and boxes with an allocation.
type wideView[T comparable, NT any] struct {
	c  *ptrSet[T]
	vw viewer[T, NT]
}

func viewWide[T comparable, NT any](c *ptrSet[T], vw viewer[T, NT]) wideView[T, NT] {
	return wideView[T, NT]{c, vw}
}

func (v wideView[T, NT]) Len() int { return v.c.Len() }
func (v wideView[T, NT]) Has(nt NT) bool {
	return false
}

func (v wideView[T, NT]) Keys() iter.Seq[NT] {
	c, vw := v.c, v.vw
	return func(yield func(NT) bool) {
		if c == nil {
			return
		}
		for t := range c.Keys() {
			if !yield(vw.To(t)) {
				return
			}
		}
	}
}

func (v wideView[T, NT]) KeySlice() []NT {
	if v.c == nil {
		return nil
	}
	out := make([]NT, 0, v.c.Len())
	for t := range v.c.Keys() {
		out = append(out, v.vw.To(t))
	}
	return out
}

// narrowView pushes the viewer behind one pointer, so the view itself is one
// word and boxes free. The cost moves to construction.
type narrowState[T comparable, NT any] struct {
	c  *ptrSet[T]
	vw viewer[T, NT]
}

type narrowView[T comparable, NT any] struct{ st *narrowState[T, NT] }

func viewNarrow[T comparable, NT any](c *ptrSet[T], vw viewer[T, NT]) narrowView[T, NT] {
	return narrowView[T, NT]{&narrowState[T, NT]{c, vw}}
}

func (v narrowView[T, NT]) Len() int {
	if v.st == nil {
		return 0
	}
	return v.st.c.Len()
}

func (v narrowView[T, NT]) Has(nt NT) bool { return false }

func (v narrowView[T, NT]) Keys() iter.Seq[NT] {
	st := v.st
	return func(yield func(NT) bool) {
		if st == nil {
			return
		}
		for t := range st.c.Keys() {
			if !yield(st.vw.To(t)) {
				return
			}
		}
	}
}

func (v narrowView[T, NT]) KeySlice() []NT {
	if v.st == nil {
		return nil
	}
	out := make([]NT, 0, v.st.c.Len())
	for t := range v.st.c.Keys() {
		out = append(out, v.st.vw.To(t))
	}
	return out
}

// countThrough is the generic boundary: it takes the interface, so a container
// or any view can be handed to it.
func countThrough[T any](s Set[T]) int { return s.Len() }
