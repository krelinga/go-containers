package containers_test

import (
	"slices"
	"testing"

	"github.com/krelinga/go-containers"
)

func TestVectorZeroValueIsUsable(t *testing.T) {
	var v containers.Vector[string]
	if v.Len() != 0 {
		t.Errorf("Len = %d, want 0", v.Len())
	}
	if got := v.ValueSlice(); len(got) != 0 {
		t.Errorf("All = %v, want empty", got)
	}
	v.Append("read")
	v.Append("write")
	if got := v.ValueSlice(); !slices.Equal(got, []string{"read", "write"}) {
		t.Errorf("All = %v", got)
	}
}

func TestVectorOrderAndIndexing(t *testing.T) {
	v := containers.NewVector(3, 1, 2)
	if got := v.ValueSlice(); !slices.Equal(got, []int{3, 1, 2}) {
		t.Errorf("All = %v, want insertion order", got)
	}
	if v.At(0) != 3 || v.At(2) != 2 {
		t.Errorf("At: got %d,%d", v.At(0), v.At(2))
	}
	v.Set(1, 99)
	if v.At(1) != 99 {
		t.Errorf("Set then At = %d, want 99", v.At(1))
	}

	var gotIdx []int
	var gotVal []int
	for i, e := range v.All() {
		gotIdx = append(gotIdx, i)
		gotVal = append(gotVal, e)
	}
	if !slices.Equal(gotIdx, []int{0, 1, 2}) || !slices.Equal(gotVal, []int{3, 99, 2}) {
		t.Errorf("AllIndexed = %v/%v", gotIdx, gotVal)
	}
}

// At and Set panic out of range, exactly as indexing a slice does (ADR 0015
// decision 7).
func TestVectorIndexOutOfRangePanics(t *testing.T) {
	v := containers.NewVector(1)
	mustPanic(t, "At(-1)", func() { _ = v.At(-1) })
	mustPanic(t, "At(1)", func() { _ = v.At(1) })
	mustPanic(t, "Set(1)", func() { v.Set(1, 0) })
}

func TestVectorBulkAppend(t *testing.T) {
	src := containers.NewVector("a", "b")

	var v containers.Vector[string]
	v.Append("first")
	v.AppendAll(src.ValueSlice()...)
	v.AppendAll(slices.Collect(slices.Values([]string{"c"}))...)

	want := []string{"first", "a", "b", "c"}
	if got := v.ValueSlice(); !slices.Equal(got, want) {
		t.Errorf("All = %v, want %v", got, want)
	}
}

// A Vector is built from another container by spreading its materialised
// elements -- problem 5 of ADR 0017.
func TestNewVectorFromAnotherContainer(t *testing.T) {
	src := containers.NewHashSet(1, 2, 3)
	v := containers.NewVector(src.KeySlice()...)
	if v.Len() != 3 {
		t.Errorf("Len = %d, want 3", v.Len())
	}
	if got := slices.Sorted(slices.Values(v.ValueSlice())); !slices.Equal(got, []int{1, 2, 3}) {
		t.Errorf("All = %v", got)
	}

	fromSeq := containers.NewVector(slices.Collect(slices.Values([]int{4, 5}))...)
	if got := fromSeq.ValueSlice(); !slices.Equal(got, []int{4, 5}) {
		t.Errorf("CollectVectorSeq = %v", got)
	}
}

// NewVector copies its argument, so the caller's slice is not aliased.
func TestVectorDoesNotAliasItsArgument(t *testing.T) {
	src := []int{1, 2, 3}
	v := containers.NewVector(src...)
	src[0] = 99
	if v.At(0) != 1 {
		t.Errorf("NewVector aliased its argument: At(0) = %d", v.At(0))
	}
}

func TestVectorCloneIsIndependent(t *testing.T) {
	a := containers.NewVector(1, 2)
	b := a.Clone()
	b.Append(3)
	b.Set(0, 99)

	if a.Len() != 2 || a.At(0) != 1 {
		t.Errorf("Clone shares state: a = %v", a.ValueSlice())
	}
}

// The reason the container exists: a reallocation is not observable, and an
// append made through one holder is seen by every other.
func TestVectorHidesReallocation(t *testing.T) {
	v := containers.NewVector[int]()
	alias := v

	appendMany(v, 8) // reallocates several times

	if v.Len() != 8 || alias.Len() != 8 {
		t.Errorf("holders diverged: v=%d alias=%d", v.Len(), alias.Len())
	}
	if !slices.Equal(v.ValueSlice(), alias.ValueSlice()) {
		t.Error("holders see different contents")
	}
}

func appendMany(v *containers.Vector[int], n int) {
	for i := range n {
		v.Append(i)
	}
}

// ADR 0002: every method panics on a nil receiver, and none special-cases it.
func TestVectorNilReceiverPanics(t *testing.T) {
	var v *containers.Vector[int]
	src := containers.NewVector(1)

	mustPanic(t, "Len", func() { _ = v.Len() })
	mustPanic(t, "At", func() { _ = v.At(0) })
	mustPanic(t, "Set", func() { v.Set(0, 1) })
	mustPanic(t, "Append", func() { v.Append(1) })
	mustPanic(t, "AppendAll", func() { v.AppendAll(src.ValueSlice()...) })
	mustPanic(t, "All", func() { _ = v.All() })
	mustPanic(t, "AllIndexed", func() { _ = v.All() })
	mustPanic(t, "Clone", func() { _ = v.Clone() })
}

// ADR 0002's eager-dereference rule: the cases that skip the panic when a loop
// or a closure never runs. Violated three times in this package's history, so
// asserted rather than assumed.
func TestVectorDereferencesEagerly(t *testing.T) {
	var v *containers.Vector[int]

	// An empty iterator must not let AppendAllSeq off.
	mustPanic(t, "AppendAllSeq with an empty seq", func() {
		v.AppendAll(slices.Collect(slices.Values([]int(nil)))...)
	})
	// A usable receiver with no arguments must NOT panic: the rule is about the
	// receiver, and a bulk call now takes a slice rather than an interface that
	// could be nil.
	good := containers.NewVector(1)
	good.AppendAll()
	if good.Len() != 1 {
		t.Errorf("AppendAll() changed the vector: Len=%d", good.Len())
	}
	// All must panic at the call, not at iteration.
	mustPanic(t, "Values", func() { _ = v.Values() })
	mustPanic(t, "All", func() { _ = v.All() })
	mustPanic(t, "ValueSlice", func() { _ = v.ValueSlice() })
}

// A Vector reaches the IndexedView contract through its view constructor, not
// directly: views are sealed (ADR 0013).
func TestVectorSatisfiesIndexedViewThroughItsView(t *testing.T) {
	var _ containers.IndexedView[int] = containers.ViewVectorIdentity(containers.NewVector(1))
}
