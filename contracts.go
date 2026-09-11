package containers

import "iter"

// The shape interfaces every container and view in this package satisfies.
//
// They are named for what a type EXPOSES, not for what kind of container it is
// (ADR 0018), so no name here collides with a container name and a future
// LinkedList is free to use Positions.
//
// They are deliberately NOT sealed: containers satisfy them too, which is the
// point. A function that needs only reads takes one of these and accepts a
// container or a view; a function that needs a read-only GUARANTEE takes a
// concrete view type, which nothing can assert back to something writable.
//
// They take `any` keys and values rather than `comparable` (ADR 0012);
// implementations state their own constraints.

// Values is anything with values: maps, sequences, and their views. Not sets,
// whose elements are keys (ADR 0017).
type Values[V any] interface {
	Len() int
	Values() iter.Seq[V]
	ValueSlice() []V
}

// Keys is sets, and the key side of maps.
type Keys[K any] interface {
	Len() int
	Has(K) bool
	Keys() iter.Seq[K]
	KeySlice() []K
}

// KeyValues is the pair side: maps and their views.
type KeyValues[K, V any] interface {
	Keys[K]
	Values[V]
	Get(K) (V, bool)
	All() iter.Seq2[K, V]
	AllSlice() []Entry[K, V]
}

// SortedKeys is the ordered key reads. The *Key spelling is what lets one
// interface cover sorted sets AND sorted maps: a sorted map's Min returns a
// pair, so it could not share a method named Min (ADR 0018).
type SortedKeys[K any] interface {
	Keys[K]
	MinKey() (K, bool)
	MaxKey() (K, bool)
	FloorKey(K) (K, bool)
	CeilKey(K) (K, bool)
	RangeKeys(lo, hi K) iter.Seq[K]
}

// SortedKeyValues adds the pair-returning ordered reads, so a SortedMap
// satisfies both ordered tiers.
type SortedKeyValues[K, V any] interface {
	KeyValues[K, V]
	SortedKeys[K]
	Min() (K, V, bool)
	Max() (K, V, bool)
	Floor(K) (K, V, bool)
	Ceil(K) (K, V, bool)
	Range(lo, hi K) iter.Seq2[K, V]
}

// Positions is the sequence analogue of Keys, spelled differently because a
// position is not a key: it is assigned by the container, not chosen by the
// caller (ADRs 0016, 0018).
//
// There is deliberately no membership test. HasPosition would be the analogue
// of Has, but on a Vector it is only p < Len(); it earns its place when a
// position is a cursor rather than an int.
type Positions[P any] interface {
	Len() int
	Positions() iter.Seq[P]
	PositionSlice() []P
}

// PositionValues is sequences: Vector, a slice view, and eventually LinkedList.
type PositionValues[P, V any] interface {
	Positions[P]
	Values[V]
	At(P) V
	All() iter.Seq2[P, V]
	AllSlice() []Entry[P, V]
}

// The mutation tiers are today's contracts, kept so every container states one.
// Whether the SETTER vocabulary should be factored the way the getters now are
// is deferred until there is a second mutable sequence to argue from (ADR 0018).

type MutableKeys[K any] interface {
	Keys[K]
	Add(K)
	AddAll(...K)
	Delete(K)
	DeleteAll(...K)
}

type MutableKeyValues[K, V any] interface {
	KeyValues[K, V]
	Set(K, V)
	SetAll(...Entry[K, V])
	Delete(K)
	DeleteAll(...K)
}

var (
	_ Keys[int]                     = MapSet[int]{}
	_ MutableKeys[int]              = MapSet[int]{}
	_ SortedKeys[int]               = SortedSet[int]{}
	_ MutableKeys[int]              = SortedSet[int]{}
	_ KeyValues[string, int]        = Map[string, int]{}
	_ MutableKeyValues[string, int] = Map[string, int]{}
	_ SortedKeyValues[string, int]  = SortedMap[string, int]{}
	_ SortedKeys[string]            = SortedMap[string, int]{}
	_ MutableKeyValues[string, int] = SortedMap[string, int]{}
	_ PositionValues[int, string]   = Vector[string]{}
	_ Values[string]                = Vector[string]{}
)

// Views satisfy the read tiers and never the mutation ones.
var (
	_ Keys[string]                 = MapSetView[string]{}
	_ SortedKeys[int]              = SortedSetView[int]{}
	_ KeyValues[string, int]       = MapView[string, int]{}
	_ SortedKeyValues[string, int] = SortedMapView[string, int]{}
	_ SortedKeys[string]           = SortedMapView[string, int]{}
	_ PositionValues[int, string]  = VectorView[string]{}
	_ Values[string]               = VectorView[string]{}
)
