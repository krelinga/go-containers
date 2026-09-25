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
//	func audit(k Keys[string])          // reads; a container or a view, either
//
// Keys and its siblings are NOT sealed -- containers satisfy them too, which is
// the point. They are a convenience, not a guarantee: a CONTAINER placed in one
// can be asserted back out and written through. Use a concrete view where the
// guarantee matters.
//
// # Every view is exactly one word
//
// This is a rule, not an implementation detail. A one-word struct is
// pointer-shaped and boxes into an interface for free; anything wider allocates
// on EVERY crossing -- measured at 13.59 ns and one allocation against 0.88 ns
// and none (ADR 0018). So a view holds one pointer to its state, and a viewer
// lives inside that state rather than beside it.
//
// **Do not add a second field to a view struct.** It compiles, and it silently
// allocates at every call site that generalises.
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

// ViewMapSet returns a view of s whose keys are converted by viewer.
func ViewMapSet[T comparable, NT any](s MapSet[T], viewer CanViewMapSet[T, NT]) MapSetView[NT] {
	return MapSetView[NT]{impl: convertedKeys[T, NT]{s, viewer}}
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

// convertedKeys adapts a MapSet through a key viewer. It satisfies Keys[NT]
// and lives behind the view's single pointer.
type convertedKeys[T comparable, NT any] struct {
	s      MapSet[T]
	viewer CanViewMapSet[T, NT]
}

func (c convertedKeys[T, NT]) Len() int { return c.s.Len() }

func (c convertedKeys[T, NT]) Has(nt NT) bool {
	t, ok := c.viewer.FromKeyView(nt)
	return ok && c.s.Has(t)
}

func (c convertedKeys[T, NT]) Keys() iter.Seq[NT] {
	s, vw := c.s, c.viewer
	return func(yield func(NT) bool) {
		for t := range s.Keys() {
			if !yield(vw.ToKeyView(t)) {
				return
			}
		}
	}
}

func (c convertedKeys[T, NT]) KeySlice() []NT {
	out := make([]NT, 0, c.s.Len())
	for t := range c.s.Keys() {
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

// ViewMap returns a view of m whose entries are converted by viewer.
func ViewMap[K comparable, V, NK, NV any](m Map[K, V], viewer CanViewMap[K, NK, V, NV]) MapView[NK, NV] {
	return MapView[NK, NV]{impl: convertedPairs[K, V, NK, NV]{m, viewer}}
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

// convertedPairs adapts a Map through a key/value viewer.
type convertedPairs[K comparable, V, NK, NV any] struct {
	m      Map[K, V]
	viewer CanViewMap[K, NK, V, NV]
}

func (c convertedPairs[K, V, NK, NV]) Len() int { return c.m.Len() }

func (c convertedPairs[K, V, NK, NV]) Has(nk NK) bool {
	k, ok := c.viewer.FromKeyView(nk)
	return ok && c.m.Has(k)
}

func (c convertedPairs[K, V, NK, NV]) Get(nk NK) (NV, bool) {
	k, ok := c.viewer.FromKeyView(nk)
	if !ok {
		var z NV
		return z, false
	}
	v, ok := c.m.Get(k)
	if !ok {
		var z NV
		return z, false
	}
	return c.viewer.ToValueView(v), true
}

func (c convertedPairs[K, V, NK, NV]) Keys() iter.Seq[NK] {
	m, vw := c.m, c.viewer
	return func(yield func(NK) bool) {
		for k := range m.Keys() {
			if !yield(vw.ToKeyView(k)) {
				return
			}
		}
	}
}

func (c convertedPairs[K, V, NK, NV]) Values() iter.Seq[NV] {
	m, vw := c.m, c.viewer
	return func(yield func(NV) bool) {
		for v := range m.Values() {
			if !yield(vw.ToValueView(v)) {
				return
			}
		}
	}
}

func (c convertedPairs[K, V, NK, NV]) All() iter.Seq2[NK, NV] {
	m, vw := c.m, c.viewer
	return func(yield func(NK, NV) bool) {
		for k, v := range m.All() {
			if !yield(vw.ToKeyView(k), vw.ToValueView(v)) {
				return
			}
		}
	}
}

func (c convertedPairs[K, V, NK, NV]) KeySlice() []NK {
	out := make([]NK, 0, c.m.Len())
	for k := range c.m.Keys() {
		out = append(out, c.viewer.ToKeyView(k))
	}
	return out
}

func (c convertedPairs[K, V, NK, NV]) ValueSlice() []NV {
	out := make([]NV, 0, c.m.Len())
	for v := range c.m.Values() {
		out = append(out, c.viewer.ToValueView(v))
	}
	return out
}

func (c convertedPairs[K, V, NK, NV]) AllSlice() []Entry[NK, NV] {
	out := make([]Entry[NK, NV], 0, c.m.Len())
	for k, v := range c.m.All() {
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

// ViewSortedMap returns a view of m whose values are converted by viewer.
func ViewSortedMap[K cmp.Ordered, V, NV any](m SortedMap[K, V], viewer CanViewSortedMap[V, NV]) SortedMapView[K, NV] {
	return SortedMapView[K, NV]{impl: convertedValues[K, V, NV]{m, viewer}}
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

// convertedValues adapts a SortedMap through a value viewer. Keys pass through.
type convertedValues[K cmp.Ordered, V, NV any] struct {
	m      SortedMap[K, V]
	viewer CanViewSortedMap[V, NV]
}

func (c convertedValues[K, V, NV]) Len() int               { return c.m.Len() }
func (c convertedValues[K, V, NV]) Has(k K) bool           { return c.m.Has(k) }
func (c convertedValues[K, V, NV]) Keys() iter.Seq[K]      { return c.m.Keys() }
func (c convertedValues[K, V, NV]) KeySlice() []K          { return c.m.KeySlice() }
func (c convertedValues[K, V, NV]) MinKey() (K, bool)      { return c.m.MinKey() }
func (c convertedValues[K, V, NV]) MaxKey() (K, bool)      { return c.m.MaxKey() }
func (c convertedValues[K, V, NV]) FloorKey(k K) (K, bool) { return c.m.FloorKey(k) }
func (c convertedValues[K, V, NV]) CeilKey(k K) (K, bool)  { return c.m.CeilKey(k) }
func (c convertedValues[K, V, NV]) RangeKeys(lo, hi K) iter.Seq[K] {
	return c.m.RangeKeys(lo, hi)
}

func (c convertedValues[K, V, NV]) Get(k K) (NV, bool) {
	v, ok := c.m.Get(k)
	if !ok {
		var z NV
		return z, false
	}
	return c.viewer.ToValueView(v), true
}

func (c convertedValues[K, V, NV]) Values() iter.Seq[NV] {
	m, vw := c.m, c.viewer
	return func(yield func(NV) bool) {
		for v := range m.Values() {
			if !yield(vw.ToValueView(v)) {
				return
			}
		}
	}
}

func (c convertedValues[K, V, NV]) All() iter.Seq2[K, NV] {
	m, vw := c.m, c.viewer
	return func(yield func(K, NV) bool) {
		for k, v := range m.All() {
			if !yield(k, vw.ToValueView(v)) {
				return
			}
		}
	}
}

func (c convertedValues[K, V, NV]) ValueSlice() []NV {
	out := make([]NV, 0, c.m.Len())
	for v := range c.m.Values() {
		out = append(out, c.viewer.ToValueView(v))
	}
	return out
}

func (c convertedValues[K, V, NV]) AllSlice() []Entry[K, NV] {
	out := make([]Entry[K, NV], 0, c.m.Len())
	for k, v := range c.m.All() {
		out = append(out, Entry[K, NV]{k, c.viewer.ToValueView(v)})
	}
	return out
}

func (c convertedValues[K, V, NV]) Min() (K, NV, bool)      { return c.conv(c.m.Min()) }
func (c convertedValues[K, V, NV]) Max() (K, NV, bool)      { return c.conv(c.m.Max()) }
func (c convertedValues[K, V, NV]) Floor(k K) (K, NV, bool) { return c.conv(c.m.Floor(k)) }
func (c convertedValues[K, V, NV]) Ceil(k K) (K, NV, bool)  { return c.conv(c.m.Ceil(k)) }

func (c convertedValues[K, V, NV]) conv(k K, v V, ok bool) (K, NV, bool) {
	if !ok {
		var zk K
		var zv NV
		return zk, zv, false
	}
	return k, c.viewer.ToValueView(v), true
}

func (c convertedValues[K, V, NV]) Range(lo, hi K) iter.Seq2[K, NV] {
	m, vw := c.m, c.viewer
	return func(yield func(K, NV) bool) {
		for k, v := range m.Range(lo, hi) {
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

// ViewVector returns a view of v whose elements are converted by viewer.
func ViewVector[T, NT any](v Vector[T], viewer CanViewVector[T, NT]) VectorView[NT] {
	return VectorView[NT]{impl: convertedElems[T, NT]{v.ValueSlice(), viewer}}
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
func ViewSlice[T, NT any](s []T, viewer CanViewSlice[T, NT]) VectorView[NT] {
	return VectorView[NT]{impl: convertedElems[T, NT]{s, viewer}}
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

// convertedElems adapts a []T through an element viewer.
type convertedElems[T, NT any] struct {
	s      []T
	viewer interface{ ToValueView(T) NT }
}

func (c convertedElems[T, NT]) Len() int    { return len(c.s) }
func (c convertedElems[T, NT]) At(i int) NT { return c.viewer.ToValueView(c.s[i]) }

func (c convertedElems[T, NT]) Values() iter.Seq[NT] {
	s, vw := c.s, c.viewer
	return func(yield func(NT) bool) {
		for _, e := range s {
			if !yield(vw.ToValueView(e)) {
				return
			}
		}
	}
}

func (c convertedElems[T, NT]) Positions() iter.Seq[int] {
	s := c.s
	return func(yield func(int) bool) {
		for i := range s {
			if !yield(i) {
				return
			}
		}
	}
}

func (c convertedElems[T, NT]) All() iter.Seq2[int, NT] {
	s, vw := c.s, c.viewer
	return func(yield func(int, NT) bool) {
		for i, e := range s {
			if !yield(i, vw.ToValueView(e)) {
				return
			}
		}
	}
}

func (c convertedElems[T, NT]) ValueSlice() []NT {
	out := make([]NT, 0, len(c.s))
	for _, e := range c.s {
		out = append(out, c.viewer.ToValueView(e))
	}
	return out
}

func (c convertedElems[T, NT]) PositionSlice() []int {
	out := make([]int, len(c.s))
	for i := range out {
		out[i] = i
	}
	return out
}

func (c convertedElems[T, NT]) AllSlice() []Entry[int, NT] {
	out := make([]Entry[int, NT], 0, len(c.s))
	for i, e := range c.s {
		out = append(out, Entry[int, NT]{i, c.viewer.ToValueView(e)})
	}
	return out
}
