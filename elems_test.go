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
		_ containers.Elems[int]          = containers.NewSet[int]()
		_ containers.Elems[int]          = containers.NewSortedSet[int]()
		_ containers.Elems2[int, string] = containers.NewSortedMap[int, string]()
	)
	var s containers.Set[int]
	var _ containers.Elems[int] = &s

	// SetLike is now Elems plus the mutating operations, same method set.
	var _ containers.SetLike[int] = containers.NewSet[int]()
	var _ containers.SetLike[int] = containers.NewSortedSet[int]()
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
		m := containers.CollectSortedMap(src2)
		if got, wantK := orderedKeys(m), []int{1, 5, 9}; !slices.Equal(got, wantK) {
			t.Errorf("CollectSortedMap with Len()=%d = %v, want %v", n, got, wantK)
		}
		var m2 containers.SortedMap[int, string]
		m2.SetAll(src2)
		if got, wantK := orderedKeys(&m2), []int{1, 5, 9}; !slices.Equal(got, wantK) {
			t.Errorf("SetAll with Len()=%d = %v, want %v", n, got, wantK)
		}
	}
}

// The sized and bare forms must produce identical results.
func TestSizedAndSeqFormsAgree(t *testing.T) {
	src := containers.NewSet(5, 1, 9, 3)

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

	msrc := containers.NewSortedMap[int, string]()
	msrc.SetAllSeq(maps.All(map[int]string{2: "b", 1: "a"}))
	if !slices.Equal(orderedKeys(containers.CollectSortedMap(msrc)),
		orderedKeys(containers.CollectSortedMapSeq(msrc.All()))) {
		t.Error("CollectSortedMap and CollectSortedMapSeq disagree")
	}
}

// The point of the change: the sized form allocates fewer times.
func TestSizedFormAllocatesLess(t *testing.T) {
	src := containers.NewSet[int]()
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
	var m containers.SortedMap[int, string]

	mustPanic(t, "AddAll(nil interface)", func() { s.AddAll(nil) })
	mustPanic(t, "SetAll(nil interface)", func() { m.SetAll(nil) })
	mustPanic(t, "CollectSortedSet(nil)", func() { _ = containers.CollectSortedSet[int](nil) })
	mustPanic(t, "CollectSortedMap(nil)", func() { _ = containers.CollectSortedMap[int, string](nil) })

	// A non-nil interface holding a nil pointer panics inside the container.
	var nilSet *containers.Set[int]
	mustPanic(t, "AddAll(typed nil)", func() { s.AddAll(nilSet) })
}

// ADR 0006 consequence: passing a container to itself is well defined.
func TestElemsSelfReference(t *testing.T) {
	s := containers.NewSortedSet(3, 1, 2)
	s.AddAll(s)
	if got, want := ssVals(s), []int{1, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("s.AddAll(s) = %v, want %v", got, want)
	}

	m := containers.NewSortedMap[int, string]()
	m.SetAllSeq(maps.All(map[int]string{1: "a", 2: "b"}))
	m.SetAll(m)
	if got, want := orderedKeys(m), []int{1, 2}; !slices.Equal(got, want) {
		t.Errorf("m.SetAll(m) = %v, want %v", got, want)
	}
}

// A Set converts to a SortedSet without the caller mentioning iterators, and
// the length comes along for free.
func ExampleCollectSortedSet() {
	seen := containers.NewSet(40, 3, 12, 7)
	sorted := containers.CollectSortedSet(seen)
	fmt.Println(slices.Collect(sorted.All()))
	// Output: [3 7 12 40]
}
