package containers

import (
	"cmp"
	"iter"
)

// A view is a read-only handle on a container, plus a projection applied to the
// elements it hands back.
//
// # What a view is for
//
// Handing out a container by pointer lets the holder mutate it. Handing out a
// read-only contract does not help: a container satisfies those contracts
// structurally, so the holder can type-assert back and mutate anyway. A view is
// a distinct struct, so that assertion fails.
//
//	v := containers.ViewHashDict(d, func(i *Item) ItemView { return ItemView{i} })
//	// v.Get returns ItemView, and there is no path from v back to d.
//
// Each container also has a View method returning the identity view, for the
// common case where the element type is not mutable through what it hands back:
//
//	v := d.View()   // HashDictView[K, V, V]
//
// # What a view is not
//
//   - **Not a snapshot.** The projection wraps the same element, so a later
//     write to the container, or through any other reference to an element, is
//     visible through the view. It denies writes *through the view*.
//   - **Not enforced against a determined caller.** Blocking the type assertion
//     leaves `reflect` plus `unsafe`, which is greppable and reviewable. A view
//     moves the bar from invisible to auditable.
//   - **Not automatic.** A container still satisfies the read contracts, so a
//     provider can pass the container instead. A view is applied at boundaries
//     the provider cares about.
//   - **Not deep.** The library cannot invent a read-only type for your
//     elements. A view over a mutable element type is shallow unless you supply
//     a projection that closes it.
//
// # Cost
//
// A view carries a pointer and a function, so it is two words and costs one
// allocation when passed as a contract interface — 12 ns, against 0.38 ns for a
// pointer-shaped alternative. That is the standing price of admitting stateful
// projections. Read it against what a view replaces: a defensive copy costs
// ~10 ns at minimum, grows per element, and an accessor copying in a caller's
// loop is O(n²) — 36 800x a view at n=16 384. See ADR 0011 and
// experiments/views.
//
// Every view is a value type with a usable-only-when-constructed zero value:
// methods on the zero view panic, consistent with the rest of this package.

// ---------------------------------------------------------------------------
// HashSet
// ---------------------------------------------------------------------------

// A HashSetView is a read-only view of a HashSet with a projection applied.
//
// Note that Has takes the set's own element type while All yields the projected
// one, so a projecting view satisfies Elems[R] but not Set[R]. An identity view
// satisfies both.
type HashSetView[T comparable, R any] struct {
	s *HashSet[T]
	f func(T) R
}

// ViewHashSet returns a read-only view of s projecting each element through f.
func ViewHashSet[T comparable, R any](s *HashSet[T], f func(T) R) HashSetView[T, R] {
	return HashSetView[T, R]{s, f}
}

// View returns a read-only view of s with no projection.
func (s *HashSet[T]) View() HashSetView[T, T] {
	return HashSetView[T, T]{s, identity[T]}
}

func (v HashSetView[T, R]) Len() int     { return v.s.Len() }
func (v HashSetView[T, R]) Has(t T) bool { return v.s.Has(t) }
func (v HashSetView[T, R]) All() iter.Seq[R] {
	return projectSeq(v.s.All(), v.f)
}

// ---------------------------------------------------------------------------
// SortedSet
// ---------------------------------------------------------------------------

// A SortedSetView is a read-only view of a SortedSet with a projection applied.
// Ordered lookups take the set's own element type and return the projected one.
type SortedSetView[T cmp.Ordered, R any] struct {
	s *SortedSet[T]
	f func(T) R
}

// ViewSortedSet returns a read-only view of s projecting each element through f.
func ViewSortedSet[T cmp.Ordered, R any](s *SortedSet[T], f func(T) R) SortedSetView[T, R] {
	return SortedSetView[T, R]{s, f}
}

// View returns a read-only view of s with no projection.
func (s *SortedSet[T]) View() SortedSetView[T, T] {
	return SortedSetView[T, T]{s, identity[T]}
}

func (v SortedSetView[T, R]) Len() int     { return v.s.Len() }
func (v SortedSetView[T, R]) Has(t T) bool { return v.s.Has(t) }
func (v SortedSetView[T, R]) All() iter.Seq[R] {
	return projectSeq(v.s.All(), v.f)
}
func (v SortedSetView[T, R]) Range(lo, hi T) iter.Seq[R] {
	return projectSeq(v.s.Range(lo, hi), v.f)
}
func (v SortedSetView[T, R]) Min() (R, bool) { t, ok := v.s.Min(); return project1(t, ok, v.f) }
func (v SortedSetView[T, R]) Max() (R, bool) { t, ok := v.s.Max(); return project1(t, ok, v.f) }
func (v SortedSetView[T, R]) Floor(t T) (R, bool) {
	e, ok := v.s.Floor(t)
	return project1(e, ok, v.f)
}
func (v SortedSetView[T, R]) Ceil(t T) (R, bool) {
	e, ok := v.s.Ceil(t)
	return project1(e, ok, v.f)
}

// ---------------------------------------------------------------------------
// HashDict
// ---------------------------------------------------------------------------

// A HashDictView is a read-only view of a HashDict with a projection applied to
// values. Keys are not projected, so a HashDictView satisfies Dict[K, R].
type HashDictView[K comparable, V, R any] struct {
	d *HashDict[K, V]
	f func(V) R
}

// ViewHashDict returns a read-only view of d projecting each value through f.
func ViewHashDict[K comparable, V, R any](d *HashDict[K, V], f func(V) R) HashDictView[K, V, R] {
	return HashDictView[K, V, R]{d, f}
}

// View returns a read-only view of d with no projection.
func (d *HashDict[K, V]) View() HashDictView[K, V, V] {
	return HashDictView[K, V, V]{d, identity[V]}
}

func (v HashDictView[K, V, R]) Len() int { return v.d.Len() }
func (v HashDictView[K, V, R]) Get(k K) (R, bool) {
	raw, ok := v.d.Get(k)
	return project1(raw, ok, v.f)
}
func (v HashDictView[K, V, R]) All() iter.Seq2[K, R] {
	return projectSeq2(v.d.All(), v.f)
}

// ---------------------------------------------------------------------------
// SortedDict
// ---------------------------------------------------------------------------

// A SortedDictView is a read-only view of a SortedDict with a projection applied
// to values. Keys are not projected, so it satisfies Dict[K, R].
type SortedDictView[K cmp.Ordered, V, R any] struct {
	d *SortedDict[K, V]
	f func(V) R
}

// ViewSortedDict returns a read-only view of d projecting each value through f.
func ViewSortedDict[K cmp.Ordered, V, R any](d *SortedDict[K, V], f func(V) R) SortedDictView[K, V, R] {
	return SortedDictView[K, V, R]{d, f}
}

// View returns a read-only view of d with no projection.
func (d *SortedDict[K, V]) View() SortedDictView[K, V, V] {
	return SortedDictView[K, V, V]{d, identity[V]}
}

func (v SortedDictView[K, V, R]) Len() int { return v.d.Len() }
func (v SortedDictView[K, V, R]) Get(k K) (R, bool) {
	raw, ok := v.d.Get(k)
	return project1(raw, ok, v.f)
}
func (v SortedDictView[K, V, R]) All() iter.Seq2[K, R] {
	return projectSeq2(v.d.All(), v.f)
}
func (v SortedDictView[K, V, R]) Range(lo, hi K) iter.Seq2[K, R] {
	return projectSeq2(v.d.Range(lo, hi), v.f)
}
func (v SortedDictView[K, V, R]) Min() (K, R, bool) {
	k, raw, ok := v.d.Min()
	return project2(k, raw, ok, v.f)
}
func (v SortedDictView[K, V, R]) Max() (K, R, bool) {
	k, raw, ok := v.d.Max()
	return project2(k, raw, ok, v.f)
}
func (v SortedDictView[K, V, R]) Floor(k K) (K, R, bool) {
	fk, raw, ok := v.d.Floor(k)
	return project2(fk, raw, ok, v.f)
}
func (v SortedDictView[K, V, R]) Ceil(k K) (K, R, bool) {
	ck, raw, ok := v.d.Ceil(k)
	return project2(ck, raw, ok, v.f)
}

// ---------------------------------------------------------------------------
// Map
// ---------------------------------------------------------------------------

// A MapView is a read-only view of a Map with a projection applied to values.
//
// Unlike the struct-backed containers, a Map is itself a map[K]V, so a caller
// holding the underlying map can mutate it regardless of any view. A view over
// a Map guards against a holder of the view, not against a holder of the map it
// wraps.
type MapView[K comparable, V, R any] struct {
	m Map[K, V]
	f func(V) R
}

// ViewMap returns a read-only view of m projecting each value through f.
func ViewMap[K comparable, V, R any](m Map[K, V], f func(V) R) MapView[K, V, R] {
	return MapView[K, V, R]{m, f}
}

// View returns a read-only view of m with no projection.
func (m Map[K, V]) View() MapView[K, V, V] {
	return MapView[K, V, V]{m, identity[V]}
}

func (v MapView[K, V, R]) Len() int { return v.m.Len() }
func (v MapView[K, V, R]) Get(k K) (R, bool) {
	raw, ok := v.m.Get(k)
	return project1(raw, ok, v.f)
}
func (v MapView[K, V, R]) All() iter.Seq2[K, R] {
	return projectSeq2(v.m.All(), v.f)
}

// ---------------------------------------------------------------------------
// projection helpers
// ---------------------------------------------------------------------------

func identity[T any](t T) T { return t }

// project1 maps a (value, ok) result through f, leaving a miss as the zero R.
// It takes f as an argument rather than currying, because Go cannot infer R
// from a call whose arguments do not mention it.
func project1[V, R any](v V, ok bool, f func(V) R) (R, bool) {
	if !ok {
		var zero R
		return zero, false
	}
	return f(v), true
}

// project2 is project1 for a (key, value, ok) result. The key is not projected.
func project2[K, V, R any](k K, v V, ok bool, f func(V) R) (K, R, bool) {
	if !ok {
		var zk K
		var zr R
		return zk, zr, false
	}
	return k, f(v), true
}

func projectSeq[T, R any](seq iter.Seq[T], f func(T) R) iter.Seq[R] {
	return func(yield func(R) bool) {
		for t := range seq {
			if !yield(f(t)) {
				return
			}
		}
	}
}

func projectSeq2[K, V, R any](seq iter.Seq2[K, V], f func(V) R) iter.Seq2[K, R] {
	return func(yield func(K, R) bool) {
		for k, v := range seq {
			if !yield(k, f(v)) {
				return
			}
		}
	}
}
