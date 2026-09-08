package containers_test

import (
	"fmt"
	"iter"
	"maps"
	"slices"
	"testing"

	"github.com/krelinga/go-containers"
)

// Every container satisfies the contract unchanged, and the pointer does while
// the value does not -- ADR 0002's uniform pointer receivers.
func TestElemsSatisfaction(t *testing.T) {
	var (
		_ containers.Elems[int]          = containers.NewHashSet[int]()
		_ containers.Elems[int]          = containers.NewSortedSet[int]()
		_ containers.Elems2[int, string] = containers.NewSortedDict[int, string]()
	)
	var s containers.HashSet[int]
	var _ containers.Elems[int] = &s

	// MutableSet is now Elems plus the mutating operations, same method set.
	var _ containers.MutableSet[int] = containers.NewHashSet[int]()
	var _ containers.MutableSet[int] = containers.NewSortedSet[int]()
}

// lyingElems reports a length that does not match what it yields. ADR 0006
// decision 3: Len is a capacity hint, never a correctness input.
type lyingElems struct {
	vals []int
	n    int
}

func (l lyingElems) Len() int           { return l.n }
func (l lyingElems) All() iter.Seq[int] { return slices.Values(l.vals) }

type lyingElems2 struct {
	vals map[int]string
	n    int
}

func (l lyingElems2) Len() int                    { return l.n }
func (l lyingElems2) All() iter.Seq2[int, string] { return maps.All(l.vals) }

func TestElemsLenIsOnlyAHint(t *testing.T) {
	vals := []int{5, 1, 9, 1, 3}
	want := []int{1, 3, 5, 9}

	for _, n := range []int{0, 1, 2, 5, 1000, -7} {
		src := lyingElems{vals: vals, n: n}

		if got := ssVals(containers.CollectSortedSet(src)); !slices.Equal(got, want) {
			t.Errorf("CollectSortedSet with Len()=%d = %v, want %v", n, got, want)
		}

		var s containers.SortedSet[int]
		s.AddAll(src)
		if got := ssVals(&s); !slices.Equal(got, want) {
			t.Errorf("AddAll with Len()=%d = %v, want %v", n, got, want)
		}

		src2 := lyingElems2{vals: map[int]string{5: "e", 1: "a", 9: "i"}, n: n}
		m := containers.CollectSortedDict(src2)
		if got, wantK := orderedKeys(m), []int{1, 5, 9}; !slices.Equal(got, wantK) {
			t.Errorf("CollectSortedDict with Len()=%d = %v, want %v", n, got, wantK)
		}
		var m2 containers.SortedDict[int, string]
		m2.SetAll(src2)
		if got, wantK := orderedKeys(&m2), []int{1, 5, 9}; !slices.Equal(got, wantK) {
			t.Errorf("SetAll with Len()=%d = %v, want %v", n, got, wantK)
		}
	}
}

// The sized and bare forms must produce identical results.
func TestSizedAndSeqFormsAgree(t *testing.T) {
	src := containers.NewHashSet(5, 1, 9, 3)

	sized := containers.CollectSortedSet(src)
	viaSeq := containers.CollectSortedSetSeq(src.All())
	if !slices.Equal(ssVals(sized), ssVals(viaSeq)) {
		t.Errorf("CollectSortedSet %v vs Seq %v", ssVals(sized), ssVals(viaSeq))
	}

	var a, b containers.SortedSet[int]
	a.AddAll(src)
	b.AddAllSeq(src.All())
	if !slices.Equal(ssVals(&a), ssVals(&b)) {
		t.Errorf("AddAll %v vs AddAllSeq %v", ssVals(&a), ssVals(&b))
	}

	msrc := containers.NewSortedDict[int, string]()
	msrc.SetAllSeq(maps.All(map[int]string{2: "b", 1: "a"}))
	if !slices.Equal(orderedKeys(containers.CollectSortedDict(msrc)),
		orderedKeys(containers.CollectSortedDictSeq(msrc.All()))) {
		t.Error("CollectSortedDict and CollectSortedDictSeq disagree")
	}
}

// The point of the change: the sized form allocates fewer times.
func TestSizedFormAllocatesLess(t *testing.T) {
	src := containers.NewHashSet[int]()
	for i := range 4096 {
		src.Add(i)
	}
	sized := testing.AllocsPerRun(20, func() {
		_ = containers.CollectSortedSet(src)
	})
	unsized := testing.AllocsPerRun(20, func() {
		_ = containers.CollectSortedSetSeq(src.All())
	})
	t.Logf("allocations: sized=%.0f unsized=%.0f", sized, unsized)
	if sized >= unsized {
		t.Errorf("sized form allocated %.0f times, unsized %.0f; expected fewer", sized, unsized)
	}
}

// ADR 0002's nil-argument rule extends to interface arguments.
func TestElemsNilArgumentPanics(t *testing.T) {
	var s containers.SortedSet[int]
	var m containers.SortedDict[int, string]

	mustPanic(t, "AddAll(nil interface)", func() { s.AddAll(nil) })
	mustPanic(t, "SetAll(nil interface)", func() { m.SetAll(nil) })
	mustPanic(t, "CollectSortedSet(nil)", func() { _ = containers.CollectSortedSet[int](nil) })
	mustPanic(t, "CollectSortedDict(nil)", func() { _ = containers.CollectSortedDict[int, string](nil) })

	// A non-nil interface holding a nil pointer panics inside the container.
	var nilSet *containers.HashSet[int]
	mustPanic(t, "AddAll(typed nil)", func() { s.AddAll(nilSet) })
}

// ADR 0006 consequence: passing a container to itself is well defined.
func TestElemsSelfReference(t *testing.T) {
	s := containers.NewSortedSet(3, 1, 2)
	s.AddAll(s)
	if got, want := ssVals(s), []int{1, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("s.AddAll(s) = %v, want %v", got, want)
	}

	m := containers.NewSortedDict[int, string]()
	m.SetAllSeq(maps.All(map[int]string{1: "a", 2: "b"}))
	m.SetAll(m)
	if got, want := orderedKeys(m), []int{1, 2}; !slices.Equal(got, want) {
		t.Errorf("m.SetAll(m) = %v, want %v", got, want)
	}
}

// A Set converts to a SortedSet without the caller mentioning iterators, and
// the length comes along for free.
func ExampleCollectSortedSet() {
	seen := containers.NewHashSet(40, 3, 12, 7)
	sorted := containers.CollectSortedSet(seen)
	fmt.Println(slices.Collect(sorted.All()))
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
	sd.SetAllSeq(maps.All(map[string]int{"a": 1, "b": 2}))
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

// MutableSet gained Remove in ADR 0008, mirroring MutableDict's Delete.
func TestMutableSetRemove(t *testing.T) {
	drop := func(s containers.MutableSet[int], vs ...int) { s.Remove(vs...) }

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
