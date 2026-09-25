package containers_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/krelinga/go-containers"
)

// The *Slice contract, which is the whole of ADR 0017 proposal D: a result is a
// full, independent copy. Nothing the container does afterwards is visible
// through it, and nothing done to it is visible in the container.
func TestSliceResultsAreIndependentCopies(t *testing.T) {
	hs := containers.NewMapSet(1, 2, 3)
	ks := hs.KeySlice()
	hs.Add(4)
	if len(ks) != 3 {
		t.Errorf("HashSet.KeySlice saw a later Add: len=%d, want 3", len(ks))
	}
	slices.Sort(ks)
	ks[0] = 99
	if !hs.Has(1) || hs.Has(99) {
		t.Error("writing to HashSet.KeySlice changed the set")
	}

	v := containers.NewVector(1, 2, 3)
	vs := v.ValueSlice()
	v.Append(4)
	vs[0] = 99
	if v.Len() != 4 || v.At(0) != 1 {
		t.Errorf("Vector.ValueSlice aliased the vector: len=%d at0=%d", v.Len(), v.At(0))
	}

	sd := containers.NewSortedMap(containers.Entry[int, string]{1, "a"})
	kvs := sd.AllSlice()
	kvs[0].Value = "mutated"
	if got, _ := sd.Get(1); got != "a" {
		t.Errorf("writing to SortedDict.AllSlice changed the dict: %q", got)
	}
}

// The payoff of that contract: the obvious spelling of "empty this container"
// is correct rather than merely lucky. Under a streaming API this silently
// corrupts a sorted container, which is why ADR 0017 preferred the copy.
func TestSelfReferentialBulkOperations(t *testing.T) {
	ss := containers.NewSortedSet(1, 2, 3)
	ss.DeleteAll(ss.KeySlice()...)
	if ss.Len() != 0 {
		t.Errorf("ss.DeleteAll(ss.KeySlice()...) left %d elements, want 0", ss.Len())
	}

	sd := containers.NewSortedMap[int, string]()
	sd.SetAll(pairsOf(maps.All(map[int]string{1: "a", 2: "b", 3: "c"}))...)
	sd.DeleteAll(sd.KeySlice()...)
	if sd.Len() != 0 {
		t.Errorf("sd.DeleteAll(sd.KeySlice()...) left %d entries, want 0", sd.Len())
	}

	// Adding a container to itself is a no-op rather than a hazard.
	ss2 := containers.NewSortedSet(3, 1, 2)
	ss2.AddAll(ss2.KeySlice()...)
	if got, want := ssVals(ss2), []int{1, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("ss2.AddAll(ss2.KeySlice()...) = %v, want %v", got, want)
	}
}

// Bulk operations copy their input and do not retain it (ADR 0017). The
// constructors deliberately do not take ownership, so that every New* means one
// thing; an adopting constructor would arrive later under its own name.
func TestBulkOperationsDoNotRetainTheirInput(t *testing.T) {
	src := []int{1, 2, 3}

	v := containers.NewVector(src...)
	src[0] = 99
	if v.At(0) != 1 {
		t.Errorf("NewVector retained its argument: At(0)=%d, want 1", v.At(0))
	}

	src2 := []int{4, 5, 6}
	v.AppendAll(src2...)
	src2[0] = 99
	if v.At(3) != 4 {
		t.Errorf("AppendAll retained its argument: At(3)=%d, want 4", v.At(3))
	}

	// A sorted container must not reorder the caller's slice while sorting.
	unsorted := []int{3, 1, 2}
	_ = containers.NewSortedSet(unsorted...)
	if !slices.Equal(unsorted, []int{3, 1, 2}) {
		t.Errorf("NewSortedSet reordered the caller's slice: %v", unsorted)
	}
}

// The zero value of every container behaves exactly as the builtin it stands
// in for: reads are total, and a write that actually writes panics (ADR 0018).
//
// This REPLACES ADR 0002's eager-dereference rule, which required every method
// to panic on an unconstructed receiver. Under reference semantics the zero
// value is a meaningful state -- the empty container you may read but not
// write -- so the rule it needs is the builtin's, not 0002's.
func TestZeroValueMatchesTheBuiltin(t *testing.T) {
	var ms containers.MapSet[int]
	var ss containers.SortedSet[int]
	var sm containers.SortedMap[int, string]
	var vec containers.Vector[int]
	var m containers.Map[int, string]

	// Reads are total everywhere.
	if ms.Len() != 0 || ss.Len() != 0 || sm.Len() != 0 || vec.Len() != 0 || m.Len() != 0 {
		t.Error("a zero container should report Len 0")
	}
	if ms.Has(1) || ss.Has(1) || sm.Has(1) || m.Has(1) {
		t.Error("a zero container should contain nothing")
	}
	if ms.KeySlice() != nil || ss.KeySlice() != nil || vec.ValueSlice() != nil {
		t.Error("a zero container should materialise as nil")
	}
	for range ms.Keys() {
		t.Error("a zero container should iterate zero times")
	}

	// Writes that write, panic.
	mustPanic(t, "MapSet.Add", func() { ms.Add(1) })
	mustPanic(t, "MapSet.AddAll", func() { ms.AddAll(1) })
	mustPanic(t, "SortedSet.Add", func() { ss.Add(1) })
	mustPanic(t, "SortedSet.AddAll", func() { ss.AddAll(1) })
	mustPanic(t, "SortedMap.Set", func() { sm.Set(1, "a") })
	mustPanic(t, "SortedMap.SetAll", func() { sm.SetAll(containers.Entry[int, string]{1, "a"}) })
	mustPanic(t, "Vector.Append", func() { vec.Append(1) })
	mustPanic(t, "Vector.AppendAll", func() { vec.AppendAll(1) })
	mustPanic(t, "Map.Set", func() { m.Set(1, "a") })

	// Writes that write NOTHING do not panic -- a loop that never reaches
	// m[k] = v does not panic either.
	ms.AddAll()
	ss.AddAll()
	sm.SetAll()
	vec.AppendAll()
	m.SetAll()

	// Deletes are no-ops, exactly as delete(nilMap, k) is.
	ms.Delete(1)
	ms.DeleteAll(1, 2)
	ss.Delete(1)
	ss.DeleteAll(1, 2)
	sm.Delete(1)
	sm.DeleteAll(1, 2)
	m.Delete(1)
	m.DeleteAll(1, 2)

	// And IsZero distinguishes "never constructed" from "constructed empty",
	// which is the narrower question m == nil answers for a builtin map.
	if !ms.IsZero() || !ss.IsZero() || !sm.IsZero() || !vec.IsZero() || !m.IsZero() {
		t.Error("a zero container should report IsZero")
	}
	if containers.NewMapSet[int]().IsZero() {
		t.Error("a constructed empty container is not zero")
	}
}

// Copies share, which is the whole point of a reference type -- and is what
// noCopy used to forbid.
func TestCopiesShare(t *testing.T) {
	a := containers.NewMapSet(1)
	b := a
	b.Add(2)
	if !a.Has(2) {
		t.Error("a copy of a MapSet should share its contents")
	}

	v := containers.NewVector(1)
	w := v
	w.Append(2)
	if v.Len() != 2 {
		t.Errorf("a copy of a Vector should share: Len=%d, want 2", v.Len())
	}

	// Clone is how you get a separate one.
	c := a.Clone()
	c.Add(99)
	if a.Has(99) {
		t.Error("Clone should be independent")
	}
}

// A set converts to a sorted set by spreading its materialised keys -- problem
// 5 of ADR 0017, which had no spelling before.
func ExampleNewSortedSet() {
	seen := containers.NewMapSet(40, 3, 12, 7)
	sorted := containers.NewSortedSet(seen.KeySlice()...)
	fmt.Println(sorted.KeySlice())
	// Output: [3 7 12 40]
}

// ---------------------------------------------------------------------------
// ADR 0008's layers, as ADR 0013 left them. The read-only tier is gone: it was
// never enforceable, because a container satisfied it structurally and a holder
// could assert back and write. A function that must not write takes a view,
// which is sealed, and a caller holding a container wraps it.
// ---------------------------------------------------------------------------

// countPresent takes a view: it cannot write through s, and it cannot be handed
// a *HashSet by mistake, because that does not compile.
func countPresent[T any](s containers.Keys[T], probes ...T) int {
	n := 0
	for _, p := range probes {
		if s.Has(p) {
			n++
		}
	}
	return n
}

// lookupAll takes the dict view. A HashDict view, a Map view and a SortedDict
// view are all acceptable here, and none of them names an implementation.
func lookupAll[K, V any](d containers.KeyValues[K, V], keys ...K) int {
	n := 0
	for _, k := range keys {
		if _, ok := d.Get(k); ok {
			n++
		}
	}
	return n
}

// The identity wrap is the migration for what the read-only tier used to do,
// and by ADR 0013 decision 7 it allocates nothing.
func TestReadOnlyBoundaryTakesViews(t *testing.T) {
	hs := containers.NewMapSet(1, 2, 3)
	ss := containers.NewSortedSet(1, 2, 3)
	for name, s := range map[string]containers.Keys[int]{
		"HashSet":   hs.View(),
		"SortedSet": ss.View(),
	} {
		if got := countPresent[int](s, 1, 3, 99); got != 2 {
			t.Errorf("%s: countPresent = %d, want 2", name, got)
		}
		if s.Len() != 3 {
			t.Errorf("%s: Len = %d, want 3", name, s.Len())
		}
	}

	m := containers.Map[string, int]{"a": 1, "b": 2}
	sd := containers.NewSortedMap[string, int]()
	sd.SetAll(pairsOf(maps.All(map[string]int{"a": 1, "b": 2}))...)
	hd := containers.Map[string, int]{}
	hd.Set("a", 1)
	hd.Set("b", 2)
	for name, d := range map[string]containers.KeyValues[string, int]{
		"Map":        m.View(),
		"SortedDict": sd.View(),
		"HashDict":   hd.View(),
	} {
		if got := lookupAll[string, int](d, "a", "b", "zz"); got != 2 {
			t.Errorf("%s: lookupAll = %d, want 2", name, got)
		}
	}
}

// Wrapping a container to read from it is free.
//
// Under ADR 0018 a view is a two-word struct holding an interface, so
// CONSTRUCTION allocates nothing -- the cost moved to generalising a view into
// a shape interface, which is what the sinks below do. That is measured
// separately; this test asserts only that building the view is free.
func TestIdentityViewsDoNotAllocate(t *testing.T) {
	hs := containers.NewMapSet(1, 2, 3)
	ss := containers.NewSortedSet(1, 2, 3)
	hd := containers.Map[string, int]{}
	sd := containers.NewSortedMap[string, int]()
	m := containers.Map[string, int]{"a": 1}

	// Sinks are the shape interfaces, because the views are now unrelated
	// concrete types -- MapSetView and SortedSetView do not substitute for one
	// another (ADR 0018 wart 5).
	var sinkMapSet containers.MapSetView[int]
	var sinkSortedSet containers.SortedSetView[int]
	var sinkMap containers.MapView[string, int]
	var sinkSortedMap containers.SortedMapView[string, int]

	for name, f := range map[string]func(){
		"ViewMapSetIdentity":    func() { sinkMapSet = hs.View() },
		"ViewSortedSet":         func() { sinkSortedSet = ss.View() },
		"ViewMapIdentity":       func() { sinkMap = hd.View() },
		"ViewSortedMapIdentity": func() { sinkSortedMap = sd.View() },
		"ViewMapIdentity/again": func() { sinkMap = m.View() },
	} {
		if got := testing.AllocsPerRun(100, f); got != 0 {
			t.Errorf("%s: %v allocs, want 0", name, got)
		}
	}
	_, _, _, _ = sinkMapSet, sinkSortedSet, sinkMap, sinkSortedMap
}

// MutableSet carries Delete, mirroring MutableDict's (ADR 0017 renamed the
// sets' Remove to match, since a set's element is its key).
func TestMutableSetDelete(t *testing.T) {
	drop := func(s interface{ Delete(int) }, v int) { s.Delete(v) }

	hs := containers.NewMapSet(1, 2, 3)
	drop(hs, 2)
	if hs.Has(2) || hs.Len() != 2 {
		t.Errorf("HashSet after Remove: Len=%d Has(2)=%v", hs.Len(), hs.Has(2))
	}

	ss := containers.NewSortedSet(1, 2, 3)
	drop(ss, 2)
	if ss.Has(2) || ss.Len() != 2 {
		t.Errorf("SortedSet after Remove: Len=%d Has(2)=%v", ss.Len(), ss.Has(2))
	}
}
