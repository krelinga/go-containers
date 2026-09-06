package containers

import "iter"

// noCopy makes `go vet`'s copylocks analyzer report attempts to copy a Set
// struct. It is declared first in Set because a zero-sized field in trailing
// position would force the struct to be padded. See ADR 0002, decision 5.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// A Set is an unordered collection of distinct values.
//
// The zero value is an empty Set ready to use:
//
//	var s containers.Set[string]
//	s.Add("read")
//
// Every method has a pointer receiver, so calling one on an addressable Set
// value takes its address automatically. A nil *Set is not usable: every method
// panics on one, deliberately, because a nil *Set is almost always a bug and
// reporting it as empty would hide that. Prefer a Set field over a *Set field
// in a struct, since the former is usable at its zero value.
//
// A Set holds a map internally, so copying the struct produces two Sets sharing
// one map, exactly as copying a map produces two names for one map. Use Clone
// to make an independent copy. `go vet` reports struct copies, but note that
// `go test` does not run that check: use `go vet ./...` or `go test -vet=all`.
//
// A Set is not safe for concurrent use.
type Set[T comparable] struct {
	_ noCopy
	m map[T]struct{}
}

// NewSet returns a Set containing vs. It is a convenience for construction with
// initial values; the zero value of Set is equally usable.
func NewSet[T comparable](vs ...T) *Set[T] {
	s := &Set[T]{m: make(map[T]struct{}, len(vs))}
	for _, v := range vs {
		s.m[v] = struct{}{}
	}
	return s
}

// Add adds vs to the set. Adding a value already present is a no-op.
func (s *Set[T]) Add(vs ...T) {
	if s.m == nil {
		s.m = make(map[T]struct{}, len(vs))
	}
	for _, v := range vs {
		s.m[v] = struct{}{}
	}
}

// Remove removes vs from the set. Removing a value not present is a no-op.
func (s *Set[T]) Remove(vs ...T) {
	if s.m == nil {
		// Also forces the nil-receiver panic when vs is empty, which a bare
		// range over vs would skip.
		return
	}
	for _, v := range vs {
		delete(s.m, v)
	}
}

// Has reports whether v is in the set.
func (s *Set[T]) Has(v T) bool {
	_, ok := s.m[v]
	return ok
}

// Len returns the number of values in the set.
func (s *Set[T]) Len() int { return len(s.m) }

// All returns an iterator over the values in the set, in no particular order.
//
// The iterator is bound to the set's contents as of the call to All, not as of
// iteration. Modifying the set during iteration is not supported.
func (s *Set[T]) All() iter.Seq[T] {
	m := s.m // read eagerly, so a nil receiver panics here rather than on iteration
	return func(yield func(T) bool) {
		for v := range m {
			if !yield(v) {
				return
			}
		}
	}
}

// Clone returns an independent copy of the set. Mutating the result does not
// affect s.
func (s *Set[T]) Clone() *Set[T] {
	out := &Set[T]{m: make(map[T]struct{}, s.Len())}
	for v := range s.All() {
		out.m[v] = struct{}{}
	}
	return out
}

// Union returns a new set containing every value in s or o.
func (s *Set[T]) Union(o *Set[T]) *Set[T] {
	out := s.Clone()
	for v := range o.All() {
		out.m[v] = struct{}{}
	}
	return out
}

// Intersect returns a new set containing the values in both s and o.
func (s *Set[T]) Intersect(o *Set[T]) *Set[T] {
	// Iterate the smaller set and probe the larger.
	small, large := s, o
	if large.Len() < small.Len() {
		small, large = large, small
	}
	out := &Set[T]{m: make(map[T]struct{})}
	for v := range small.All() {
		if large.Has(v) {
			out.m[v] = struct{}{}
		}
	}
	return out
}

// Difference returns a new set containing the values in s that are not in o.
func (s *Set[T]) Difference(o *Set[T]) *Set[T] {
	// o.Len() both sizes the result and forces o to be evaluated, so a nil o
	// panics even when s is empty and the loop below never runs.
	out := &Set[T]{m: make(map[T]struct{}, max(0, s.Len()-o.Len()))}
	for v := range s.All() {
		if !o.Has(v) {
			out.m[v] = struct{}{}
		}
	}
	return out
}

// SetLike is the container contract shared by set implementations. Algebra is
// deliberately excluded: Go has no covariant returns, so a method returning a
// concrete *Set cannot satisfy an interface method returning the interface.
// Operations generic over containers are free functions over iter.Seq instead.
//
// The name is a placeholder; see ADR 0002 follow-ups.
type SetLike[T comparable] interface {
	Add(...T)
	Has(T) bool
	Len() int
	All() iter.Seq[T]
}

var _ SetLike[int] = (*Set[int])(nil)
