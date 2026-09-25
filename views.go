package containers

import (
	"cmp"
	"iter"
	"slices"
)

// A view is a read-only handle on a container, plus the conversions a viewer
// applies to what it hands back.
//
// # Two tiers, two jobs
//
// Under ADR 0018 a view is a CONCRETE struct, one per container, and that is
// what carries the read-only guarantee: there is no interface to assert back
// through, and the struct's only field is unexported, so nothing outside this
// package can build one populated or reach the container inside it.
//
// The shape interfaces in contracts.go carry the other job, generality:
//
//	func handOut() MapSetView[string]   // nothing can write through this
//	func audit(k Keys[string])          // reads, from any container kind
//	audit(s.View())                     // ...and a CONTAINER does not fit
//
// Keys and its siblings are SEALED (ADR 0022): each declares an unexported
// callViewFirst that only a view implements, so passing a container where reads
// are promised is a compile error and the caller writes c.View(). The unexported
// inner* mirrors in contracts.go are the unsealed copies the plumbing needs,
// because the struct below holds one.
//
// # A view is two words, and boxing a wider one allocates
//
// Every view here is struct{ impl <inner tier> }, and an interface field is two
// words -- 16 B, measured. The exception is SortedSetView, which holds its
// SortedSet directly and is 8 B, because its keys are cmp.Ordered and never
// convert, so there is no type parameter to erase.
//
// The rule that matters is not the count but the consequence: a one-word value is
// pointer-shaped and boxes into an interface for free, while anything wider
// allocates on EVERY crossing -- measured at 13.59 ns and one allocation against
// 0.88 ns and none (ADR 0018). Two words is already past that line, which is why
// passing a view into a shape interface costs an allocation where passing a
// container does not (ADR 0022 records the numbers, and 0023 the alternative).
//
// **Do not add a second field to a view struct.** Two words box at one
// allocation; three words box at one allocation and copy more, and the struct
// stops being a candidate for ever becoming pointer-shaped again.
//
// # Zero values
//
// A view's zero value reads as empty, matching its container's (ADR 0018). The
// nil check sits inside any closure a method returns, and hands yield straight
// to the inner walk rather than re-yielding: checking before the closure, or
// ranging and re-yielding, each puts a closure on the heap.

// ---------------------------------------------------------------------------
// MapSet
// ---------------------------------------------------------------------------

// MapSetView is a read-only view of a MapSet, with keys converted by a viewer.
type MapSetView[NT any] struct{ impl innerKeys[NT] }

// ViewMapSetWith returns a view of v whose keys are converted by viewer.
//
// The source is a view, so views compose: pass s.View() to convert a container,
// or an existing MapSetView to narrow one further (ADR 0023). Each layer costs
// ~160 ns and 3 allocations per iteration call, and nothing per element.
func ViewMapSetWith[T, NT any](v MapSetView[T], viewer CanViewMapSet[T, NT]) MapSetView[NT] {
	return MapSetView[NT]{impl: convertedKeys[T, NT]{v, viewer}}
}

// View returns a read-only view of s that converts nothing.
//
// It is free: the container is one word, and a one-word value boxes into the
// view's interface field without allocating (0.39 ns, 0 allocs --
// experiments/sealing). It replaced ViewMapSetIdentity in ADR 0022.
//
// The view is read-only in STRUCTURE: nothing can be added, removed or replaced
// through it, and nothing can assert it back to the container. If T is a pointer
// or contains one, the values it yields remain mutable -- Go cannot express
// otherwise. Use ViewMapSet with a key viewer when that matters (ADR 0012).
func (s MapSet[T]) View() MapSetView[T] {
	return MapSetView[T]{impl: s}
}

func (v MapSetView[NT]) IsZero() bool { return v.impl == nil }

func (v MapSetView[NT]) Len() int {
	if v.impl == nil {
		return 0
	}
	return v.impl.Len()
}

func (v MapSetView[NT]) Has(nt NT) bool {
	if v.impl == nil {
		return false
	}
	return v.impl.Has(nt)
}

func (v MapSetView[NT]) Keys() iter.Seq[NT] {
	impl := v.impl
	return func(yield func(NT) bool) {
		if impl == nil {
			return
		}
		impl.Keys()(yield)
	}
}

func (v MapSetView[NT]) KeySlice() []NT {
	if v.impl == nil {
		return nil
	}
	return v.impl.KeySlice()
}

// convertedKeys adapts a MapSetView through a key viewer (ADR 0023).
//
// The source is a VIEW, not the container: that is what makes views composable,
// and it costs nothing per element -- the extra hop is one inlinable static call
// when the iterator is obtained, not a range-over-func layer. T is `any` rather
// than `comparable` for the same reason: membership goes through the view's Has
// rather than a map lookup here.
type convertedKeys[T, NT any] struct {
	src    MapSetView[T]
	viewer CanViewMapSet[T, NT]
}

func (c convertedKeys[T, NT]) Len() int { return c.src.Len() }

func (c convertedKeys[T, NT]) Has(nt NT) bool {
	t, ok := c.viewer.FromKeyView(nt)
	return ok && c.src.Has(t)
}

func (c convertedKeys[T, NT]) Keys() iter.Seq[NT] {
	src, vw := c.src, c.viewer
	return func(yield func(NT) bool) {
		for t := range src.Keys() {
			if !yield(vw.ToKeyView(t)) {
				return
			}
		}
	}
}

func (c convertedKeys[T, NT]) KeySlice() []NT {
	out := make([]NT, 0, c.src.Len())
	for t := range c.src.Keys() {
		out = append(out, c.viewer.ToKeyView(t))
	}
	return out
}

// ---------------------------------------------------------------------------
// SortedSet -- keys are cmp.Ordered, so there is nothing to convert (ADR 0012)
// ---------------------------------------------------------------------------

// SortedSetView is a read-only view of a SortedSet.
type SortedSetView[T cmp.Ordered] struct{ s SortedSet[T] }

// View returns a read-only view of s. There is no converting form: a SortedSet
// keys on cmp.Ordered, which admits only immutable value types, so its keys need
// no protection (ADR 0012) -- and for the same reason its values need no
// aliasing caveat, unlike the other containers' View.
//
// It replaced ViewSortedSet in ADR 0022.
func (s SortedSet[T]) View() SortedSetView[T] {
	return SortedSetView[T]{s: s}
}

// SortedSetView holds the SortedSet directly rather than an interface, because
// there is nothing to erase: keys are cmp.Ordered and pass through unchanged.
// That makes it one word, and it needs no nil checks of its own -- a zero
// SortedSet already reads as empty.

func (v SortedSetView[T]) IsZero() bool { return v.s.IsZero() }

func (v SortedSetView[T]) Len() int                       { return v.s.Len() }
func (v SortedSetView[T]) Has(t T) bool                   { return v.s.Has(t) }
func (v SortedSetView[T]) Keys() iter.Seq[T]              { return v.s.Keys() }
func (v SortedSetView[T]) KeySlice() []T                  { return v.s.KeySlice() }
func (v SortedSetView[T]) MinKey() (T, bool)              { return v.s.MinKey() }
func (v SortedSetView[T]) MaxKey() (T, bool)              { return v.s.MaxKey() }
func (v SortedSetView[T]) FloorKey(t T) (T, bool)         { return v.s.FloorKey(t) }
func (v SortedSetView[T]) CeilKey(t T) (T, bool)          { return v.s.CeilKey(t) }
func (v SortedSetView[T]) RangeKeys(lo, hi T) iter.Seq[T] { return v.s.RangeKeys(lo, hi) }

// ---------------------------------------------------------------------------
// Map
// ---------------------------------------------------------------------------

// MapView is a read-only view of a Map, with keys and values converted.
type MapView[NK, NV any] struct{ impl innerKeyValues[NK, NV] }

// ViewMapWith returns a view of v whose entries are converted by viewer.
//
// The source is a view, so views compose (ADR 0023): pass m.View() to convert a
// container, or an existing MapView to narrow one further.
func ViewMapWith[K, V, NK, NV any](v MapView[K, V], viewer CanViewMap[K, NK, V, NV]) MapView[NK, NV] {
	return MapView[NK, NV]{impl: convertedPairs[K, V, NK, NV]{v, viewer}}
}

// View returns a read-only view of m that converts nothing. Free, as
// MapSet.View is. It replaced ViewMapIdentity in ADR 0022.
//
// Read-only in STRUCTURE only: if K or V is a pointer or contains one, what the
// view yields stays mutable. Use ViewMap with a viewer when that matters.
func (m Map[K, V]) View() MapView[K, V] {
	return MapView[K, V]{impl: m}
}

func (v MapView[NK, NV]) IsZero() bool { return v.impl == nil }

func (v MapView[NK, NV]) Len() int {
	if v.impl == nil {
		return 0
	}
	return v.impl.Len()
}

func (v MapView[NK, NV]) Has(nk NK) bool {
	if v.impl == nil {
		return false
	}
	return v.impl.Has(nk)
}

func (v MapView[NK, NV]) Get(nk NK) (NV, bool) {
	if v.impl == nil {
		var z NV
		return z, false
	}
	return v.impl.Get(nk)
}

func (v MapView[NK, NV]) Keys() iter.Seq[NK] {
	impl := v.impl
	return func(yield func(NK) bool) {
		if impl == nil {
			return
		}
		impl.Keys()(yield)
	}
}

func (v MapView[NK, NV]) Values() iter.Seq[NV] {
	impl := v.impl
	return func(yield func(NV) bool) {
		if impl == nil {
			return
		}
		impl.Values()(yield)
	}
}

func (v MapView[NK, NV]) All() iter.Seq2[NK, NV] {
	impl := v.impl
	return func(yield func(NK, NV) bool) {
		if impl == nil {
			return
		}
		impl.All()(yield)
	}
}

func (v MapView[NK, NV]) KeySlice() []NK {
	if v.impl == nil {
		return nil
	}
	return v.impl.KeySlice()
}

func (v MapView[NK, NV]) ValueSlice() []NV {
	if v.impl == nil {
		return nil
	}
	return v.impl.ValueSlice()
}

func (v MapView[NK, NV]) AllSlice() []Entry[NK, NV] {
	if v.impl == nil {
		return nil
	}
	return v.impl.AllSlice()
}

// convertedPairs adapts a MapView through a key/value viewer (ADR 0023). The
// source is a view, so these compose; see convertedKeys for why K is `any`.
type convertedPairs[K, V, NK, NV any] struct {
	src    MapView[K, V]
	viewer CanViewMap[K, NK, V, NV]
}

func (c convertedPairs[K, V, NK, NV]) Len() int { return c.src.Len() }

func (c convertedPairs[K, V, NK, NV]) Has(nk NK) bool {
	k, ok := c.viewer.FromKeyView(nk)
	return ok && c.src.Has(k)
}

func (c convertedPairs[K, V, NK, NV]) Get(nk NK) (NV, bool) {
	k, ok := c.viewer.FromKeyView(nk)
	if !ok {
		var z NV
		return z, false
	}
	v, ok := c.src.Get(k)
	if !ok {
		var z NV
		return z, false
	}
	return c.viewer.ToValueView(v), true
}

func (c convertedPairs[K, V, NK, NV]) Keys() iter.Seq[NK] {
	src, vw := c.src, c.viewer
	return func(yield func(NK) bool) {
		for k := range src.Keys() {
			if !yield(vw.ToKeyView(k)) {
				return
			}
		}
	}
}

func (c convertedPairs[K, V, NK, NV]) Values() iter.Seq[NV] {
	src, vw := c.src, c.viewer
	return func(yield func(NV) bool) {
		for v := range src.Values() {
			if !yield(vw.ToValueView(v)) {
				return
			}
		}
	}
}

func (c convertedPairs[K, V, NK, NV]) All() iter.Seq2[NK, NV] {
	src, vw := c.src, c.viewer
	return func(yield func(NK, NV) bool) {
		for k, v := range src.All() {
			if !yield(vw.ToKeyView(k), vw.ToValueView(v)) {
				return
			}
		}
	}
}

func (c convertedPairs[K, V, NK, NV]) KeySlice() []NK {
	out := make([]NK, 0, c.src.Len())
	for k := range c.src.Keys() {
		out = append(out, c.viewer.ToKeyView(k))
	}
	return out
}

func (c convertedPairs[K, V, NK, NV]) ValueSlice() []NV {
	out := make([]NV, 0, c.src.Len())
	for v := range c.src.Values() {
		out = append(out, c.viewer.ToValueView(v))
	}
	return out
}

func (c convertedPairs[K, V, NK, NV]) AllSlice() []Entry[NK, NV] {
	out := make([]Entry[NK, NV], 0, c.src.Len())
	for k, v := range c.src.All() {
		out = append(out, Entry[NK, NV]{c.viewer.ToKeyView(k), c.viewer.ToValueView(v)})
	}
	return out
}

// ---------------------------------------------------------------------------
// SortedMap -- values convert, keys do not (ADR 0012)
// ---------------------------------------------------------------------------

// SortedMapView is a read-only view of a SortedMap, with values converted.
// Keys are cmp.Ordered and pass through unchanged, which is what lets a sorted
// view stand in for an unordered one without breaking the hierarchy (ADR 0012).
type SortedMapView[K cmp.Ordered, NV any] struct {
	impl innerSortedKeyValues[K, NV]
}

// ViewSortedMapWith returns a view of v whose values are converted by viewer.
//
// The source is a view, so views compose (ADR 0023): pass m.View() to convert a
// container, or an existing SortedMapView to narrow one further. Keys are not
// converted at any depth -- see ADR 0012 section 3.
func ViewSortedMapWith[K cmp.Ordered, V, NV any](v SortedMapView[K, V], viewer CanViewSortedMap[V, NV]) SortedMapView[K, NV] {
	return SortedMapView[K, NV]{impl: convertedValues[K, V, NV]{v, viewer}}
}

// View returns a read-only view of m that converts nothing. Free, as
// MapSet.View is. It replaced ViewSortedMapIdentity in ADR 0022.
//
// Read-only in STRUCTURE only: keys are cmp.Ordered and safe, but if V is a
// pointer or contains one, the values stay mutable. Use ViewSortedMap with a
// value viewer when that matters.
func (m SortedMap[K, V]) View() SortedMapView[K, V] {
	return SortedMapView[K, V]{impl: m}
}

func (v SortedMapView[K, NV]) IsZero() bool { return v.impl == nil }

func (v SortedMapView[K, NV]) Len() int {
	if v.impl == nil {
		return 0
	}
	return v.impl.Len()
}

func (v SortedMapView[K, NV]) Has(k K) bool {
	if v.impl == nil {
		return false
	}
	return v.impl.Has(k)
}

func (v SortedMapView[K, NV]) Get(k K) (NV, bool) {
	if v.impl == nil {
		var z NV
		return z, false
	}
	return v.impl.Get(k)
}

func (v SortedMapView[K, NV]) Keys() iter.Seq[K] {
	impl := v.impl
	return func(yield func(K) bool) {
		if impl == nil {
			return
		}
		impl.Keys()(yield)
	}
}

func (v SortedMapView[K, NV]) Values() iter.Seq[NV] {
	impl := v.impl
	return func(yield func(NV) bool) {
		if impl == nil {
			return
		}
		impl.Values()(yield)
	}
}

func (v SortedMapView[K, NV]) All() iter.Seq2[K, NV] {
	impl := v.impl
	return func(yield func(K, NV) bool) {
		if impl == nil {
			return
		}
		impl.All()(yield)
	}
}

func (v SortedMapView[K, NV]) KeySlice() []K {
	if v.impl == nil {
		return nil
	}
	return v.impl.KeySlice()
}

func (v SortedMapView[K, NV]) ValueSlice() []NV {
	if v.impl == nil {
		return nil
	}
	return v.impl.ValueSlice()
}

func (v SortedMapView[K, NV]) AllSlice() []Entry[K, NV] {
	if v.impl == nil {
		return nil
	}
	return v.impl.AllSlice()
}

func (v SortedMapView[K, NV]) Min() (K, NV, bool) {
	if v.impl == nil {
		var zk K
		var zv NV
		return zk, zv, false
	}
	return v.impl.Min()
}

func (v SortedMapView[K, NV]) Max() (K, NV, bool) {
	if v.impl == nil {
		var zk K
		var zv NV
		return zk, zv, false
	}
	return v.impl.Max()
}

func (v SortedMapView[K, NV]) Floor(k K) (K, NV, bool) {
	if v.impl == nil {
		var zk K
		var zv NV
		return zk, zv, false
	}
	return v.impl.Floor(k)
}

func (v SortedMapView[K, NV]) Ceil(k K) (K, NV, bool) {
	if v.impl == nil {
		var zk K
		var zv NV
		return zk, zv, false
	}
	return v.impl.Ceil(k)
}

func (v SortedMapView[K, NV]) Range(lo, hi K) iter.Seq2[K, NV] {
	if v.impl == nil {
		return func(func(K, NV) bool) {}
	}
	return v.impl.Range(lo, hi)
}

func (v SortedMapView[K, NV]) MinKey() (K, bool) {
	if v.impl == nil {
		var z K
		return z, false
	}
	return v.impl.MinKey()
}

func (v SortedMapView[K, NV]) MaxKey() (K, bool) {
	if v.impl == nil {
		var z K
		return z, false
	}
	return v.impl.MaxKey()
}

func (v SortedMapView[K, NV]) FloorKey(k K) (K, bool) {
	if v.impl == nil {
		var z K
		return z, false
	}
	return v.impl.FloorKey(k)
}

func (v SortedMapView[K, NV]) CeilKey(k K) (K, bool) {
	if v.impl == nil {
		var z K
		return z, false
	}
	return v.impl.CeilKey(k)
}

func (v SortedMapView[K, NV]) RangeKeys(lo, hi K) iter.Seq[K] {
	if v.impl == nil {
		return func(func(K) bool) {}
	}
	return v.impl.RangeKeys(lo, hi)
}

// convertedValues adapts a SortedMapView through a value viewer; keys pass
// through (ADR 0012 section 3). The source is a view, so these compose (ADR 0023).
// K stays cmp.Ordered because SortedMapView requires it.
type convertedValues[K cmp.Ordered, V, NV any] struct {
	src    SortedMapView[K, V]
	viewer CanViewSortedMap[V, NV]
}

func (c convertedValues[K, V, NV]) Len() int               { return c.src.Len() }
func (c convertedValues[K, V, NV]) Has(k K) bool           { return c.src.Has(k) }
func (c convertedValues[K, V, NV]) Keys() iter.Seq[K]      { return c.src.Keys() }
func (c convertedValues[K, V, NV]) KeySlice() []K          { return c.src.KeySlice() }
func (c convertedValues[K, V, NV]) MinKey() (K, bool)      { return c.src.MinKey() }
func (c convertedValues[K, V, NV]) MaxKey() (K, bool)      { return c.src.MaxKey() }
func (c convertedValues[K, V, NV]) FloorKey(k K) (K, bool) { return c.src.FloorKey(k) }
func (c convertedValues[K, V, NV]) CeilKey(k K) (K, bool)  { return c.src.CeilKey(k) }
func (c convertedValues[K, V, NV]) RangeKeys(lo, hi K) iter.Seq[K] {
	return c.src.RangeKeys(lo, hi)
}

func (c convertedValues[K, V, NV]) Get(k K) (NV, bool) {
	v, ok := c.src.Get(k)
	if !ok {
		var z NV
		return z, false
	}
	return c.viewer.ToValueView(v), true
}

func (c convertedValues[K, V, NV]) Values() iter.Seq[NV] {
	src, vw := c.src, c.viewer
	return func(yield func(NV) bool) {
		for v := range src.Values() {
			if !yield(vw.ToValueView(v)) {
				return
			}
		}
	}
}

func (c convertedValues[K, V, NV]) All() iter.Seq2[K, NV] {
	src, vw := c.src, c.viewer
	return func(yield func(K, NV) bool) {
		for k, v := range src.All() {
			if !yield(k, vw.ToValueView(v)) {
				return
			}
		}
	}
}

func (c convertedValues[K, V, NV]) ValueSlice() []NV {
	out := make([]NV, 0, c.src.Len())
	for v := range c.src.Values() {
		out = append(out, c.viewer.ToValueView(v))
	}
	return out
}

func (c convertedValues[K, V, NV]) AllSlice() []Entry[K, NV] {
	out := make([]Entry[K, NV], 0, c.src.Len())
	for k, v := range c.src.All() {
		out = append(out, Entry[K, NV]{k, c.viewer.ToValueView(v)})
	}
	return out
}

func (c convertedValues[K, V, NV]) Min() (K, NV, bool)      { return c.conv(c.src.Min()) }
func (c convertedValues[K, V, NV]) Max() (K, NV, bool)      { return c.conv(c.src.Max()) }
func (c convertedValues[K, V, NV]) Floor(k K) (K, NV, bool) { return c.conv(c.src.Floor(k)) }
func (c convertedValues[K, V, NV]) Ceil(k K) (K, NV, bool)  { return c.conv(c.src.Ceil(k)) }

func (c convertedValues[K, V, NV]) conv(k K, v V, ok bool) (K, NV, bool) {
	if !ok {
		var zk K
		var zv NV
		return zk, zv, false
	}
	return k, c.viewer.ToValueView(v), true
}

func (c convertedValues[K, V, NV]) Range(lo, hi K) iter.Seq2[K, NV] {
	src, vw := c.src, c.viewer
	return func(yield func(K, NV) bool) {
		for k, v := range src.Range(lo, hi) {
			if !yield(k, vw.ToValueView(v)) {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Sequences: Vector and a plain []T
//
// These are separate concrete types rather than one IndexedView, because tier 1
// is one view per container (ADR 0018). Anything wanting either takes
// PositionValues[int, NT].
// ---------------------------------------------------------------------------

// VectorView is a read-only view of a Vector, with elements converted.
type VectorView[NT any] struct{ impl innerPositionValues[int, NT] }

// ViewVectorWith returns a view of v whose elements are converted by viewer.
//
// The source is a view, so views compose (ADR 0023): pass v.View() to convert a
// Vector, ViewSliceIdentity(s) to convert a plain slice, or an existing
// VectorView to narrow one further.
//
// It also tracks growth, which the old ViewVector did not: it held
// v.ValueSlice() and so froze at construction. See convertedElems.
func ViewVectorWith[T, NT any](v VectorView[T], viewer CanViewVector[T, NT]) VectorView[NT] {
	return VectorView[NT]{impl: convertedElems[T, NT]{v, viewer}}
}

// View returns a read-only view of v that converts nothing. Free, as
// MapSet.View is. It replaced ViewVectorIdentity in ADR 0022.
//
// Unlike ViewSlice, this tracks growth: the view holds the Vector, and a Vector
// hides reallocation from every holder (ADR 0015).
//
// Read-only in STRUCTURE only: if T is a pointer or contains one, the elements
// stay mutable. Use ViewVector with a viewer when that matters.
func (v Vector[T]) View() VectorView[T] {
	return VectorView[T]{impl: v}
}

// ViewSlice returns a view of a plain []T.
//
// It views a VALUE, not a variable (ADR 0016): an element write is visible
// through the view and an append is not, because the append made a different
// slice the view was never given. Reach for Vector when growth must be visible.
// It keeps a []T source where every other converting constructor takes a view,
// because a []T has no View() method to call -- a builtin is not this package's
// type to extend. That is the language rather than an exception (ADR 0023).
func ViewSliceWith[T, NT any](s []T, viewer CanViewSlice[T, NT]) VectorView[NT] {
	return VectorView[NT]{impl: convertedElems[T, NT]{ViewSliceIdentity(s), viewer}}
}

// ViewSliceIdentity returns a view of s that converts nothing.
//
// This is the one identity view that stayed a function when ADR 0022 turned the
// rest into a View() method: a plain []T has no methods to hang one on.
func ViewSliceIdentity[T any](s []T) VectorView[T] {
	return VectorView[T]{impl: rawSlice[T]{s}}
}

func (v VectorView[NT]) IsZero() bool { return v.impl == nil }

func (v VectorView[NT]) Len() int {
	if v.impl == nil {
		return 0
	}
	return v.impl.Len()
}

func (v VectorView[NT]) At(i int) NT {
	if v.impl == nil {
		panic("containers: index out of range on a zero VectorView")
	}
	return v.impl.At(i)
}

func (v VectorView[NT]) Values() iter.Seq[NT] {
	impl := v.impl
	return func(yield func(NT) bool) {
		if impl == nil {
			return
		}
		impl.Values()(yield)
	}
}

func (v VectorView[NT]) Positions() iter.Seq[int] {
	impl := v.impl
	return func(yield func(int) bool) {
		if impl == nil {
			return
		}
		impl.Positions()(yield)
	}
}

func (v VectorView[NT]) All() iter.Seq2[int, NT] {
	impl := v.impl
	return func(yield func(int, NT) bool) {
		if impl == nil {
			return
		}
		impl.All()(yield)
	}
}

func (v VectorView[NT]) ValueSlice() []NT {
	if v.impl == nil {
		return nil
	}
	return v.impl.ValueSlice()
}

func (v VectorView[NT]) PositionSlice() []int {
	if v.impl == nil {
		return nil
	}
	return v.impl.PositionSlice()
}

func (v VectorView[NT]) AllSlice() []Entry[int, NT] {
	if v.impl == nil {
		return nil
	}
	return v.impl.AllSlice()
}

// rawSlice adapts a plain []T to PositionValues without conversion.
type rawSlice[T any] struct{ s []T }

func (r rawSlice[T]) Len() int   { return len(r.s) }
func (r rawSlice[T]) At(i int) T { return r.s[i] }

func (r rawSlice[T]) Values() iter.Seq[T] { return slices.Values(r.s) }

func (r rawSlice[T]) Positions() iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range r.s {
			if !yield(i) {
				return
			}
		}
	}
}

func (r rawSlice[T]) All() iter.Seq2[int, T] { return slices.All(r.s) }

func (r rawSlice[T]) ValueSlice() []T { return slices.Clone(r.s) }

func (r rawSlice[T]) PositionSlice() []int {
	out := make([]int, len(r.s))
	for i := range out {
		out[i] = i
	}
	return out
}

func (r rawSlice[T]) AllSlice() []Entry[int, T] {
	out := make([]Entry[int, T], 0, len(r.s))
	for i, e := range r.s {
		out = append(out, Entry[int, T]{i, e})
	}
	return out
}

// convertedElems adapts a VectorView through an element viewer (ADR 0023).
//
// The source is a VIEW, not a []T. That is what fixes the defect ADR 0023
// records: the old form held v.ValueSlice(), so a converting Vector view
// SNAPSHOTTED at construction and stopped tracking appends while the identity
// view tracked them -- contradicting Vector's contract (ADR 0015) and View's own
// doc comment. Holding the view walks it live.
//
// A plain []T reaches this through ViewSliceWith, which wraps the slice in an
// identity view first; that keeps a slice view's header semantics unchanged
// (ADR 0016 decision 2).
type convertedElems[T, NT any] struct {
	src    VectorView[T]
	viewer interface{ ToValueView(T) NT }
}

func (c convertedElems[T, NT]) Len() int    { return c.src.Len() }
func (c convertedElems[T, NT]) At(i int) NT { return c.viewer.ToValueView(c.src.At(i)) }

func (c convertedElems[T, NT]) Values() iter.Seq[NT] {
	src, vw := c.src, c.viewer
	return func(yield func(NT) bool) {
		for e := range src.Values() {
			if !yield(vw.ToValueView(e)) {
				return
			}
		}
	}
}

func (c convertedElems[T, NT]) Positions() iter.Seq[int] { return c.src.Positions() }

func (c convertedElems[T, NT]) All() iter.Seq2[int, NT] {
	src, vw := c.src, c.viewer
	return func(yield func(int, NT) bool) {
		for i, e := range src.All() {
			if !yield(i, vw.ToValueView(e)) {
				return
			}
		}
	}
}

func (c convertedElems[T, NT]) ValueSlice() []NT {
	out := make([]NT, 0, c.src.Len())
	for e := range c.src.Values() {
		out = append(out, c.viewer.ToValueView(e))
	}
	return out
}

func (c convertedElems[T, NT]) PositionSlice() []int { return c.src.PositionSlice() }

func (c convertedElems[T, NT]) AllSlice() []Entry[int, NT] {
	out := make([]Entry[int, NT], 0, c.src.Len())
	for i, e := range c.src.All() {
		out = append(out, Entry[int, NT]{i, c.viewer.ToValueView(e)})
	}
	return out
}
