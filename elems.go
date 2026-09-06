package containers

import "iter"

// Elems is the read-only contract shared by the single-value containers in this
// package: something with a known number of elements that can be iterated.
//
// Its purpose is the length. iter.Seq is a function and carries no size, so a
// constructor handed one must grow its backing array as it goes — reallocating
// repeatedly and, for a large source, churning several times the memory a
// single sized allocation would. Accepting an Elems lets the constructor
// allocate once. See ADR 0006 and experiments/sizedcollect.
//
// Len is a capacity *hint*. An implementation whose Len disagrees with what All
// yields produces a worse allocation, never a wrong result.
//
// *Set[T] and *SortedSet[T] satisfy Elems[T]. Note that the pointer does, and
// the value does not, since every method takes a pointer receiver.
type Elems[T any] interface {
	Len() int
	All() iter.Seq[T]
}

// Elems2 is Elems for key-value containers. *SortedMap[K, V] satisfies it.
//
// It is a separate interface rather than a parameterisation of Elems because Go
// has no higher-kinded types: iter.Seq and iter.Seq2 cannot be unified. This is
// the same limit that keeps set algebra off an interface in ADR 0002.
type Elems2[K, V any] interface {
	Len() int
	All() iter.Seq2[K, V]
}
