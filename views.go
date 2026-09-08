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
// bare structural contract does not help: a container satisfies one, so the
// holder can assert back to it and mutate. A view is a *sealed interface* — its
// method set includes an unexported method that only this package's view types
// have — so a container cannot be passed where a view is expected, and a view
// cannot be asserted back to its container. Both failures are at compile time.
//
//	func render(v containers.DictView[string, ItemView]) error
//
//	render(containers.ViewHashDict(d, viewer))  // fine
//	render(d)                                   // does not compile
//
// Every view is constructed explicitly. There is deliberately no View method: it
// would read as "give me a view" with no hint that key and value handling is a
// dimension at all. Where no conversion is wanted, say so:
//
//	v := containers.ViewHashDictIdentity(d)
//
// # Five interfaces
//
// SetView and DictView are what an unordered container's view satisfies, and are
// what a boundary should usually name. SortedSetView and SortedDictView embed
// them and add the ordered reads, so a sorted view can be passed wherever the
// unordered one is wanted. A Map's view and a HashDict's view are the same type.
//
// VectorView stands alone. A sequence is neither: its reads are positional, so
// it cannot be a SetView, and giving it a DictView keyed by int would make All
// yield pairs, which collides with Elems[T].
//
// The concrete types behind these interfaces are unexported and may change.
//
// # What a view is not
//
//   - **Not a snapshot.** A conversion wraps the same element, so a later write
//     to the container, or through any other reference, is visible through the
//     view. It denies writes *through the view*.
//   - **Not enforced against a determined caller.** Sealing leaves reflect plus
//     unsafe, which is greppable and reviewable. A view moves the bar from
//     invisible to auditable.
//   - **Not automatic.** A boundary declared with an unsealed type — Elems2, say,
//     which containers also satisfy — accepts the container as happily as ever.
//     The seal binds where the boundary names it.
//   - **Not deep.** The library cannot invent a read-only type for your keys or
//     values. A view over mutable types is shallow unless you supply a viewer
//     that closes it.
//
// # Ordered containers convert values only
//
// SortedSet and SortedDict key on cmp.Ordered, which admits only integers,
// floats and strings — all immutable — so their keys need no conversion.
// SortedSetView therefore converts nothing at all. This is also what lets the
// ordered interfaces embed the unordered ones: a sorted dict view's Get takes K
// and its All yields iter.Seq2[K, NV], which is exactly DictView[K, NV].
//
// # Cost
//
// A view carrying a viewer costs one allocation, at construction, and nothing
// thereafter however many boundaries it crosses. A view that converts nothing —
// every Identity view, and every SortedSetView — is one word and costs nothing
// at all. Read either against what a view replaces: a defensive copy costs ~10
// ns at minimum, grows per element, and an accessor copying in a caller's loop
// is O(n²). See ADRs 0011, 0012 and 0013, and experiments/viewiface.
//
// A nil view panics when used, exactly as a zero container does. Nothing in this
// package tests a view for nil.

// SetView is a read-only view of a set, with elements converted by a viewer.
//
// Sealed: only this package's view types satisfy it, so a set cannot be passed
// as one.
type SetView[NT any] interface {
	Elems[NT]

	// Has reports whether an element is present. An element that does not
	// convert back to the container's own type cannot be, so it reads as a miss.
	Has(NT) bool

	sealedView()
}

// DictView is a read-only view of a key-value container, with keys and values
// converted by a viewer.
//
// A HashDict's view and a Map's view are both a DictView, and a SortedDictView
// is one too, so a consumer naming this type is not naming an implementation.
//
// Sealed: only this package's view types satisfy it.
type DictView[NK, NV any] interface {
	Elems2[NK, NV]

	// Get returns the value under a key. A key that does not convert back to the
	// container's own type cannot be present, so it reads as a miss.
	Get(NK) (NV, bool)

	sealedView()
}

// SortedSetView is a read-only view of a SortedSet: everything SetView offers,
// plus the ordered reads.
//
// It converts nothing. A sorted set's elements are cmp.Ordered, which admits
// only immutable value types, so there is nothing a conversion would protect.
type SortedSetView[T cmp.Ordered] interface {
	SetView[T]

	Range(lo, hi T) iter.Seq[T]
	Min() (T, bool)
	Max() (T, bool)
	Floor(T) (T, bool)
	Ceil(T) (T, bool)
}

// SortedDictView is a read-only view of a SortedDict: everything DictView offers,
// plus the ordered reads.
//
// Values are converted; keys are not, being cmp.Ordered and hence immutable. So
// the ordered reads take and return the container's own key type, and no question
// of order preservation arises.
type SortedDictView[K cmp.Ordered, NV any] interface {
	DictView[K, NV]

	Range(lo, hi K) iter.Seq2[K, NV]
	Min() (K, NV, bool)
	Max() (K, NV, bool)
	Floor(K) (K, NV, bool)
	Ceil(K) (K, NV, bool)
}

// VectorView is a read-only view of a Vector, with elements converted by a
// viewer.
//
// It is the answer to ADR 0001's accessor problem for sequences: a type holding
// a Vector field hands one of these out in O(1) and no allocation, where an
// accessor over a []T field must copy on every call or return a mutable
// interior.
//
// Indices are not converted — an index is a position the container assigned,
// not a key the caller supplied — so At takes an int on both sides.
//
// Sealed: only this package's view types satisfy it.
type VectorView[NT any] interface {
	Elems[NT]

	// At returns the element at i, converted. It panics if i is out of range.
	At(int) NT

	// AllIndexed iterates index and converted element together.
	AllIndexed() iter.Seq2[int, NT]

	sealedView()
}

// ---------------------------------------------------------------------------
// HashSet
// ---------------------------------------------------------------------------

type hashSetView[T comparable, NT any] struct {
	s      *HashSet[T]
	viewer CanViewHashSet[T, NT]
}

// ViewHashSet returns a read-only view of s, converting elements through viewer.
func ViewHashSet[T comparable, NT any](s *HashSet[T], viewer CanViewHashSet[T, NT]) SetView[NT] {
	return hashSetView[T, NT]{s, viewer}
}

func (v hashSetView[T, NT]) sealedView() {}
func (v hashSetView[T, NT]) Len() int    { return v.s.Len() }

func (v hashSetView[T, NT]) Has(nt NT) bool {
	t, ok := v.viewer.FromKeyView(nt)
	return ok && v.s.Has(t)
}

func (v hashSetView[T, NT]) All() iter.Seq[NT] {
	seq, vw := v.s.All(), v.viewer // eager, so a broken view panics here
	return func(yield func(NT) bool) {
		for t := range seq {
			if !yield(vw.ToKeyView(t)) {
				return
			}
		}
	}
}

// hashSetIdentityView converts nothing, so it needs no viewer and is one word.
type hashSetIdentityView[T comparable] struct{ s *HashSet[T] }

// ViewHashSetIdentity returns a read-only view of s that converts nothing.
func ViewHashSetIdentity[T comparable](s *HashSet[T]) SetView[T] {
	return hashSetIdentityView[T]{s}
}

func (v hashSetIdentityView[T]) sealedView()      {}
func (v hashSetIdentityView[T]) Len() int         { return v.s.Len() }
func (v hashSetIdentityView[T]) Has(t T) bool     { return v.s.Has(t) }
func (v hashSetIdentityView[T]) All() iter.Seq[T] { return v.s.All() }

// ---------------------------------------------------------------------------
// SortedSet — no viewer, so one word for every construction.
// ---------------------------------------------------------------------------

type sortedSetView[T cmp.Ordered] struct{ s *SortedSet[T] }

// ViewSortedSet returns a read-only view of s.
func ViewSortedSet[T cmp.Ordered](s *SortedSet[T]) SortedSetView[T] {
	return sortedSetView[T]{s}
}

func (v sortedSetView[T]) sealedView()                {}
func (v sortedSetView[T]) Len() int                   { return v.s.Len() }
func (v sortedSetView[T]) Has(t T) bool               { return v.s.Has(t) }
func (v sortedSetView[T]) All() iter.Seq[T]           { return v.s.All() }
func (v sortedSetView[T]) Range(lo, hi T) iter.Seq[T] { return v.s.Range(lo, hi) }
func (v sortedSetView[T]) Min() (T, bool)             { return v.s.Min() }
func (v sortedSetView[T]) Max() (T, bool)             { return v.s.Max() }
func (v sortedSetView[T]) Floor(t T) (T, bool)        { return v.s.Floor(t) }
func (v sortedSetView[T]) Ceil(t T) (T, bool)         { return v.s.Ceil(t) }

// ---------------------------------------------------------------------------
// HashDict
// ---------------------------------------------------------------------------

type hashDictView[K comparable, V, NK, NV any] struct {
	d      *HashDict[K, V]
	viewer CanViewHashDict[K, NK, V, NV]
}

// ViewHashDict returns a read-only view of d, converting through viewer.
func ViewHashDict[K comparable, V, NK, NV any](d *HashDict[K, V], viewer CanViewHashDict[K, NK, V, NV]) DictView[NK, NV] {
	return hashDictView[K, V, NK, NV]{d, viewer}
}

func (v hashDictView[K, V, NK, NV]) sealedView() {}
func (v hashDictView[K, V, NK, NV]) Len() int    { return v.d.Len() }

func (v hashDictView[K, V, NK, NV]) Get(nk NK) (NV, bool) {
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

func (v hashDictView[K, V, NK, NV]) All() iter.Seq2[NK, NV] {
	seq, vw := v.d.All(), v.viewer // eager, so a broken view panics here
	return func(yield func(NK, NV) bool) {
		for k, raw := range seq {
			if !yield(vw.ToKeyView(k), vw.ToValueView(raw)) {
				return
			}
		}
	}
}

type hashDictIdentityView[K comparable, V any] struct{ d *HashDict[K, V] }

// ViewHashDictIdentity returns a read-only view of d that converts nothing.
func ViewHashDictIdentity[K comparable, V any](d *HashDict[K, V]) DictView[K, V] {
	return hashDictIdentityView[K, V]{d}
}

func (v hashDictIdentityView[K, V]) sealedView()          {}
func (v hashDictIdentityView[K, V]) Len() int             { return v.d.Len() }
func (v hashDictIdentityView[K, V]) Get(k K) (V, bool)    { return v.d.Get(k) }
func (v hashDictIdentityView[K, V]) All() iter.Seq2[K, V] { return v.d.All() }

// ---------------------------------------------------------------------------
// SortedDict — values converted; keys pass through, being cmp.Ordered.
// ---------------------------------------------------------------------------

type sortedDictView[K cmp.Ordered, V, NV any] struct {
	d      *SortedDict[K, V]
	viewer CanViewSortedDict[V, NV]
}

// ViewSortedDict returns a read-only view of d, converting values through viewer.
func ViewSortedDict[K cmp.Ordered, V, NV any](d *SortedDict[K, V], viewer CanViewSortedDict[V, NV]) SortedDictView[K, NV] {
	return sortedDictView[K, V, NV]{d, viewer}
}

func (v sortedDictView[K, V, NV]) sealedView() {}
func (v sortedDictView[K, V, NV]) Len() int    { return v.d.Len() }

func (v sortedDictView[K, V, NV]) Get(k K) (NV, bool) {
	raw, ok := v.d.Get(k)
	if !ok {
		var zero NV
		return zero, false
	}
	return v.viewer.ToValueView(raw), true
}

func (v sortedDictView[K, V, NV]) All() iter.Seq2[K, NV] {
	return projectValues(v.d.All(), v.viewer)
}

func (v sortedDictView[K, V, NV]) Range(lo, hi K) iter.Seq2[K, NV] {
	return projectValues(v.d.Range(lo, hi), v.viewer)
}

func (v sortedDictView[K, V, NV]) Min() (K, NV, bool) {
	k, raw, ok := v.d.Min()
	return projectPair(k, raw, ok, v.viewer)
}
func (v sortedDictView[K, V, NV]) Max() (K, NV, bool) {
	k, raw, ok := v.d.Max()
	return projectPair(k, raw, ok, v.viewer)
}
func (v sortedDictView[K, V, NV]) Floor(k K) (K, NV, bool) {
	fk, raw, ok := v.d.Floor(k)
	return projectPair(fk, raw, ok, v.viewer)
}
func (v sortedDictView[K, V, NV]) Ceil(k K) (K, NV, bool) {
	ck, raw, ok := v.d.Ceil(k)
	return projectPair(ck, raw, ok, v.viewer)
}

type sortedDictIdentityView[K cmp.Ordered, V any] struct{ d *SortedDict[K, V] }

// ViewSortedDictIdentity returns a read-only view of d that converts nothing.
func ViewSortedDictIdentity[K cmp.Ordered, V any](d *SortedDict[K, V]) SortedDictView[K, V] {
	return sortedDictIdentityView[K, V]{d}
}

func (v sortedDictIdentityView[K, V]) sealedView()                    {}
func (v sortedDictIdentityView[K, V]) Len() int                       { return v.d.Len() }
func (v sortedDictIdentityView[K, V]) Get(k K) (V, bool)              { return v.d.Get(k) }
func (v sortedDictIdentityView[K, V]) All() iter.Seq2[K, V]           { return v.d.All() }
func (v sortedDictIdentityView[K, V]) Range(lo, hi K) iter.Seq2[K, V] { return v.d.Range(lo, hi) }
func (v sortedDictIdentityView[K, V]) Min() (K, V, bool)              { return v.d.Min() }
func (v sortedDictIdentityView[K, V]) Max() (K, V, bool)              { return v.d.Max() }
func (v sortedDictIdentityView[K, V]) Floor(k K) (K, V, bool)         { return v.d.Floor(k) }
func (v sortedDictIdentityView[K, V]) Ceil(k K) (K, V, bool)          { return v.d.Ceil(k) }

// ---------------------------------------------------------------------------
// Map
// ---------------------------------------------------------------------------

// A Map is itself a map[K]V, so a caller holding the underlying map can mutate
// it regardless of any view. A view over a Map guards against a holder of the
// view, not against a holder of the map it wraps.
type mapView[K comparable, V, NK, NV any] struct {
	m      Map[K, V]
	viewer CanViewMap[K, NK, V, NV]
}

// ViewMap returns a read-only view of m, converting through viewer.
func ViewMap[K comparable, V, NK, NV any](m Map[K, V], viewer CanViewMap[K, NK, V, NV]) DictView[NK, NV] {
	return mapView[K, V, NK, NV]{m, viewer}
}

func (v mapView[K, V, NK, NV]) sealedView() {}
func (v mapView[K, V, NK, NV]) Len() int    { return v.m.Len() }

func (v mapView[K, V, NK, NV]) Get(nk NK) (NV, bool) {
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

func (v mapView[K, V, NK, NV]) All() iter.Seq2[NK, NV] {
	seq, vw := v.m.All(), v.viewer // eager, so a broken view panics here
	return func(yield func(NK, NV) bool) {
		for k, raw := range seq {
			if !yield(vw.ToKeyView(k), vw.ToValueView(raw)) {
				return
			}
		}
	}
}

type mapIdentityView[K comparable, V any] struct{ m Map[K, V] }

// ViewMapIdentity returns a read-only view of m that converts nothing.
func ViewMapIdentity[K comparable, V any](m Map[K, V]) DictView[K, V] {
	return mapIdentityView[K, V]{m}
}

func (v mapIdentityView[K, V]) sealedView()          {}
func (v mapIdentityView[K, V]) Len() int             { return v.m.Len() }
func (v mapIdentityView[K, V]) Get(k K) (V, bool)    { return v.m.Get(k) }
func (v mapIdentityView[K, V]) All() iter.Seq2[K, V] { return v.m.All() }

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// projectValues converts values and leaves keys alone. The sequence and viewer
// are read eagerly, so a broken view panics at the call rather than at iteration.
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

// The hierarchy, asserted: an ordered view is usable wherever the unordered one
// is wanted. This holds because ADR 0012 made ordered views convert values only
// — a sorted dict view's Get takes K and its All yields iter.Seq2[K, NV], which
// is exactly DictView[K, NV]. Converting ordered keys would break it.
var (
	_ SetView[int]          = (SortedSetView[int])(nil)
	_ DictView[int, string] = (SortedDictView[int, string])(nil)
)

// ---------------------------------------------------------------------------
// Vector
// ---------------------------------------------------------------------------

type vectorView[T, NT any] struct {
	vec    *Vector[T]
	viewer CanViewVector[T, NT]
}

// ViewVector returns a read-only view of vec, converting elements through viewer.
func ViewVector[T, NT any](vec *Vector[T], viewer CanViewVector[T, NT]) VectorView[NT] {
	return vectorView[T, NT]{vec, viewer}
}

func (v vectorView[T, NT]) sealedView() {}
func (v vectorView[T, NT]) Len() int    { return v.vec.Len() }

func (v vectorView[T, NT]) At(i int) NT { return v.viewer.ToValueView(v.vec.At(i)) }

func (v vectorView[T, NT]) All() iter.Seq[NT] {
	seq, vw := v.vec.All(), v.viewer // eager, so a broken view panics here
	return func(yield func(NT) bool) {
		for e := range seq {
			if !yield(vw.ToValueView(e)) {
				return
			}
		}
	}
}

func (v vectorView[T, NT]) AllIndexed() iter.Seq2[int, NT] {
	seq, vw := v.vec.AllIndexed(), v.viewer // eager
	return func(yield func(int, NT) bool) {
		for i, e := range seq {
			if !yield(i, vw.ToValueView(e)) {
				return
			}
		}
	}
}

type vectorIdentityView[T any] struct{ vec *Vector[T] }

// ViewVectorIdentity returns a read-only view of vec that converts nothing.
func ViewVectorIdentity[T any](vec *Vector[T]) VectorView[T] {
	return vectorIdentityView[T]{vec}
}

func (v vectorIdentityView[T]) sealedView()                   {}
func (v vectorIdentityView[T]) Len() int                      { return v.vec.Len() }
func (v vectorIdentityView[T]) At(i int) T                    { return v.vec.At(i) }
func (v vectorIdentityView[T]) All() iter.Seq[T]              { return v.vec.All() }
func (v vectorIdentityView[T]) AllIndexed() iter.Seq2[int, T] { return v.vec.AllIndexed() }
