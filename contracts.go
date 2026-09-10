package containers

import "iter"

// The contracts every container in this package satisfies.
//
// There are two layers, and no read-only tier: a container satisfies a
// structural read contract inherently, so one prevents nothing (ADR 0013).
// Read-only is expressed by the sealed view interfaces in views.go instead.
//
// Elems and Elems2 used to sit below these, and are gone (ADR 0017). Their job
// was carrying a length for preallocating constructors, and bulk operations now
// take slices, which carry their own.
//
// Both layers take `any` keys and values rather than `comparable` (ADR 0012);
// implementations state their own constraints.

// MutableSet is a set that can be read and written.
//
// A set's element is its key (ADR 0017), so the read methods are the key-shaped
// ones and there is no value side.
type MutableSet[T any] interface {
	Len() int
	Keys() iter.Seq[T]
	KeySlice() []T
	Has(T) bool
	Add(T)
	Delete(T)
}

// MutableDict is a key/value container that can be read and written.
type MutableDict[K any, V any] interface {
	Len() int
	Keys() iter.Seq[K]
	Values() iter.Seq[V]
	All() iter.Seq2[K, V]
	KeySlice() []K
	ValueSlice() []V
	AllSlice() []KeyValue[K, V]
	Get(K) (V, bool)
	Set(K, V)
	Delete(K)
}

var (
	_ MutableSet[int]          = (*HashSet[int])(nil)
	_ MutableSet[int]          = (*SortedSet[int])(nil)
	_ MutableDict[string, int] = Map[string, int]{}
	_ MutableDict[string, int] = (*HashDict[string, int])(nil)
	_ MutableDict[string, int] = (*SortedDict[string, int])(nil)
)
