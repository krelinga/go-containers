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
// *HashSet[T] and *SortedSet[T] satisfy Elems[T]. Note that the pointer does, and
// the value does not, since every method takes a pointer receiver.
type Elems[T any] interface {
	Len() int
	All() iter.Seq[T]
}

// Elems2 is Elems for key-value containers. *SortedDict[K, V] satisfies it.
//
// It is a separate interface rather than a parameterisation of Elems because Go
// has no higher-kinded types: iter.Seq and iter.Seq2 cannot be unified. This is
// the same limit that keeps set algebra off an interface in ADR 0002.
//
// Whether an iterator reflects writes made after All returned is **unspecified**.
// SortedDict binds its contents at call time, but only as a side effect of the
// rule that a nil receiver must panic at the call rather than at iteration; Map
// wraps a builtin map, for which the Go spec leaves the question open. Do not
// depend on either behaviour.
type Elems2[K, V any] interface {
	Len() int
	All() iter.Seq2[K, V]
}

// Set is the read-only contract for set-like containers: everything Elems
// offers, plus the question a set exists to answer.
//
// Satisfied by *HashSet[T] and *SortedSet[T]. Take this rather than MutableSet
// when a function only needs to read, so it cannot write through the argument.
type Set[T comparable] interface {
	Elems[T]
	Has(T) bool
}

// Dict is the read-only contract for key-value containers.
//
// Satisfied by Map[K, V] as a value and *SortedDict[K, V] as a pointer. That
// asymmetry is inherent to Go: value receivers satisfy from a value, pointer
// receivers do not.
//
// Dict carries only Get, not Floor, Ceil, Min, Max or Range, because Map cannot
// provide them. Generic code over Dict cannot do ordered reads; see ADR 0008's
// follow-up on a contract for ordered containers.
type Dict[K comparable, V any] interface {
	Elems2[K, V]
	Get(K) (V, bool)
}

// MutableSet adds writing to Set.
//
// Operations returning the container type are absent — Union, Intersect,
// Difference and Clone — because Go has no covariant returns, so a method
// returning a concrete *HashSet cannot satisfy an interface method returning
// the interface. That is the ceiling ADR 0002 documented for set algebra.
type MutableSet[T comparable] interface {
	Set[T]
	Add(...T)
	Remove(...T)
}

// MutableDict adds writing to Dict.
//
// # Mutation during iteration is not supported
//
// Implementations differ, so the contract takes the stricter guarantee. Ranging
// over a builtin map while deleting from it is permitted by the Go spec, but
// SortedDict.All binds its backing slice and Delete shifts elements within it,
// so the same code silently corrupts that iteration. Collect the keys you want
// to change, finish iterating, then apply them.
//
// Whether an iterator reflects writes made after it was created is likewise
// unspecified; see Elems2.
type MutableDict[K comparable, V any] interface {
	Dict[K, V]
	Set(K, V)
	Delete(K)
}

// The layering, asserted against every container.
var (
	_ Set[int]                 = (*HashSet[int])(nil)
	_ Set[int]                 = (*SortedSet[int])(nil)
	_ MutableSet[int]          = (*HashSet[int])(nil)
	_ MutableSet[int]          = (*SortedSet[int])(nil)
	_ Dict[string, int]        = Map[string, int]{}
	_ Dict[string, int]        = (*SortedDict[string, int])(nil)
	_ MutableDict[string, int] = Map[string, int]{}
	_ MutableDict[string, int] = (*SortedDict[string, int])(nil)
)
