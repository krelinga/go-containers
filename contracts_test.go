package containers_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/krelinga/go-containers"
)

// Every container satisfies its contract, and the pointer does while the value
// does not -- ADR 0002's uniform pointer receivers.
func TestContractSatisfaction(t *testing.T) {
	var (
		_ containers.MutableSet[int]          = containers.NewHashSet[int]()
		_ containers.MutableSet[int]          = containers.NewSortedSet[int]()
		_ containers.MutableDict[int, string] = containers.NewSortedDict[int, string]()
		_ containers.MutableDict[int, string] = containers.NewHashDict[int, string]()
		_ containers.MutableDict[int, string] = containers.Map[int, string]{}
	)
	var s containers.HashSet[int]
	var _ containers.MutableSet[int] = &s
}

// The *Slice contract, which is the whole of ADR 0017 proposal D: a result is a
// full, independent copy. Nothing the container does afterwards is visible
// through it, and nothing done to it is visible in the container.
func TestSliceResultsAreIndependentCopies(t *testing.T) {
	hs := containers.NewHashSet(1, 2, 3)
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

	sd := containers.NewSortedDict(containers.KeyValue[int, string]{1, "a"})
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

	sd := containers.NewSortedDict[int, string]()
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

// ADR 0002's eager-dereference rule. A bulk call with no arguments must still
// touch the receiver: the rule has been violated three times in this
// repository, every time via a loop that could run zero times.
func TestEmptyBulkCallsStillDereference(t *testing.T) {
	var ps *containers.SortedSet[int]
	var pv *containers.Vector[int]
	var pd *containers.SortedDict[int, string]
	var ph *containers.HashSet[int]
	var phd *containers.HashDict[int, string]

	mustPanic(t, "SortedSet.AddAll()", func() { ps.AddAll() })
	mustPanic(t, "SortedSet.DeleteAll()", func() { ps.DeleteAll() })
	mustPanic(t, "Vector.AppendAll()", func() { pv.AppendAll() })
	mustPanic(t, "SortedDict.SetAll()", func() { pd.SetAll() })
	mustPanic(t, "SortedDict.DeleteAll()", func() { pd.DeleteAll() })
	mustPanic(t, "HashSet.AddAll()", func() { ph.AddAll() })
	mustPanic(t, "HashSet.DeleteAll()", func() { ph.DeleteAll() })
	mustPanic(t, "HashDict.SetAll()", func() { phd.SetAll() })
	mustPanic(t, "HashDict.DeleteAll()", func() { phd.DeleteAll() })
}

// A set converts to a sorted set by spreading its materialised keys -- problem
// 5 of ADR 0017, which had no spelling before.
func ExampleNewSortedSet() {
	seen := containers.NewHashSet(40, 3, 12, 7)
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
func countPresent[T any](s containers.SetView[T], probes ...T) int {
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
func lookupAll[K, V any](d containers.DictView[K, V], keys ...K) int {
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
	hs := containers.NewHashSet(1, 2, 3)
	ss := containers.NewSortedSet(1, 2, 3)
	for name, s := range map[string]containers.SetView[int]{
		"HashSet":   containers.ViewHashSetIdentity(hs),
		"SortedSet": containers.ViewSortedSet(ss),
	} {
		if got := countPresent[int](s, 1, 3, 99); got != 2 {
			t.Errorf("%s: countPresent = %d, want 2", name, got)
		}
		if s.Len() != 3 {
			t.Errorf("%s: Len = %d, want 3", name, s.Len())
		}
	}

	m := containers.Map[string, int]{"a": 1, "b": 2}
	sd := containers.NewSortedDict[string, int]()
	sd.SetAll(pairsOf(maps.All(map[string]int{"a": 1, "b": 2}))...)
	hd := containers.NewHashDict[string, int]()
	hd.Set("a", 1)
	hd.Set("b", 2)
	for name, d := range map[string]containers.DictView[string, int]{
		"Map":        containers.ViewMapIdentity(m),
		"SortedDict": containers.ViewSortedDictIdentity(sd),
		"HashDict":   containers.ViewHashDictIdentity(hd),
	} {
		if got := lookupAll[string, int](d, "a", "b", "zz"); got != 2 {
			t.Errorf("%s: lookupAll = %d, want 2", name, got)
		}
	}
}

// Wrapping a container to read from it is free, which is what makes the line
// above an acceptable replacement for the deleted read-only contract.
func TestIdentityViewsDoNotAllocate(t *testing.T) {
	hs := containers.NewHashSet(1, 2, 3)
	ss := containers.NewSortedSet(1, 2, 3)
	hd := containers.NewHashDict[string, int]()
	sd := containers.NewSortedDict[string, int]()
	m := containers.Map[string, int]{"a": 1}

	var sinkSet containers.SetView[int]
	var sinkDict containers.DictView[string, int]

	for name, f := range map[string]func(){
		"ViewHashSetIdentity":    func() { sinkSet = containers.ViewHashSetIdentity(hs) },
		"ViewSortedSet":          func() { sinkSet = containers.ViewSortedSet(ss) },
		"ViewHashDictIdentity":   func() { sinkDict = containers.ViewHashDictIdentity(hd) },
		"ViewSortedDictIdentity": func() { sinkDict = containers.ViewSortedDictIdentity(sd) },
		"ViewMapIdentity":        func() { sinkDict = containers.ViewMapIdentity(m) },
	} {
		if got := testing.AllocsPerRun(100, f); got != 0 {
			t.Errorf("%s: %v allocs, want 0", name, got)
		}
	}
	_, _ = sinkSet, sinkDict
}

// MutableSet carries Delete, mirroring MutableDict's (ADR 0017 renamed the
// sets' Remove to match, since a set's element is its key).
func TestMutableSetDelete(t *testing.T) {
	drop := func(s containers.MutableSet[int], v int) { s.Delete(v) }

	hs := containers.NewHashSet(1, 2, 3)
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
