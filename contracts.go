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

// MutableSet is the contract for set-like containers: everything Elems offers,
// the question a set exists to answer, and the writes.
//
// T is unconstrained. Implementations that need comparable elements say so in
// their own type parameters; the contract never needed to. See ADR 0012.
//
// There is no read-only tier below this one. ADR 0013 removed it: a container
// satisfies a structural read contract inherently, so such a contract prevents
// nothing — a holder asserts back to the container and writes. A function that
// must not write takes a SetView, which is sealed and which a container cannot
// satisfy. A caller holding a container passes ViewHashSetIdentity(s), which
// converts nothing and allocates nothing.
//
// Operations returning the container type are absent — Union, Intersect,
// Difference and Clone — because Go has no covariant returns, so a method
// returning a concrete *HashSet cannot satisfy an interface method returning
// the interface. That is the ceiling ADR 0002 documented for set algebra.
type MutableSet[T any] interface {
	Elems[T]
	Has(T) bool
	Add(...T)
	Remove(...T)
}

// MutableDict is the contract for key-value containers: everything Elems2
// offers, Get, and the writes.
//
// K is unconstrained, for the reason given on MutableSet, and there is likewise
// no read-only tier below this one — a function that must not write takes a
// DictView. See ADR 0013.
//
// MutableDict carries only Get, not Floor, Ceil, Min, Max or Range, because Map
// cannot provide them. The ordered reads live on SortedDictView, on the view
// side; ADR 0008's follow-up on an ordered contract for *containers* is still
// open.
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
type MutableDict[K any, V any] interface {
	Elems2[K, V]
	Get(K) (V, bool)
	Set(K, V)
	Delete(K)
}

// The two layers, asserted against every container.
var (
	_ Elems[int]               = (*HashSet[int])(nil)
	_ Elems[int]               = (*SortedSet[int])(nil)
	_ Elems2[string, int]      = Map[string, int]{}
	_ Elems2[string, int]      = (*HashDict[string, int])(nil)
	_ Elems2[string, int]      = (*SortedDict[string, int])(nil)
	_ MutableSet[int]          = (*HashSet[int])(nil)
	_ MutableSet[int]          = (*SortedSet[int])(nil)
	_ MutableDict[string, int] = Map[string, int]{}
	_ MutableDict[string, int] = (*HashDict[string, int])(nil)
	_ MutableDict[string, int] = (*SortedDict[string, int])(nil)
)

// No container satisfies a view interface. This is the seal, asserted: each
// line below is a compile error if uncommented.
//
//	var _ SetView[int]           = (*HashSet[int])(nil)     // missing sealedView
//	var _ DictView[string, int]  = (*HashDict[string, int])(nil)
//	var _ DictView[string, int]  = Map[string, int]{}
