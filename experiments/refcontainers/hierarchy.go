package refcontainers

// A compile-checked sketch of option (b)'s interface layer: shape interfaces
// named for what a type EXPOSES rather than for what kind of container it is.
//
// Nothing here is measured -- it is a type-checking question, and the file
// earns its place by FAILING to compile if the factoring is wrong. The negative
// cases live in hierarchy_test.go, which is what makes the assertions worth
// anything: an earlier draft shared an embedded body between containers and
// views and silently let views satisfy the mutation tiers.

import (
	"cmp"
	"iter"
)

// One pair type for keys AND positions. The second element is always a value;
// only the first needed a name general enough to cover both.
type hSlotValue[S, V any] struct {
	Slot  S
	Value V
}

// PositionValue mirrors hKeyValue for the positional side. Whether this is worth
// a second pair type, or whether positions should reuse hKeyValue, is an open
// question -- see the notes.

// ---------------------------------------------------------------------------
// Shape interfaces: named for what a type EXPOSES, not for what kind of
// container it is. No name here collides with a container name.
// ---------------------------------------------------------------------------

// hValues is anything with values: dicts, sequences, and their views. Not sets,
// which are key-only (ADR 0017).
type hValues[V any] interface {
	Len() int
	hValues() iter.Seq[V]
	ValueSlice() []V
}

// hKeys is sets, and the key side of dicts.
type hKeys[K any] interface {
	Len() int
	Has(K) bool
	hKeys() iter.Seq[K]
	KeySlice() []K
}

// hKeyValues is the pair side: dicts and their views.
type hKeyValues[K, V any] interface {
	hKeys[K]
	hValues[V]
	Get(K) (V, bool)
	All() iter.Seq2[K, V]
	AllSlice() []hSlotValue[K, V]
}

// hPositions is the sequence analogue of hKeys, deliberately named differently
// because a position is not a key: it is assigned by the container, not chosen
// by the caller.
type hPositions[P any] interface {
	Len() int
	hPositions() iter.Seq[P]
	PositionSlice() []P
}

// hPositionValues is sequences: hVector, a slice view, and eventually LinkedList.
type hPositionValues[P, V any] interface {
	hPositions[P]
	hValues[V]
	At(P) V
	All() iter.Seq2[P, V]
	AllSlice() []hSlotValue[P, V]
}

// --- read bodies ---

type hKeysBody[K any] struct{}

func (hKeysBody[K]) Len() int           { return 0 }
func (hKeysBody[K]) Has(K) bool         { return false }
func (hKeysBody[K]) hKeys() iter.Seq[K] { return emptySeq[K] }
func (hKeysBody[K]) KeySlice() []K      { return nil }

type hKvBody[K, V any] struct{}

func (hKvBody[K, V]) Len() int                     { return 0 }
func (hKvBody[K, V]) Has(K) bool                   { return false }
func (hKvBody[K, V]) hKeys() iter.Seq[K]           { return emptySeq[K] }
func (hKvBody[K, V]) KeySlice() []K                { return nil }
func (hKvBody[K, V]) hValues() iter.Seq[V]         { return emptySeq[V] }
func (hKvBody[K, V]) ValueSlice() []V              { return nil }
func (hKvBody[K, V]) Get(K) (V, bool)              { var z V; return z, false }
func (hKvBody[K, V]) All() iter.Seq2[K, V]         { return nil }
func (hKvBody[K, V]) AllSlice() []hSlotValue[K, V] { return nil }

type hPosBody[P, V any] struct{}

func (hPosBody[P, V]) Len() int                     { return 0 }
func (hPosBody[P, V]) hPositions() iter.Seq[P]      { return emptySeq[P] }
func (hPosBody[P, V]) PositionSlice() []P           { return nil }
func (hPosBody[P, V]) hValues() iter.Seq[V]         { return emptySeq[V] }
func (hPosBody[P, V]) ValueSlice() []V              { return nil }
func (hPosBody[P, V]) At(P) V                       { var z V; return z }
func (hPosBody[P, V]) All() iter.Seq2[P, V]         { return nil }
func (hPosBody[P, V]) AllSlice() []hSlotValue[P, V] { return nil }

// --- ordered reads: these do NOT factor the same way ---

type hOrdKeys[K any] struct{}

func (hOrdKeys[K]) MinKey() (K, bool)            { var z K; return z, false }
func (hOrdKeys[K]) MaxKey() (K, bool)            { var z K; return z, false }
func (hOrdKeys[K]) FloorKey(K) (K, bool)         { var z K; return z, false }
func (hOrdKeys[K]) CeilKey(K) (K, bool)          { var z K; return z, false }
func (hOrdKeys[K]) RangeKeys(a, b K) iter.Seq[K] { return emptySeq[K] }

type hOrdPairs[K, V any] struct{}

func (hOrdPairs[K, V]) Min() (K, V, bool)            { var k K; var v V; return k, v, false }
func (hOrdPairs[K, V]) Max() (K, V, bool)            { var k K; var v V; return k, v, false }
func (hOrdPairs[K, V]) Floor(K) (K, V, bool)         { var k K; var v V; return k, v, false }
func (hOrdPairs[K, V]) Ceil(K) (K, V, bool)          { var k K; var v V; return k, v, false }
func (hOrdPairs[K, V]) Range(a, b K) iter.Seq2[K, V] { return nil }

// --- mutators, kept separate so views cannot satisfy them ---

type hKeysMut[K any] struct{}

func (hKeysMut[K]) Add(K)          {}
func (hKeysMut[K]) AddAll(...K)    {}
func (hKeysMut[K]) Delete(K)       {}
func (hKeysMut[K]) DeleteAll(...K) {}

type hKvMut[K, V any] struct{}

func (hKvMut[K, V]) Set(K, V)                   {}
func (hKvMut[K, V]) SetAll(...hSlotValue[K, V]) {}
func (hKvMut[K, V]) Delete(K)                   {}
func (hKvMut[K, V]) DeleteAll(...K)             {}

// --- containers ---

type hHashSet[T comparable] struct {
	hKeysBody[T]
	hKeysMut[T]
}

type hSortedSet[T cmp.Ordered] struct {
	hKeysBody[T]
	hOrdKeys[T]
	hKeysMut[T]
}

type hMap[K comparable, V any] struct {
	hKvBody[K, V]
	hKvMut[K, V]
}

type hSortedDict[K cmp.Ordered, V any] struct {
	hKvBody[K, V]
	hOrdKeys[K]     // MinKey MaxKey FloorKey CeilKey RangeKeys
	hOrdPairs[K, V] // Min Max Floor Ceil Range
	hKvMut[K, V]
}

type hVector[T any] struct{ hPosBody[int, T] }

// --- views ---

type hHashSetView[NT any] struct{ hKeysBody[NT] }

type hSortedSetView[T cmp.Ordered] struct {
	hKeysBody[T]
	hOrdKeys[T]
}

type hMapView[NK, NV any] struct{ hKvBody[NK, NV] }

type hSortedDictView[K cmp.Ordered, NV any] struct {
	hKvBody[K, NV]
	hOrdKeys[K]
	hOrdPairs[K, NV]
}

type hVectorView[NT any] struct{ hPosBody[int, NT] }
type hSliceView[NT any] struct{ hPosBody[int, NT] }

// ---------------------------------------------------------------------------
// The table, as assertions.
// ---------------------------------------------------------------------------

var (
	_ hKeys[int] = hHashSet[int]{}
	_ hKeys[int] = hSortedSet[int]{}
	_ hKeys[int] = hHashSetView[int]{}
	_ hKeys[int] = hSortedSetView[int]{}

	// A dict is a hKeys of its keys AND a hValues of its values.
	_ hKeys[string]           = hMap[string, int]{}
	_ hValues[int]            = hMap[string, int]{}
	_ hKeyValues[string, int] = hMap[string, int]{}
	_ hKeys[string]           = hSortedDict[string, int]{}
	_ hValues[int]            = hSortedDict[string, int]{}
	_ hKeyValues[string, int] = hSortedDict[string, int]{}
	_ hKeyValues[string, int] = hMapView[string, int]{}
	_ hKeyValues[string, int] = hSortedDictView[string, int]{}

	// The payoff: sequences share hValues with dicts.
	_ hValues[int]                 = hVector[int]{}
	_ hPositions[int]              = hVector[int]{}
	_ hPositionValues[int, int]    = hVector[int]{}
	_ hValues[string]              = hVectorView[string]{}
	_ hPositionValues[int, string] = hVectorView[string]{}
	_ hPositionValues[int, string] = hSliceView[string]{}
)

// oneVocabulary is the thing the shape names buy: a single function that reads
// values out of a dict, a vector, or a slice view.
func hSum(vs hValues[int]) int {
	t := 0
	for v := range vs.hValues() {
		t += v
	}
	return t
}

var (
	_ = hSum(hMap[string, int]{})
	_ = hSum(hSortedDict[string, int]{})
	_ = hSum(hVector[int]{})
	_ = hSum(hVectorView[int]{})
	_ = hSum(hSliceView[int]{})
)

func emptySeq[T any](yield func(T) bool)        {}
func emptySeq2[K, V any](yield func(K, V) bool) {}

// The ordered reads do not factor into one interface, because a sorted set's
// Min returns (K, bool) and a sorted dict's returns (K, V, bool). Two
// interfaces is the option that changes no shipped behaviour.

// OrderedKeys is satisfied by sorted SETS and sorted DICTS alike, because the
// key-only reads are spelled *Key and so do not collide with a dict's
// pair-returning Min/Max/Floor/Ceil/Range.
type hOrderedKeys[K any] interface {
	hKeys[K]
	MinKey() (K, bool)
	MaxKey() (K, bool)
	FloorKey(K) (K, bool)
	CeilKey(K) (K, bool)
	RangeKeys(lo, hi K) iter.Seq[K]
}

// OrderedKeyValues adds the pair-returning reads on top, so a sorted dict
// satisfies both tiers.
type hOrderedKeyValues[K, V any] interface {
	hKeyValues[K, V]
	hOrderedKeys[K]
	Min() (K, V, bool)
	Max() (K, V, bool)
	Floor(K) (K, V, bool)
	Ceil(K) (K, V, bool)
	Range(lo, hi K) iter.Seq2[K, V]
}

// Mutation tiers are DEFERRED as shape interfaces -- there are not yet enough
// mutable types to know what a unified setter vocabulary looks like. These are
// today's contracts, kept so the table is complete.

type hMutableKeys[K any] interface {
	hKeys[K]
	Add(K)
	AddAll(...K)
	Delete(K)
	DeleteAll(...K)
}

type hMutableKeyValues[K, V any] interface {
	hKeyValues[K, V]
	Set(K, V)
	SetAll(...hSlotValue[K, V])
	Delete(K)
	DeleteAll(...K)
}

// The complete table, as assertions.
var (
	_ hOrderedKeys[int]              = hSortedSet[int]{}
	_ hOrderedKeys[int]              = hSortedSetView[int]{}
	_ hOrderedKeyValues[string, int] = hSortedDict[string, int]{}
	_ hOrderedKeyValues[string, int] = hSortedDictView[string, int]{}

	_ hMutableKeys[int]              = hHashSet[int]{}
	_ hMutableKeys[int]              = hSortedSet[int]{}
	_ hMutableKeyValues[string, int] = hMap[string, int]{}
	_ hMutableKeyValues[string, int] = hSortedDict[string, int]{}
)
