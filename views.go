package containers

import (
	"cmp"
	"iter"
)

// A view is a read-only handle on a container, plus the conversions a viewer
// applies to what it hands back.
//
// # What a view is for
//
// Handing out a container by pointer lets the holder mutate it. Handing out a
// read-only contract does not help: a container satisfies those contracts
// structurally, so the holder can type-assert back and mutate anyway. A view is
// a distinct struct, so that assertion fails.
//
//	v := containers.ViewHashDict(d, releaseViewer{})
//	// v.Get takes and returns the view's own types; there is no path back to d.
//
// Every view is constructed explicitly. There is deliberately no View method:
// it would read as "give me a view" with no hint that key and value handling is
// a dimension at all. Where no conversion is wanted, say so:
//
//	v := containers.ViewHashDictIdentity(d)
//
// # What a view is not
//
//   - **Not a snapshot.** A projection wraps the same element, so a later write
//     to the container, or through any other reference, is visible through the
//     view. It denies writes *through the view*.
//   - **Not enforced against a determined caller.** Blocking the type assertion
//     leaves reflect plus unsafe, which is greppable and reviewable. A view moves
//     the bar from invisible to auditable.
//   - **Not automatic.** A container still satisfies the read contracts, so a
//     provider can pass the container instead. A view is applied at boundaries
//     the provider cares about.
//   - **Not deep.** The library cannot invent a read-only type for your keys or
//     values. A view over mutable types is shallow unless you supply a viewer
//     that closes it.
//
// # Ordered containers convert values only
//
// SortedSet and SortedDict key on cmp.Ordered, which admits only integers,
// floats and strings — all immutable — so their keys cannot be mutated through
// a view and need no conversion. SortedSetView therefore takes no viewer at all.
//
// # Cost
//
// A view carrying a viewer is two words and costs one allocation when passed as
// a contract interface. Read that against what a view replaces: a defensive copy
// costs ~10 ns at minimum, grows per element, and an accessor copying in a
// caller's loop is O(n²). See ADR 0011, ADR 0012 and experiments/views.
//
// Methods on a zero view panic, consistent with the rest of this package.

// ---------------------------------------------------------------------------
// HashSet
// ---------------------------------------------------------------------------

// A HashSetView is a read-only view of a HashSet, with elements converted by a
// viewer. Has takes and All yields the converted type, so a HashSetView
// satisfies Set[NT].
type HashSetView[T comparable, NT any] struct {
	s      *HashSet[T]
	viewer CanViewHashSet[T, NT]
}

// ViewHashSet returns a read-only view of s, converting elements through viewer.
func ViewHashSet[T comparable, NT any](s *HashSet[T], viewer CanViewHashSet[T, NT]) HashSetView[T, NT] {
	return HashSetView[T, NT]{s, viewer}
}

// ViewHashSetIdentity returns a read-only view of s that converts nothing.
func ViewHashSetIdentity[T comparable](s *HashSet[T]) HashSetView[T, T] {
	return HashSetView[T, T]{s, IdentityViewer[T, T]{}}
}

func (v HashSetView[T, NT]) Len() int { return v.s.Len() }

// Has reports whether nt is in the set. An element that does not convert back
// cannot be present, so it reads as a miss.
func (v HashSetView[T, NT]) Has(nt NT) bool {
	t, ok := v.viewer.FromKeyView(nt)
	return ok && v.s.Has(t)
}

func (v HashSetView[T, NT]) All() iter.Seq[NT] {
	seq, vw := v.s.All(), v.viewer // eager, so a zero view panics here
	return func(yield func(NT) bool) {
		for t := range seq {
			if !yield(vw.ToKeyView(t)) {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// SortedSet — no viewer: its elements are cmp.Ordered, hence immutable.
// ---------------------------------------------------------------------------

// A SortedSetView is a read-only view of a SortedSet.
//
// It converts nothing. A sorted set's elements are cmp.Ordered, which admits
// only immutable value types, so there is nothing a conversion would protect.
type SortedSetView[T cmp.Ordered] struct{ s *SortedSet[T] }

// ViewSortedSet returns a read-only view of s.
func ViewSortedSet[T cmp.Ordered](s *SortedSet[T]) SortedSetView[T] {
	return SortedSetView[T]{s}
}

func (v SortedSetView[T]) Len() int                   { return v.s.Len() }
func (v SortedSetView[T]) Has(t T) bool               { return v.s.Has(t) }
func (v SortedSetView[T]) All() iter.Seq[T]           { return v.s.All() }
func (v SortedSetView[T]) Range(lo, hi T) iter.Seq[T] { return v.s.Range(lo, hi) }
func (v SortedSetView[T]) Min() (T, bool)             { return v.s.Min() }
func (v SortedSetView[T]) Max() (T, bool)             { return v.s.Max() }
func (v SortedSetView[T]) Floor(t T) (T, bool)        { return v.s.Floor(t) }
func (v SortedSetView[T]) Ceil(t T) (T, bool)         { return v.s.Ceil(t) }

// ---------------------------------------------------------------------------
// HashDict
// ---------------------------------------------------------------------------

// A HashDictView is a read-only view of a HashDict, with keys and values
// converted by a viewer. It satisfies Dict[NK, NV], so the container's own key
// and value types never appear in its API.
type HashDictView[K comparable, V, NK, NV any] struct {
	d      *HashDict[K, V]
	viewer CanViewHashDict[K, NK, V, NV]
}

// ViewHashDict returns a read-only view of d, converting through viewer.
func ViewHashDict[K comparable, V, NK, NV any](d *HashDict[K, V], viewer CanViewHashDict[K, NK, V, NV]) HashDictView[K, V, NK, NV] {
	return HashDictView[K, V, NK, NV]{d, viewer}
}

// ViewHashDictIdentity returns a read-only view of d that converts nothing.
func ViewHashDictIdentity[K comparable, V any](d *HashDict[K, V]) HashDictView[K, V, K, V] {
	return HashDictView[K, V, K, V]{d, IdentityViewer[K, V]{}}
}

func (v HashDictView[K, V, NK, NV]) Len() int { return v.d.Len() }

// Get returns the value under nk. A key that does not convert back cannot be
// present, so it reads as a miss.
func (v HashDictView[K, V, NK, NV]) Get(nk NK) (NV, bool) {
	k, ok := v.viewer.FromKeyView(nk)
	if !ok {
		var zero NV
		return zero, false
	}
	raw, ok := v.d.Get(k)
	if !ok {
		var zero NV
		return zero, false
	}
	return v.viewer.ToValueView(raw), true
}

func (v HashDictView[K, V, NK, NV]) All() iter.Seq2[NK, NV] {
	seq, vw := v.d.All(), v.viewer // eager, so a zero view panics here
	return func(yield func(NK, NV) bool) {
		for k, raw := range seq {
			if !yield(vw.ToKeyView(k), vw.ToValueView(raw)) {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// SortedDict — values converted; keys pass through, being cmp.Ordered.
// ---------------------------------------------------------------------------

// A SortedDictView is a read-only view of a SortedDict, with values converted by
// a viewer. Keys are not converted: they are cmp.Ordered, hence immutable, so
// ordered lookups take and return the container's own key type and no question
// of order preservation arises.
type SortedDictView[K cmp.Ordered, V, NV any] struct {
	d      *SortedDict[K, V]
	viewer CanViewSortedDict[V, NV]
}

// ViewSortedDict returns a read-only view of d, converting values through viewer.
func ViewSortedDict[K cmp.Ordered, V, NV any](d *SortedDict[K, V], viewer CanViewSortedDict[V, NV]) SortedDictView[K, V, NV] {
	return SortedDictView[K, V, NV]{d, viewer}
}

// ViewSortedDictIdentity returns a read-only view of d that converts nothing.
func ViewSortedDictIdentity[K cmp.Ordered, V any](d *SortedDict[K, V]) SortedDictView[K, V, V] {
	return SortedDictView[K, V, V]{d, IdentityValueViewer[V]{}}
}

func (v SortedDictView[K, V, NV]) Len() int { return v.d.Len() }

func (v SortedDictView[K, V, NV]) Get(k K) (NV, bool) {
	raw, ok := v.d.Get(k)
	if !ok {
		var zero NV
		return zero, false
	}
	return v.viewer.ToValueView(raw), true
}

func (v SortedDictView[K, V, NV]) All() iter.Seq2[K, NV] {
	return projectValues(v.d.All(), v.viewer)
}

func (v SortedDictView[K, V, NV]) Range(lo, hi K) iter.Seq2[K, NV] {
	return projectValues(v.d.Range(lo, hi), v.viewer)
}

func (v SortedDictView[K, V, NV]) Min() (K, NV, bool) {
	k, raw, ok := v.d.Min()
	return projectPair(k, raw, ok, v.viewer)
}
func (v SortedDictView[K, V, NV]) Max() (K, NV, bool) {
	k, raw, ok := v.d.Max()
	return projectPair(k, raw, ok, v.viewer)
}
func (v SortedDictView[K, V, NV]) Floor(k K) (K, NV, bool) {
	fk, raw, ok := v.d.Floor(k)
	return projectPair(fk, raw, ok, v.viewer)
}
func (v SortedDictView[K, V, NV]) Ceil(k K) (K, NV, bool) {
	ck, raw, ok := v.d.Ceil(k)
	return projectPair(ck, raw, ok, v.viewer)
}

// ---------------------------------------------------------------------------
// Map
// ---------------------------------------------------------------------------

// A MapView is a read-only view of a Map, with keys and values converted by a
// viewer.
//
// Unlike the struct-backed containers, a Map is itself a map[K]V, so a caller
// holding the underlying map can mutate it regardless of any view. A view over a
// Map guards against a holder of the view, not against a holder of the map it
// wraps.
type MapView[K comparable, V, NK, NV any] struct {
	m      Map[K, V]
	viewer CanViewMap[K, NK, V, NV]
}

// ViewMap returns a read-only view of m, converting through viewer.
func ViewMap[K comparable, V, NK, NV any](m Map[K, V], viewer CanViewMap[K, NK, V, NV]) MapView[K, V, NK, NV] {
	return MapView[K, V, NK, NV]{m, viewer}
}

// ViewMapIdentity returns a read-only view of m that converts nothing.
func ViewMapIdentity[K comparable, V any](m Map[K, V]) MapView[K, V, K, V] {
	return MapView[K, V, K, V]{m, IdentityViewer[K, V]{}}
}

func (v MapView[K, V, NK, NV]) Len() int { return v.m.Len() }

func (v MapView[K, V, NK, NV]) Get(nk NK) (NV, bool) {
	k, ok := v.viewer.FromKeyView(nk)
	if !ok {
		var zero NV
		return zero, false
	}
	raw, ok := v.m.Get(k)
	if !ok {
		var zero NV
		return zero, false
	}
	return v.viewer.ToValueView(raw), true
}

func (v MapView[K, V, NK, NV]) All() iter.Seq2[NK, NV] {
	seq, vw := v.m.All(), v.viewer // eager, so a zero view panics here
	return func(yield func(NK, NV) bool) {
		for k, raw := range seq {
			if !yield(vw.ToKeyView(k), vw.ToValueView(raw)) {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// projectValues converts values and leaves keys alone. The sequence and viewer
// are read eagerly, so a zero view panics at the call rather than at iteration.
func projectValues[K, V, NV any](seq iter.Seq2[K, V], vw ValueViewer[V, NV]) iter.Seq2[K, NV] {
	if vw == nil {
		panic("containers: view has no viewer")
	}
	return func(yield func(K, NV) bool) {
		for k, raw := range seq {
			if !yield(k, vw.ToValueView(raw)) {
				return
			}
		}
	}
}

// projectPair converts the value of a (key, value, ok) result, leaving a miss as
// the zero NV.
func projectPair[K, V, NV any](k K, v V, ok bool, vw ValueViewer[V, NV]) (K, NV, bool) {
	if !ok {
		var zk K
		var zv NV
		return zk, zv, false
	}
	return k, vw.ToValueView(v), true
}
