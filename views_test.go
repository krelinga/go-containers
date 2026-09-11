package containers_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/krelinga/go-containers"
)

// ---- the caller-supplied read-only types ADR 0012 says the library cannot
// invent ---------------------------------------------------------------------

type item struct{ Name string }

type itemView struct{ i *item }

func (v itemView) Name() string { return v.i.Name }

// A key viewer for *item keys: out to a string, back by lookup in a registry.
// It is stateful, which is why the type-parameter witness shape cannot express
// ADR 0012's viewers.
type itemKeys struct{ known map[string]*item }

func (k itemKeys) ToKeyView(i *item) string { return i.Name }
func (k itemKeys) FromKeyView(s string) (*item, bool) {
	i, ok := k.known[s]
	return i, ok
}

type itemValues struct{}

func (itemValues) ToValueView(i *item) itemView { return itemView{i} }

// Composed by embedding, as ADR 0012 intends.
type itemViewer struct {
	itemKeys
	itemValues
}

// ---- what a view is for ----------------------------------------------------

// The seal is a compile-time property, so most of it cannot be asserted at
// runtime. These lines are the test, and they are checked by the compiler every
// build:
//
//	var _ containers.MapView[string, int] = containers.Map[string, int]{}
//	  -> *HashDict[string, int] does not implement DictView[string, int]
//	     (missing method sealedView)
//
//	v := containers.ViewMapIdentity(d)
//	_ = v.(containers.Map[string, int])
//	  -> impossible type assertion
//
// What is left to check at runtime is that laundering through any() does not
// recover the container either.
func TestViewDoesNotLeakItsContainer(t *testing.T) {
	d := containers.Map[string, int]{}
	d.Set("a", 1)
	v := containers.ViewMapIdentity(d)

	if _, ok := any(v).(containers.Map[string, int]); ok {
		t.Error("view was assertable back to its container through any()")
	}
	if _, ok := any(v).(containers.MutableKeyValues[string, int]); ok {
		t.Error("view satisfied the mutation contract")
	}
	if v.Len() != 1 {
		t.Errorf("Len = %d, want 1", v.Len())
	}
}

// ADR 0012's motivating hole: comparable admits pointers, so a value-only view
// leaked mutable keys. A key viewer closes it.
func TestViewConvertsKeys(t *testing.T) {
	k := &item{Name: "alpha"}
	d := containers.Map[*item, *item]{}
	d.Set(k, &item{Name: "payload"})

	vw := itemViewer{itemKeys: itemKeys{known: map[string]*item{"alpha": k}}}
	var v containers.MapView[string, itemView] = containers.ViewMap(d, vw)

	for gotKey, gotVal := range v.All() {
		if gotKey != "alpha" {
			t.Errorf("key = %v, want %q", gotKey, "alpha")
		}
		if gotVal.Name() != "payload" {
			t.Errorf("value = %q", gotVal.Name())
		}
	}
}

// A key that does not convert back cannot be present.
func TestFromKeyViewFailureIsAMiss(t *testing.T) {
	k := &item{Name: "alpha"}
	d := containers.Map[*item, *item]{}
	d.Set(k, &item{Name: "payload"})

	vw := itemViewer{itemKeys: itemKeys{known: map[string]*item{"alpha": k}}}
	v := containers.ViewMap(d, vw)

	if _, ok := v.Get("alpha"); !ok {
		t.Error("a convertible, present key should hit")
	}
	got, ok := v.Get("not-a-known-key")
	if ok || got != (itemView{}) {
		t.Errorf("unconvertible key = %v,%v; want zero,false", got, ok)
	}
}

// A miss must not call the value conversion.
func TestMissDoesNotConvert(t *testing.T) {
	d := containers.NewSortedMap[int, *item]()
	called := false
	v := containers.ViewSortedMap(d, valueCounter{&called})
	if _, ok := v.Get(99); ok {
		t.Error("empty dict should miss")
	}
	if called {
		t.Error("conversion ran for a missing key")
	}
}

type valueCounter struct{ called *bool }

func (c valueCounter) ToValueView(i *item) itemView { *c.called = true; return itemView{i} }

// ---- ordered containers convert values only (ADR 0012 decision 3) ----------

func TestSortedViewsConvertValuesOnly(t *testing.T) {
	sd := containers.NewSortedMap[int, *item]()
	sd.Set(1, &item{Name: "one"})
	sd.Set(3, &item{Name: "three"})
	v := containers.ViewSortedMap(sd, itemValues{})

	// Ordered lookups take and return the container's own key type: no
	// conversion, so no order-preservation question arises.
	if k, val, ok := v.Min(); k != 1 || !ok || val.Name() != "one" {
		t.Errorf("Min = %v,%v,%v", k, val, ok)
	}
	if k, _, ok := v.Ceil(2); k != 3 || !ok {
		t.Errorf("Ceil(2) = %v,%v", k, ok)
	}
	if got := slices.Sorted(maps.Keys(maps.Collect(v.Range(1, 3)))); !slices.Equal(got, []int{1}) {
		t.Errorf("Range(1,3) keys = %v", got)
	}

	// A sorted set view takes no viewer at all.
	sv := containers.ViewSortedSet(containers.NewSortedSet(3, 1, 2))
	if mn, ok := sv.MinKey(); mn != 1 || !ok {
		t.Errorf("SortedSetView.Min = %v,%v", mn, ok)
	}
	if got := slices.Collect(sv.RangeKeys(2, 4)); !slices.Equal(got, []int{2, 3}) {
		t.Errorf("SortedSetView.Range = %v", got)
	}
}

// ---- the hierarchy (ADR 0013 decision 2) -----------------------------------

// An ordered view substitutes for the unordered one, so a consumer can be
// written against DictView without naming an implementation.
func TestOrderedViewsSubstituteForBase(t *testing.T) {
	sd := containers.NewSortedMap[string, int]()
	sd.Set("a", 1)
	hd := containers.Map[string, int]{}
	hd.Set("a", 1)
	m := containers.Map[string, int]{"a": 1}

	// All three are the same type at this boundary.
	for name, d := range map[string]containers.KeyValues[string, int]{
		"SortedDict": containers.ViewSortedMapIdentity(sd),
		"HashDict":   containers.ViewMapIdentity(hd),
		"Map":        containers.ViewMapIdentity(m),
	} {
		if got, ok := d.Get("a"); !ok || got != 1 {
			t.Errorf("%s: Get(a) = %v,%v", name, got, ok)
		}
	}

	ss := containers.NewSortedSet(1, 2)
	hs := containers.NewMapSet(1, 2)
	for name, s := range map[string]containers.Keys[int]{
		"SortedSet": containers.ViewSortedSet(ss),
		"HashSet":   containers.ViewMapSetIdentity(hs),
	} {
		if !s.Has(1) {
			t.Errorf("%s: Has(1) = false", name)
		}
	}
}

// ---- shape rules -----------------------------------------------------------

// A zero view reads as empty, exactly as a zero container does (ADR 0018).
//
// This inverts what ADR 0013 asserted. Views were sealed interfaces then, so a
// zero one was a nil interface and every call panicked; they are concrete
// structs now, and the rule across the whole package is that reads are total
// and writes panic. A view has no writes, so a zero view is simply empty.
func TestZeroViewReadsAsEmpty(t *testing.T) {
	var sv containers.MapSetView[int]
	var dv containers.MapView[int, string]
	var ssv containers.SortedSetView[int]
	var sdv containers.SortedMapView[int, string]
	var vv containers.VectorView[int]

	if !sv.IsZero() || !dv.IsZero() || !ssv.IsZero() || !sdv.IsZero() || !vv.IsZero() {
		t.Error("a zero view should report IsZero")
	}
	if sv.Len() != 0 || sv.Has(1) || sv.KeySlice() != nil {
		t.Error("zero MapSetView should read as empty")
	}
	if _, ok := dv.Get(1); ok || dv.Len() != 0 {
		t.Error("zero MapView should read as empty")
	}
	if _, ok := ssv.MinKey(); ok {
		t.Error("zero SortedSetView should have no minimum")
	}
	for range sdv.Range(1, 2) {
		t.Error("zero SortedMapView should yield nothing")
	}
	for range vv.Values() {
		t.Error("zero VectorView should yield nothing")
	}

	// At is the exception: an index is out of range for anything empty, which
	// is what indexing a nil slice does too.
	mustPanic(t, "VectorView.At", func() { _ = vv.At(0) })
}

// A view over a zero container reads as empty too -- the emptiness composes
// rather than turning into a panic somewhere in the middle.
func TestViewOverZeroContainerReadsAsEmpty(t *testing.T) {
	zs := containers.NewMapSet[int]()
	zss := containers.NewSortedSet[int]()
	zv := containers.NewVector[int]()

	if v := containers.ViewMapSetIdentity(zs); v.Len() != 0 || v.Has(1) {
		t.Error("view over a zero MapSet should be empty")
	}
	if v := containers.ViewSortedSet(zss); v.Len() != 0 {
		t.Error("view over a zero SortedSet should be empty")
	}
	if v := containers.ViewVectorIdentity(zv); v.Len() != 0 {
		t.Error("view over a zero Vector should be empty")
	}
	n := 0
	for range containers.ViewMapSetIdentity(zs).Keys() {
		n++
	}
	if n != 0 {
		t.Errorf("iterating a view over a zero container yielded %d", n)
	}
}

// ---- IndexedView (ADR 0015 decision 5) --------------------------------------

type itemValuesOnly struct{}

func (itemValuesOnly) ToValueView(i *item) itemView { return itemView{i} }

func TestIndexedViewConvertsElements(t *testing.T) {
	v := containers.NewVector(&item{Name: "first"}, &item{Name: "second"})
	view := containers.ViewVector(v, itemValuesOnly{})

	if view.Len() != 2 {
		t.Errorf("Len = %d, want 2", view.Len())
	}
	if got := view.At(1).Name(); got != "second" {
		t.Errorf("At(1).Name() = %q", got)
	}

	var names []string
	for _, e := range view.All() {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{"first", "second"}) {
		t.Errorf("AllIndexed = %v", names)
	}
}

// A view is the O(1) answer to ADR 0001's accessor problem, so the identity
// form must not allocate.
func TestVectorIdentityViewDoesNotAllocate(t *testing.T) {
	v := containers.NewVector(1, 2, 3)
	var sink containers.VectorView[int]
	if got := testing.AllocsPerRun(100, func() { sink = containers.ViewVectorIdentity(v) }); got != 0 {
		t.Errorf("ViewVectorIdentity: %v allocs, want 0", got)
	}
	if sink.Len() != 3 {
		t.Errorf("Len = %d", sink.Len())
	}
}

// The seal, and the fact that a view carries no way to write.
//
//	var _ containers.VectorView[int] = containers.NewVector(1)
//	  -> *Vector[int] does not implement IndexedView[int] (missing method sealedView)
func TestIndexedViewIsSealed(t *testing.T) {
	v := containers.NewVector(1, 2)
	view := containers.ViewVectorIdentity(v)

	if _, ok := any(view).(containers.Vector[int]); ok {
		t.Error("view was assertable back to its container")
	}
	if _, ok := any(view).(interface{ Set(int, int) }); ok {
		t.Error("view exposed a mutator")
	}
	if _, ok := any(view).(interface{ Append(int) }); ok {
		t.Error("view exposed Append")
	}
}

// A zero VectorView reads as empty; only At panics, because an index is out of
// range for anything empty (ADR 0018).
func TestZeroVectorViewReadsAsEmpty(t *testing.T) {
	var zero containers.VectorView[int]
	if !zero.IsZero() || zero.Len() != 0 || zero.ValueSlice() != nil {
		t.Error("a zero VectorView should read as empty")
	}
	for range zero.All() {
		t.Error("a zero VectorView should yield nothing")
	}
	mustPanic(t, "VectorView.At", func() { _ = zero.At(0) })

	// And a view over a zero Vector is the same thing.
	zv := containers.NewVector[int]()
	if v := containers.ViewVectorIdentity(zv); v.Len() != 0 {
		t.Error("view over a zero Vector should be empty")
	}
}

// ---- ViewSlice (ADR 0016) --------------------------------------------------

func TestSliceViewConvertsElements(t *testing.T) {
	s := []*item{{Name: "first"}, {Name: "second"}}
	view := containers.ViewSlice(s, itemValuesOnly{})

	if view.Len() != 2 {
		t.Errorf("Len = %d, want 2", view.Len())
	}
	if got := view.At(1).Name(); got != "second" {
		t.Errorf("At(1).Name() = %q", got)
	}
	var names []string
	for _, e := range view.All() {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{"first", "second"}) {
		t.Errorf("AllIndexed = %v", names)
	}
}

// It views a slice, not a variable (ADR 0016 decision 2). An element write is
// visible because the view wraps that element; an append is not, because it
// produces a different slice value the view was never given.
func TestSliceViewViewsAValueNotAVariable(t *testing.T) {
	hosts := make([]string, 1, 4)
	hosts[0] = "original"
	v := containers.ViewSliceIdentity(hosts)

	hosts[0] = "rewritten"
	if got := v.At(0); got != "rewritten" {
		t.Errorf("element write not visible: At(0) = %q, want %q", got, "rewritten")
	}

	hosts = append(hosts, "appended") // within capacity
	if v.Len() != 1 {
		t.Errorf("append became visible: Len = %d, want 1", v.Len())
	}

	// And once the owner reallocates, the value this view holds is untouched.
	hosts = append(hosts, "b", "c", "d", "e") // forces a new backing array
	hosts[0] = "written after realloc"
	if got := v.At(0); got != "rewritten" {
		t.Errorf("view followed the reallocation: At(0) = %q", got)
	}
}

// A nil slice is a valid empty slice, so it makes a valid empty view.
func TestSliceViewOfNil(t *testing.T) {
	v := containers.ViewSliceIdentity[string](nil)
	if v.Len() != 0 {
		t.Errorf("Len = %d, want 0", v.Len())
	}
	if got := v.ValueSlice(); len(got) != 0 {
		t.Errorf("All = %v, want empty", got)
	}
	mustPanic(t, "At on an empty view", func() { _ = v.At(0) })
}

// A slice view costs one allocation, where every other view here costs none.
// Asserted so the cost is visible if it ever changes in either direction.
func TestSliceViewAllocatesOnce(t *testing.T) {
	s := []int{1, 2, 3}
	var sink containers.VectorView[int]
	if got := testing.AllocsPerRun(100, func() { sink = containers.ViewSliceIdentity(s) }); got != 1 {
		t.Errorf("ViewSliceIdentity: %v allocs, want 1 (a slice header is three words)", got)
	}
	if sink.Len() != 3 {
		t.Errorf("Len = %d", sink.Len())
	}
}

// The seal, and the absence of any way to write. A bare slice cannot be passed
// as a view:
//
//	var _ containers.VectorView[int] = []int{1}
//	  -> []int does not implement IndexedView[int] (missing method All)
//
// The compiler names All rather than sealedView because a slice is missing
// several methods and reports the first. The seal is still what makes the type
// unforgeable from outside the package; it is just not what the error says here.
func TestSliceViewIsSealed(t *testing.T) {
	view := containers.ViewSliceIdentity([]int{1, 2})

	if _, ok := any(view).([]int); ok {
		t.Error("view was assertable back to its slice")
	}
	if _, ok := any(view).(interface{ Set(int, int) }); ok {
		t.Error("view exposed a mutator")
	}
}

func TestBothSequenceViewsSatisfyIndexedView(t *testing.T) {
	vec := containers.NewVector("a", "b")
	for name, v := range map[string]containers.VectorView[string]{
		"Vector": containers.ViewVectorIdentity(vec),
		"slice":  containers.ViewSliceIdentity([]string{"a", "b"}),
	} {
		if v.Len() != 2 || v.At(0) != "a" {
			t.Errorf("%s: Len=%d At(0)=%q", name, v.Len(), v.At(0))
		}
	}
}
