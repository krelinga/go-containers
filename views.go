package containers

import (
	"cmp"
	"iter"
	"slices"
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
// IndexedView stands alone. A sequence is neither: its reads are positional, so
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
	Len() int

	// Keys iterates the elements, converted. A set's element is its key
	// (ADR 0017).
	Keys() iter.Seq[NT]

	// KeySlice returns the elements as a new slice. The result is a full,
	// independent copy, so it hands out nothing the view protects.
	KeySlice() []NT

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
	Len() int

	// Keys, Values and All iterate the converted entries.
	Keys() iter.Seq[NK]
	Values() iter.Seq[NV]
	All() iter.Seq2[NK, NV]

	// KeySlice, ValueSlice and AllSlice materialise them. Each result is a
	// full, independent copy, so none hands out anything the view protects.
	KeySlice() []NK
	ValueSlice() []NV
	AllSlice() []KeyValue[NK, NV]

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

// IndexedView is a read-only view of a Vector, with elements converted by a
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
type IndexedView[NT any] interface {
	Len() int

	// Values iterates the elements, converted; All pairs each with its index.
	Values() iter.Seq[NT]
	All() iter.Seq2[int, NT]

	// ValueSlice and AllSlice materialise them, each as a full, independent
	// copy.
	ValueSlice() []NT
	AllSlice() []KeyValue[int, NT]

	// At returns the element at i, converted. It panics if i is out of range.
	At(int) NT

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

func (v hashSetView[T, NT]) Keys() iter.Seq[NT] {
	seq, vw := v.s.Keys(), v.viewer // eager, so a broken view panics here
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

func (v hashSetIdentityView[T]) sealedView()       {}
func (v hashSetIdentityView[T]) Len() int          { return v.s.Len() }
func (v hashSetIdentityView[T]) Has(t T) bool      { return v.s.Has(t) }
func (v hashSetIdentityView[T]) Keys() iter.Seq[T] { return v.s.Keys() }

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
func (v sortedSetView[T]) Keys() iter.Seq[T]          { return v.s.Keys() }
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
// Slice
// ---------------------------------------------------------------------------
//
// A view over a plain slice, which is the only way to hand one out read-only
// without copying it. See ADR 0016.
//
// # It views a slice, not a variable
//
// The constructor takes []T rather than *[]T, because a slice is a value: Go's
// append returns a value the caller must rebind, and a reallocated slice is a
// new slice, not the same one. ADR 0015 exists because of that — a Vector is
// what gives a sequence identity across reallocation.
//
// So the view holds the slice it was given, and two things follow. A write to an
// element is visible through it, because it wraps that element — the same rule
// every other view in this package follows. An **append is not**, because an
// append produces a different slice value, which this view was never given:
//
//	hosts := []string{"a"}
//	v := containers.ViewSliceIdentity(hosts)
//	hosts[0] = "z"                  // v.At(0) == "z"
//	hosts = append(hosts, "b")      // v.Len() == 1, still
//
// If you want a sequence whose growth is visible to everyone holding it, that is
// what Vector is for.
//
// # Cost
//
// One allocation per view, unlike every other view here. A slice header is three
// words, so it is not pointer-shaped and boxes into the interface. That is the
// price of viewing a value rather than a container, and it is unavoidable:
// taking the address of the parameter escapes and allocates too.
//
// A nil slice makes a valid, empty view, exactly as a nil slice is a valid empty
// slice.

type sliceView[T, NT any] struct {
	s      []T
	viewer CanViewSlice[T, NT]
}

// ViewSlice returns a read-only view of s, converting elements through viewer.
func ViewSlice[T, NT any](s []T, viewer CanViewSlice[T, NT]) IndexedView[NT] {
	return sliceView[T, NT]{s, viewer}
}

func (v sliceView[T, NT]) sealedView() {}
func (v sliceView[T, NT]) Len() int    { return len(v.s) }

func (v sliceView[T, NT]) At(i int) NT { return v.viewer.ToValueView(v.s[i]) }

func (v sliceView[T, NT]) Values() iter.Seq[NT] {
	es, vw := v.s, v.viewer
	if vw == nil {
		panic("containers: view has no viewer")
	}
	return func(yield func(NT) bool) {
		for _, e := range es {
			if !yield(vw.ToValueView(e)) {
				return
			}
		}
	}
}

func (v sliceView[T, NT]) All() iter.Seq2[int, NT] {
	es, vw := v.s, v.viewer
	if vw == nil {
		panic("containers: view has no viewer")
	}
	return func(yield func(int, NT) bool) {
		for i, e := range es {
			if !yield(i, vw.ToValueView(e)) {
				return
			}
		}
	}
}

type sliceIdentityView[T any] struct{ s []T }

// ViewSliceIdentity returns a read-only view of s that converts nothing.
func ViewSliceIdentity[T any](s []T) IndexedView[T] {
	return sliceIdentityView[T]{s}
}

func (v sliceIdentityView[T]) sealedView() {}
func (v sliceIdentityView[T]) Len() int    { return len(v.s) }
func (v sliceIdentityView[T]) At(i int) T  { return v.s[i] }

func (v sliceIdentityView[T]) Values() iter.Seq[T] {
	es := v.s
	return func(yield func(T) bool) {
		for _, e := range es {
			if !yield(e) {
				return
			}
		}
	}
}

func (v sliceIdentityView[T]) All() iter.Seq2[int, T] {
	es := v.s
	return func(yield func(int, T) bool) {
		for i, e := range es {
			if !yield(i, e) {
				return
			}
		}
	}
}

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
func ViewVector[T, NT any](vec *Vector[T], viewer CanViewVector[T, NT]) IndexedView[NT] {
	return vectorView[T, NT]{vec, viewer}
}

func (v vectorView[T, NT]) sealedView() {}
func (v vectorView[T, NT]) Len() int    { return v.vec.Len() }

func (v vectorView[T, NT]) At(i int) NT { return v.viewer.ToValueView(v.vec.At(i)) }

func (v vectorView[T, NT]) Values() iter.Seq[NT] {
	seq, vw := v.vec.Values(), v.viewer // eager, so a broken view panics here
	return func(yield func(NT) bool) {
		for e := range seq {
			if !yield(vw.ToValueView(e)) {
				return
			}
		}
	}
}

func (v vectorView[T, NT]) All() iter.Seq2[int, NT] {
	seq, vw := v.vec.All(), v.viewer // eager
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
func ViewVectorIdentity[T any](vec *Vector[T]) IndexedView[T] {
	return vectorIdentityView[T]{vec}
}

func (v vectorIdentityView[T]) sealedView()            {}
func (v vectorIdentityView[T]) Len() int               { return v.vec.Len() }
func (v vectorIdentityView[T]) At(i int) T             { return v.vec.At(i) }
func (v vectorIdentityView[T]) Values() iter.Seq[T]    { return v.vec.Values() }
func (v vectorIdentityView[T]) All() iter.Seq2[int, T] { return v.vec.All() }

// ---------------------------------------------------------------------------
// Materialisers
//
// Every view offers the same *Slice methods its container does (ADR 0017),
// because a container has to be constructible from a view -- a view is the
// read-only boundary a caller is meant to pass around, and without these it
// would be a dead end.
//
// Handing out a slice costs the view nothing, because the result is a full,
// independent copy. A converting view applies its viewer once per element while
// materialising, exactly as its iterator does.
// ---------------------------------------------------------------------------

func sliceOfSeq[T any](seq iter.Seq[T], n int) []T {
	out := make([]T, 0, max(0, n))
	for v := range seq {
		out = append(out, v)
	}
	return out
}

func keysOfSeq2[K, V any](seq iter.Seq2[K, V], n int) []K {
	out := make([]K, 0, max(0, n))
	for k := range seq {
		out = append(out, k)
	}
	return out
}

func valuesOfSeq2[K, V any](seq iter.Seq2[K, V], n int) []V {
	out := make([]V, 0, max(0, n))
	for _, v := range seq {
		out = append(out, v)
	}
	return out
}

func pairsOfSeq2[K, V any](seq iter.Seq2[K, V], n int) []KeyValue[K, V] {
	out := make([]KeyValue[K, V], 0, max(0, n))
	for k, v := range seq {
		out = append(out, KeyValue[K, V]{k, v})
	}
	return out
}

func keySeqOf[K, V any](seq iter.Seq2[K, V]) iter.Seq[K] {
	return func(yield func(K) bool) {
		for k := range seq {
			if !yield(k) {
				return
			}
		}
	}
}

func valueSeqOf[K, V any](seq iter.Seq2[K, V]) iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, v := range seq {
			if !yield(v) {
				return
			}
		}
	}
}

func (v hashSetView[T, NT]) KeySlice() []NT    { return sliceOfSeq(v.Keys(), v.Len()) }
func (v hashSetIdentityView[T]) KeySlice() []T { return v.s.KeySlice() }
func (v sortedSetView[T]) KeySlice() []T       { return v.s.KeySlice() }

func (v hashDictView[K, V, NK, NV]) Keys() iter.Seq[NK]   { return keySeqOf(v.All()) }
func (v hashDictView[K, V, NK, NV]) Values() iter.Seq[NV] { return valueSeqOf(v.All()) }
func (v hashDictView[K, V, NK, NV]) KeySlice() []NK       { return keysOfSeq2(v.All(), v.Len()) }
func (v hashDictView[K, V, NK, NV]) ValueSlice() []NV     { return valuesOfSeq2(v.All(), v.Len()) }
func (v hashDictView[K, V, NK, NV]) AllSlice() []KeyValue[NK, NV] {
	return pairsOfSeq2(v.All(), v.Len())
}

func (v hashDictIdentityView[K, V]) Keys() iter.Seq[K]          { return v.d.Keys() }
func (v hashDictIdentityView[K, V]) Values() iter.Seq[V]        { return v.d.Values() }
func (v hashDictIdentityView[K, V]) KeySlice() []K              { return v.d.KeySlice() }
func (v hashDictIdentityView[K, V]) ValueSlice() []V            { return v.d.ValueSlice() }
func (v hashDictIdentityView[K, V]) AllSlice() []KeyValue[K, V] { return v.d.AllSlice() }

func (v sortedDictView[K, V, NV]) Keys() iter.Seq[K]    { return v.d.Keys() }
func (v sortedDictView[K, V, NV]) Values() iter.Seq[NV] { return valueSeqOf(v.All()) }
func (v sortedDictView[K, V, NV]) KeySlice() []K        { return v.d.KeySlice() }
func (v sortedDictView[K, V, NV]) ValueSlice() []NV     { return valuesOfSeq2(v.All(), v.Len()) }
func (v sortedDictView[K, V, NV]) AllSlice() []KeyValue[K, NV] {
	return pairsOfSeq2(v.All(), v.Len())
}

func (v sortedDictIdentityView[K, V]) Keys() iter.Seq[K]          { return v.d.Keys() }
func (v sortedDictIdentityView[K, V]) Values() iter.Seq[V]        { return v.d.Values() }
func (v sortedDictIdentityView[K, V]) KeySlice() []K              { return v.d.KeySlice() }
func (v sortedDictIdentityView[K, V]) ValueSlice() []V            { return v.d.ValueSlice() }
func (v sortedDictIdentityView[K, V]) AllSlice() []KeyValue[K, V] { return v.d.AllSlice() }

func (v mapView[K, V, NK, NV]) Keys() iter.Seq[NK]   { return keySeqOf(v.All()) }
func (v mapView[K, V, NK, NV]) Values() iter.Seq[NV] { return valueSeqOf(v.All()) }
func (v mapView[K, V, NK, NV]) KeySlice() []NK       { return keysOfSeq2(v.All(), v.Len()) }
func (v mapView[K, V, NK, NV]) ValueSlice() []NV     { return valuesOfSeq2(v.All(), v.Len()) }
func (v mapView[K, V, NK, NV]) AllSlice() []KeyValue[NK, NV] {
	return pairsOfSeq2(v.All(), v.Len())
}

func (v mapIdentityView[K, V]) Keys() iter.Seq[K]          { return v.m.Keys() }
func (v mapIdentityView[K, V]) Values() iter.Seq[V]        { return v.m.Values() }
func (v mapIdentityView[K, V]) KeySlice() []K              { return v.m.KeySlice() }
func (v mapIdentityView[K, V]) ValueSlice() []V            { return v.m.ValueSlice() }
func (v mapIdentityView[K, V]) AllSlice() []KeyValue[K, V] { return v.m.AllSlice() }

func (v sliceView[T, NT]) ValueSlice() []NT { return sliceOfSeq(v.Values(), v.Len()) }
func (v sliceView[T, NT]) AllSlice() []KeyValue[int, NT] {
	return pairsOfSeq2(v.All(), v.Len())
}

func (v sliceIdentityView[T]) ValueSlice() []T { return slices.Clone(v.s) }
func (v sliceIdentityView[T]) AllSlice() []KeyValue[int, T] {
	return pairsOfSeq2(v.All(), v.Len())
}

func (v vectorView[T, NT]) ValueSlice() []NT { return sliceOfSeq(v.Values(), v.Len()) }
func (v vectorView[T, NT]) AllSlice() []KeyValue[int, NT] {
	return pairsOfSeq2(v.All(), v.Len())
}

func (v vectorIdentityView[T]) ValueSlice() []T              { return v.vec.ValueSlice() }
func (v vectorIdentityView[T]) AllSlice() []KeyValue[int, T] { return v.vec.AllSlice() }
