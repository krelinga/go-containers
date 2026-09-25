package containers

import "iter"

// The shape interfaces every container and view in this package satisfies.
//
// They are named for what a type EXPOSES, not for what kind of container it is
// (ADR 0018), so no name here collides with a container name and a future
// LinkedList is free to use Positions.
//
// They are SEALED (ADR 0022): each declares the unexported callViewFirst, which
// only a view implements. So a container cannot be passed where reads are
// promised -- the caller writes c.View(), which is free -- and a value taken
// from one of these can never be asserted back to something writable.
//
// Sealing is only a guarantee because the CONTAINER is excluded. A sealed
// interface that a container also satisfies is exactly as leaky as an unsealed
// one, since the container is then genuinely the dynamic type. And ONE token
// serves the whole package: an interface-typed value satisfies another sealed
// interface only if it DECLARES the same unexported method, so a token per
// interface would stop the tiers composing (experiments/sealing).
//
// The seal constrains STRUCTURE only. Through a view over a container of
// pointers, the pointed-to values stay mutable; Go cannot express "contains no
// pointers" as a constraint, so that half stays a documented convention -- as it
// does for slices.Clone.
//
// They take `any` keys and values rather than `comparable` (ADR 0012);
// implementations state their own constraints.

// Values is anything with values: maps, sequences, and their views. Not sets,
// whose elements are keys (ADR 0017).
type Values[V any] interface {
	Len() int
	Values() iter.Seq[V]
	ValueSlice() []V

	callViewFirst()
}

// Keys is sets, and the key side of maps.
type Keys[K any] interface {
	Len() int
	Has(K) bool
	Keys() iter.Seq[K]
	KeySlice() []K

	callViewFirst()
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

	callViewFirst()
}

// PositionValues is sequences: Vector, a slice view, and eventually LinkedList.
type PositionValues[P, V any] interface {
	Positions[P]
	Values[V]
	At(P) V
	All() iter.Seq2[P, V]
	AllSlice() []Entry[P, V]
}

// mutatesKeys and mutatesKeyValues are the mutator vocabulary ADR 0017 settled,
// CHECKED but not published (ADR 0022). They replaced exported MutableKeys and
// MutableKeyValues, and two properties are load-bearing:
//
//   - UNEXPORTED, so no caller can take one as a parameter. An exported mutation
//     tier is a read-only-looking handle that writes, which is what 0022 closes.
//   - They never EMBED a read tier. Embedding a sealed one would force every
//     container to implement callViewFirst, which reopens the hole entirely --
//     measured, not supposed (experiments/sealing).
//
// Their only job is to fail the build when a container drifts: a new container
// that forgets DeleteAll, a rename applied to one sibling and not the other, or a
// signature that slips from ...K to []K. Nothing else asserts that vocabulary
// across containers; each container's own tests only cover itself.

type mutatesKeys[K any] interface {
	Add(K)
	AddAll(...K)
	Delete(K)
	DeleteAll(...K)
}

type mutatesKeyValues[K, V any] interface {
	Set(K, V)
	SetAll(...Entry[K, V])
	Delete(K)
	DeleteAll(...K)
}

var (
	_ mutatesKeys[int]              = MapSet[int]{}
	_ mutatesKeys[int]              = SortedSet[int]{}
	_ mutatesKeyValues[string, int] = Map[string, int]{}
	_ mutatesKeyValues[string, int] = SortedMap[string, int]{}
)

// Containers deliberately do NOT appear here: sealing the read tiers IS the
// statement that a container does not satisfy them. What a container exposes for
// reads is checked by the inner tiers below, which the views hold.

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

// ---------------------------------------------------------------------------
// The inner tiers: unexported, unsealed mirrors of the read tiers above.
//
// A view is struct{ impl <tier> }, and an identity view passes its CONTAINER
// straight in as that implementation. Sealing the public tiers forbids exactly
// that, so the plumbing needs copies a container can satisfy. These are what
// check "a new container exposes its reads whole" now that the public tiers
// cannot (ADR 0022).
//
// Adapters were the alternative: one per container, supplying the token and
// forwarding every read. That is ~43 forwarding methods against ~7 interface
// declarations, and forwarding is where transcription bugs live.
// ---------------------------------------------------------------------------

type innerValues[V any] interface {
	Len() int
	Values() iter.Seq[V]
	ValueSlice() []V
}

type innerKeys[K any] interface {
	Len() int
	Has(K) bool
	Keys() iter.Seq[K]
	KeySlice() []K
}

type innerKeyValues[K, V any] interface {
	innerKeys[K]
	innerValues[V]
	Get(K) (V, bool)
	All() iter.Seq2[K, V]
	AllSlice() []Entry[K, V]
}

type innerSortedKeys[K any] interface {
	innerKeys[K]
	MinKey() (K, bool)
	MaxKey() (K, bool)
	FloorKey(K) (K, bool)
	CeilKey(K) (K, bool)
	RangeKeys(lo, hi K) iter.Seq[K]
}

type innerSortedKeyValues[K, V any] interface {
	innerKeyValues[K, V]
	innerSortedKeys[K]
	Min() (K, V, bool)
	Max() (K, V, bool)
	Floor(K) (K, V, bool)
	Ceil(K) (K, V, bool)
	Range(lo, hi K) iter.Seq2[K, V]
}

type innerPositions[P any] interface {
	Len() int
	Positions() iter.Seq[P]
	PositionSlice() []P
}

type innerPositionValues[P, V any] interface {
	innerPositions[P]
	innerValues[V]
	At(P) V
	All() iter.Seq2[P, V]
	AllSlice() []Entry[P, V]
}

// Containers satisfy the inner tiers whole. This is the assertion block that
// replaces the container half of the public one above, and the place to extend
// when a container is added.
var (
	_ innerKeys[int]                    = MapSet[int]{}
	_ innerSortedKeys[int]              = SortedSet[int]{}
	_ innerKeyValues[string, int]       = Map[string, int]{}
	_ innerSortedKeyValues[string, int] = SortedMap[string, int]{}
	_ innerSortedKeys[string]           = SortedMap[string, int]{}
	_ innerPositionValues[int, string]  = Vector[string]{}
	_ innerValues[string]               = Vector[string]{}
)

// callViewFirst is the package-wide seal token. It is named for the compiler
// error it produces, because that clause is the entire diagnostic a caller gets:
//
//	cannot use s (variable of struct type MapSet[int]) as Keys[int] value in
//	argument to Publish: MapSet[int] does not implement Keys[int]
//	(missing method callViewFirst)
//
// Only views implement it. Never add it to a container.
func (v MapSetView[NT]) callViewFirst()       {}
func (v SortedSetView[T]) callViewFirst()     {}
func (v MapView[NK, NV]) callViewFirst()      {}
func (v SortedMapView[K, NV]) callViewFirst() {}
func (v VectorView[NT]) callViewFirst()       {}
