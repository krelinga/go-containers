package containers

import (
	"iter"
	"maps"
)

// MapSet is an unordered set backed by a map.
//
// It is a reference type (ADR 0018): a one-word value whose copies share the
// same contents. Copying one is defined behaviour, not a bug, and there is
// nothing for go vet's copylocks to catch.
//
// The zero value behaves exactly as a nil map does -- reads are total, writes
// panic -- and it gets that for free, because the field IS a map:
//
//	var s MapSet[int]
//	s.Len()    // 0
//	s.Has(1)   // false
//	s.Add(1)   // panics, as m[k] = v does on a nil map
type MapSet[T comparable] struct {
	m map[T]struct{}
}

// NewMapSet returns a MapSet containing vs.
//
// vs is copied and not retained; the caller remains free to modify it (ADR
// 0017). Spread a slice to build from another container:
//
//	NewMapSet(other.KeySlice()...)
func NewMapSet[T comparable](vs ...T) MapSet[T] {
	s := MapSet[T]{m: make(map[T]struct{}, len(vs))}
	for _, v := range vs {
		s.m[v] = struct{}{}
	}
	return s
}

// IsZero reports whether s was ever constructed. It is the analogue of
// m == nil for a builtin map, and is needed about as often: because reads are
// total, the question a caller usually has is Len() == 0.
func (s MapSet[T]) IsZero() bool { return s.m == nil }

// Add inserts one element. Adding a value already present is a no-op.
// Adding to a zero MapSet panics, as writing to a nil map does.
func (s MapSet[T]) Add(v T) { s.m[v] = struct{}{} }

// AddAll inserts every element of vs. Spread a slice to bulk-insert:
//
//	s.AddAll(other.KeySlice()...)
//
// vs is copied and not retained. With no arguments it writes nothing and so
// does not panic, exactly as a loop that never reaches m[k] = v does not.
func (s MapSet[T]) AddAll(vs ...T) {
	for _, v := range vs {
		s.m[v] = struct{}{} // panics on a zero MapSet, as m[k] = v does
	}
}

// Delete removes one element. Removing a value not present is a no-op, and
// deleting from a zero MapSet is a no-op, as delete on a nil map is.
func (s MapSet[T]) Delete(v T) { delete(s.m, v) }

// DeleteAll removes every element of ks. Spread a slice to bulk-delete:
//
//	s.DeleteAll(s.KeySlice()...)
//
// That is safe because KeySlice already returned a copy (ADR 0017).
func (s MapSet[T]) DeleteAll(ks ...T) {
	for _, k := range ks {
		delete(s.m, k)
	}
}

// Has reports whether v is in the set.
func (s MapSet[T]) Has(v T) bool {
	_, ok := s.m[v]
	return ok
}

// Len returns the number of elements.
func (s MapSet[T]) Len() int { return len(s.m) }

// Keys iterates the elements in no particular order. A set's element is its key
// (ADR 0017), so this is the key-shaped read and there is no value side.
//
// Unlike the slice-backed containers, this binds the LIVE map: a later Add IS
// seen. That is what ranging a builtin map gives you, and modifying a map while
// iterating it is unspecified in Go -- so the rule is the same either way, which
// is: do not modify a container while iterating it.
func (s MapSet[T]) Keys() iter.Seq[T] { return maps.Keys(s.m) }

// KeySlice returns the elements as a new slice, in no particular order.
//
// The result is a full, independent copy: nothing the set does afterwards is
// visible through it, and nothing done to it is visible in the set.
func (s MapSet[T]) KeySlice() []T {
	if s.m == nil {
		return nil
	}
	out := make([]T, 0, len(s.m))
	for v := range s.m {
		out = append(out, v)
	}
	return out
}

// Clone returns an independent copy. Mutating the result does not affect s.
//
// This is the operation that matters now that copies share: `t := s` gives
// another handle on the same set, and Clone gives a separate one.
func (s MapSet[T]) Clone() MapSet[T] { return MapSet[T]{m: maps.Clone(s.m)} }

// Union returns a new set holding every element of s and o.
func (s MapSet[T]) Union(o MapSet[T]) MapSet[T] {
	out := MapSet[T]{m: make(map[T]struct{}, len(s.m)+len(o.m))}
	for v := range s.m {
		out.m[v] = struct{}{}
	}
	for v := range o.m {
		out.m[v] = struct{}{}
	}
	return out
}

// Intersect returns a new set holding the elements present in both.
func (s MapSet[T]) Intersect(o MapSet[T]) MapSet[T] {
	small, large := s.m, o.m
	if len(large) < len(small) {
		small, large = large, small
	}
	out := MapSet[T]{m: make(map[T]struct{})}
	for v := range small {
		if _, ok := large[v]; ok {
			out.m[v] = struct{}{}
		}
	}
	return out
}

// Difference returns a new set holding the elements of s not in o.
func (s MapSet[T]) Difference(o MapSet[T]) MapSet[T] {
	out := MapSet[T]{m: make(map[T]struct{})}
	for v := range s.m {
		if _, ok := o.m[v]; !ok {
			out.m[v] = struct{}{}
		}
	}
	return out
}
