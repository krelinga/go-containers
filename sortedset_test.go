package containers_test

import (
	"slices"
	"testing"

	"github.com/krelinga/go-containers"
)

func ssVals(s *containers.SortedSet[int]) []int { return slices.Collect(s.All()) }

func TestSortedSetZeroValue(t *testing.T) {
	var s containers.SortedSet[int]
	if s.Len() != 0 || s.Has(1) {
		t.Errorf("fresh zero value: Len=%d Has=%v", s.Len(), s.Has(1))
	}
	if _, ok := s.Min(); ok {
		t.Error("Min on zero value returned ok")
	}
	if got := ssVals(&s); got != nil {
		t.Errorf("All on zero value = %v, want empty", got)
	}
	s.Add(10, 5)
	if got, want := ssVals(&s), []int{5, 10}; !slices.Equal(got, want) {
		t.Errorf("after Add = %v, want %v", got, want)
	}
}

// ADR 0002 decision 2, inherited: every method panics on a nil receiver, on a
// path that always executes.
func TestSortedSetNilPanicsUniformly(t *testing.T) {
	var p *containers.SortedSet[int]
	other := containers.NewSortedSet(1)

	mustPanic(t, "Add", func() { p.Add(1) })
	mustPanic(t, "Add() no args", func() { p.Add() })
	mustPanic(t, "Add multi", func() { p.Add(1, 2) })
	mustPanic(t, "AddAll", func() { p.AddAll(slices.Values([]int{1})) })
	mustPanic(t, "AddAll empty", func() { p.AddAll(func(func(int) bool) {}) })
	mustPanic(t, "Remove", func() { p.Remove(1) })
	mustPanic(t, "Remove() no args", func() { p.Remove() })
	mustPanic(t, "Has", func() { _ = p.Has(1) })
	mustPanic(t, "Len", func() { _ = p.Len() })
	mustPanic(t, "All", func() { _ = p.All() })
	mustPanic(t, "Range", func() { _ = p.Range(1, 2) })
	mustPanic(t, "Floor", func() { _, _ = p.Floor(1) })
	mustPanic(t, "Ceil", func() { _, _ = p.Ceil(1) })
	mustPanic(t, "Min", func() { _, _ = p.Min() })
	mustPanic(t, "Max", func() { _, _ = p.Max() })
	mustPanic(t, "Clone", func() { _ = p.Clone() })
	mustPanic(t, "Union", func() { _ = p.Union(other) })
	mustPanic(t, "Intersect", func() { _ = p.Intersect(other) })
	mustPanic(t, "Difference", func() { _ = p.Difference(other) })
}

func TestSortedSetNilArgumentPanics(t *testing.T) {
	var nilSet *containers.SortedSet[int]
	full := containers.NewSortedSet(1)
	var empty containers.SortedSet[int]

	for _, tc := range []struct {
		name string
		recv *containers.SortedSet[int]
	}{{"full", full}, {"empty", &empty}} {
		mustPanic(t, tc.name+".Union(nil)", func() { _ = tc.recv.Union(nilSet) })
		mustPanic(t, tc.name+".Intersect(nil)", func() { _ = tc.recv.Intersect(nilSet) })
		mustPanic(t, tc.name+".Difference(nil)", func() { _ = tc.recv.Difference(nilSet) })
	}
}

// Both Add paths -- single insert and multi-value merge -- must agree.
func TestSortedSetAddPathsAgree(t *testing.T) {
	one := containers.NewSortedSet[int]()
	for _, v := range []int{40, 3, 12, 7, 3, 40} {
		one.Add(v)
	}
	multi := containers.NewSortedSet[int]()
	multi.Add(40, 3, 12, 7, 3, 40)
	viaAddAll := containers.NewSortedSet[int]()
	viaAddAll.AddAll(slices.Values([]int{40, 3, 12, 7, 3, 40}))
	collected := containers.CollectSortedSet(slices.Values([]int{40, 3, 12, 7, 3, 40}))

	want := []int{3, 7, 12, 40}
	for _, tc := range []struct {
		name string
		s    *containers.SortedSet[int]
	}{{"repeated Add", one}, {"Add multi", multi}, {"AddAll", viaAddAll}, {"Collect", collected}} {
		if got := ssVals(tc.s); !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", tc.name, got, want)
		}
	}
}

func TestSortedSetAddAllMergesWithExisting(t *testing.T) {
	s := containers.NewSortedSet(10, 30, 50)
	s.AddAll(slices.Values([]int{20, 30, 60}))
	if got, want := ssVals(s), []int{10, 20, 30, 50, 60}; !slices.Equal(got, want) {
		t.Errorf("= %v, want %v", got, want)
	}

	var z containers.SortedSet[int]
	z.AddAll(func(func(int) bool) {})
	if z.Len() != 0 {
		t.Error("empty AddAll on zero value should stay empty")
	}
}

func TestSortedSetRemove(t *testing.T) {
	s := containers.NewSortedSet(1, 2, 3)
	s.Remove(2, 99)
	if got, want := ssVals(s), []int{1, 3}; !slices.Equal(got, want) {
		t.Errorf("= %v, want %v", got, want)
	}
	var z containers.SortedSet[int]
	z.Remove(1) // no-op, not a panic
}

func TestSortedSetOrderedLookups(t *testing.T) {
	s := containers.NewSortedSet(3, 7, 12, 40)

	for _, tc := range []struct {
		in     int
		want   int
		wantOK bool
	}{{1, 0, false}, {3, 3, true}, {9, 7, true}, {40, 40, true}, {999, 40, true}} {
		if got, ok := s.Floor(tc.in); got != tc.want || ok != tc.wantOK {
			t.Errorf("Floor(%d) = %d,%v want %d,%v", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
	for _, tc := range []struct {
		in     int
		want   int
		wantOK bool
	}{{1, 3, true}, {3, 3, true}, {9, 12, true}, {40, 40, true}, {41, 0, false}} {
		if got, ok := s.Ceil(tc.in); got != tc.want || ok != tc.wantOK {
			t.Errorf("Ceil(%d) = %d,%v want %d,%v", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
	if v, ok := s.Min(); v != 3 || !ok {
		t.Errorf("Min = %d,%v", v, ok)
	}
	if v, ok := s.Max(); v != 40 || !ok {
		t.Errorf("Max = %d,%v", v, ok)
	}
}

// ADR 0005: inverted bounds are empty, NOT a panic -- unlike the hand-rolled
// baseline in callsites_test.go, which reslices between two searches.
func TestSortedSetRange(t *testing.T) {
	s := containers.NewSortedSet(3, 7, 12, 40)
	for _, tc := range []struct {
		name   string
		lo, hi int
		want   []int
	}{
		{"middle", 7, 40, []int{7, 12}},
		{"all", 0, 100, []int{3, 7, 12, 40}},
		{"empty span", 4, 6, nil},
		{"above everything", 50, 99, nil},
		{"lower inclusive, upper exclusive", 40, 41, []int{40}},
		{"inverted is empty, not a panic", 40, 7, nil},
		{"equal bounds", 7, 7, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := slices.Collect(s.Range(tc.lo, tc.hi)); !slices.Equal(got, tc.want) {
				t.Errorf("Range(%d,%d) = %v, want %v", tc.lo, tc.hi, got, tc.want)
			}
		})
	}
}

func TestSortedSetIteratorsBindAtCallTime(t *testing.T) {
	var s containers.SortedSet[int]
	s.Add(10)
	all, rng := s.All(), s.Range(0, 100)
	s.Add(20)

	if got := slices.Collect(all); !slices.Equal(got, []int{10}) {
		t.Errorf("All saw post-call writes: %v", got)
	}
	if got := slices.Collect(rng); !slices.Equal(got, []int{10}) {
		t.Errorf("Range saw post-call writes: %v", got)
	}
}

func TestSortedSetAlgebra(t *testing.T) {
	a := containers.NewSortedSet(1, 3, 5, 7)
	b := containers.NewSortedSet(3, 7, 9)

	for _, tc := range []struct {
		name string
		got  *containers.SortedSet[int]
		want []int
	}{
		{"Union", a.Union(b), []int{1, 3, 5, 7, 9}},
		{"Intersect", a.Intersect(b), []int{3, 7}},
		{"Difference", a.Difference(b), []int{1, 5}},
		{"Union is symmetric", b.Union(a), []int{1, 3, 5, 7, 9}},
		{"Intersect is symmetric", b.Intersect(a), []int{3, 7}},
		{"reverse Difference", b.Difference(a), []int{9}},
	} {
		if got := ssVals(tc.got); !slices.Equal(got, tc.want) {
			t.Errorf("%s = %v, want %v", tc.name, got, tc.want)
		}
	}

	// Operands unchanged, and the result independent of both.
	if a.Len() != 4 || b.Len() != 3 {
		t.Errorf("algebra mutated operands: %v %v", ssVals(a), ssVals(b))
	}
	u := a.Union(b)
	u.Add(99)
	u.Remove(1)
	if !a.Has(1) || b.Has(99) {
		t.Error("mutating the result leaked into an operand")
	}

	// Empty operands.
	var empty containers.SortedSet[int]
	if got := ssVals(a.Union(&empty)); !slices.Equal(got, []int{1, 3, 5, 7}) {
		t.Errorf("Union with empty = %v", got)
	}
	if a.Intersect(&empty).Len() != 0 {
		t.Error("Intersect with empty should be empty")
	}
	if got := ssVals(a.Difference(&empty)); !slices.Equal(got, []int{1, 3, 5, 7}) {
		t.Errorf("Difference with empty = %v", got)
	}
}

func TestSortedSetCloneIsIndependent(t *testing.T) {
	orig := containers.NewSortedSet(1, 2)
	clone := orig.Clone()
	clone.Add(3)
	orig.Remove(1)

	if got, want := ssVals(orig), []int{2}; !slices.Equal(got, want) {
		t.Errorf("orig = %v, want %v", got, want)
	}
	if got, want := ssVals(clone), []int{1, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("clone = %v, want %v", got, want)
	}
	var zero containers.SortedSet[int]
	if zero.Clone().Len() != 0 {
		t.Error("Clone of zero value should be empty")
	}
}
