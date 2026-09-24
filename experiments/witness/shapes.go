package witness

import (
	"iter"
	"maps"
)

type item struct{ Name string }

// A map-backed set, in the shape ADR 0018 ships: a one-word reference value.
type set[T comparable] struct{ m map[T]struct{} }

func newSet[T comparable](vs ...T) set[T] {
	s := set[T]{m: make(map[T]struct{}, len(vs))}
	for _, v := range vs {
		s.m[v] = struct{}{}
	}
	return s
}

func (s set[T]) Len() int          { return len(s.m) }
func (s set[T]) Has(v T) bool      { _, ok := s.m[v]; return ok }
func (s set[T]) Keys() iter.Seq[T] { return maps.Keys(s.m) }

// --- viewers ---

// statelessViewer is zero-size: the case a pure witness can serve.
type statelessViewer struct{}

func (statelessViewer) ToKeyView(i *item) string { return i.Name }

// statefulViewer holds a registry, which is what ADR 0012 said a witness cannot
// do -- for the PURE variant. The carried variant stores the value.
type statefulViewer struct{ known map[string]*item }

func (v statefulViewer) ToKeyView(i *item) string { return i.Name }
func (v statefulViewer) FromKeyView(s string) (*item, bool) {
	i, ok := v.known[s]
	return i, ok
}

// ---------------------------------------------------------------------------
// TODAY: the view holds a shape interface. Two words; dispatch on every call.
// ---------------------------------------------------------------------------

type keysIface[NT any] interface {
	Len() int
	Keys() iter.Seq[NT]
}

type ifaceView[NT any] struct{ impl keysIface[NT] }

func (v ifaceView[NT]) Len() int           { return v.impl.Len() }
func (v ifaceView[NT]) Keys() iter.Seq[NT] { return v.impl.Keys() }

type convertedKeys[T comparable, NT any] struct {
	s  set[T]
	vw interface{ ToKeyView(T) NT }
}

func (c convertedKeys[T, NT]) Len() int { return c.s.Len() }
func (c convertedKeys[T, NT]) Keys() iter.Seq[NT] {
	s, vw := c.s, c.vw
	return func(yield func(NT) bool) {
		for t := range s.Keys() {
			if !yield(vw.ToKeyView(t)) {
				return
			}
		}
	}
}

func viewIface[T comparable, NT any](s set[T], vw interface{ ToKeyView(T) NT }) ifaceView[NT] {
	return ifaceView[NT]{impl: convertedKeys[T, NT]{s, vw}}
}

// ---------------------------------------------------------------------------
// PURE WITNESS: the converter is a type parameter, its zero value materialised
// per call. One word. Stateless converters only (ADR 0012's objection).
// ---------------------------------------------------------------------------

type toKeyView[T, NT any] interface{ ToKeyView(T) NT }

type pureWitness[T comparable, NT any, VW toKeyView[T, NT]] struct{ s set[T] }

func (v pureWitness[T, NT, VW]) Len() int { return v.s.Len() }

func (v pureWitness[T, NT, VW]) Keys() iter.Seq[NT] {
	var vw VW // zero value, materialised here
	s := v.s
	return func(yield func(NT) bool) {
		for t := range s.Keys() {
			if !yield(vw.ToKeyView(t)) {
				return
			}
		}
	}
}

func viewPure[T comparable, NT any, VW toKeyView[T, NT]](s set[T]) pureWitness[T, NT, VW] {
	return pureWitness[T, NT, VW]{s: s}
}

// ---------------------------------------------------------------------------
// CARRIED WITNESS: the converter is a CONCRETE type parameter AND a field. The
// call is still static, and the viewer may hold state. Width is 1 + sizeof(VW),
// so it is one word when the viewer is zero-size.
// ---------------------------------------------------------------------------

type carriedWitness[T comparable, NT any, VW toKeyView[T, NT]] struct {
	s  set[T]
	vw VW
}

func (v carriedWitness[T, NT, VW]) Len() int { return v.s.Len() }

func (v carriedWitness[T, NT, VW]) Keys() iter.Seq[NT] {
	s, vw := v.s, v.vw
	return func(yield func(NT) bool) {
		for t := range s.Keys() {
			if !yield(vw.ToKeyView(t)) {
				return
			}
		}
	}
}

func viewCarried[T comparable, NT any, VW toKeyView[T, NT]](s set[T], vw VW) carriedWitness[T, NT, VW] {
	return carriedWitness[T, NT, VW]{s: s, vw: vw}
}

// A shape interface, to price boxing.
type Keys[NT any] interface {
	Len() int
	Keys() iter.Seq[NT]
}

// A zero-size field at the END of a struct is padded, to keep a pointer to it
// from running past the allocation. At the FRONT it costs nothing. So field
// order decides whether a carried witness with a stateless viewer is one word.
type carriedFront[T comparable, NT any, VW toKeyView[T, NT]] struct {
	vw VW
	s  set[T]
}

func (v carriedFront[T, NT, VW]) Len() int { return v.s.Len() }

func (v carriedFront[T, NT, VW]) Keys() iter.Seq[NT] {
	s, vw := v.s, v.vw
	return func(yield func(NT) bool) {
		for t := range s.Keys() {
			if !yield(vw.ToKeyView(t)) {
				return
			}
		}
	}
}

func viewCarriedFront[T comparable, NT any, VW toKeyView[T, NT]](s set[T], vw VW) carriedFront[T, NT, VW] {
	return carriedFront[T, NT, VW]{vw: vw, s: s}
}

// ---------------------------------------------------------------------------
// PARAMETERISED CONTAINER: the container itself carries the witness as a type
// parameter, and View() boxes it into a view INTERFACE.
//
// The witness must be STATELESS -- a defined map type has nowhere to store one.
// The conversion is still static (VW is concrete on the container); only the
// outer call through the view interface is dynamic.
// ---------------------------------------------------------------------------

type paramSet[T comparable, NT any, VW toKeyView[T, NT]] struct{ m map[T]struct{} }

func (s paramSet[T, NT, VW]) Len() int { return len(s.m) }

func (s paramSet[T, NT, VW]) Keys() iter.Seq[NT] {
	var vw VW // stateless
	m := s.m
	return func(yield func(NT) bool) {
		for t := range m {
			if !yield(vw.ToKeyView(t)) {
				return
			}
		}
	}
}

// View boxes the one-word container into the interface.
func (s paramSet[T, NT, VW]) View() Keys[NT] { return s }

func newParamSet[T comparable, NT any, VW toKeyView[T, NT]](vs ...T) paramSet[T, NT, VW] {
	s := paramSet[T, NT, VW]{m: make(map[T]struct{}, len(vs))}
	for _, v := range vs {
		s.m[v] = struct{}{}
	}
	return s
}
