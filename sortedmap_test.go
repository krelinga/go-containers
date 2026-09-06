package containers_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/krelinga/go-containers"
)

func keysOf(m *containers.SortedMap[int, string]) []int {
	return slices.Collect(maps.Keys(maps.Collect(m.All())))
}

func orderedKeys(m *containers.SortedMap[int, string]) []int {
	var out []int
	for k := range m.All() {
		out = append(out, k)
	}
	return out
}

// ADR 0002 decision 2, inherited by 0003: the zero value is usable.
func TestSortedMapZeroValue(t *testing.T) {
	var m containers.SortedMap[int, string]

	if m.Len() != 0 {
		t.Errorf("fresh zero value Len = %d, want 0", m.Len())
	}
	if _, ok := m.Get(1); ok {
		t.Error("Get on zero value returned ok")
	}
	if _, _, ok := m.Min(); ok {
		t.Error("Min on zero value returned ok")
	}
	if _, _, ok := m.Max(); ok {
		t.Error("Max on zero value returned ok")
	}
	if got := orderedKeys(&m); got != nil {
		t.Errorf("All on zero value = %v, want empty", got)
	}

	m.Set(10, "0.90")
	if v, ok := m.Get(10); !ok || v != "0.90" {
		t.Errorf("after Set: %v,%v", v, ok)
	}
}

// ADR 0002 decision 2, inherited by 0003: every method panics on a nil
// receiver, on a path that always executes.
func TestSortedMapNilPanicsUniformly(t *testing.T) {
	var p *containers.SortedMap[int, string]

	mustPanic(t, "Set", func() { p.Set(1, "a") })
	mustPanic(t, "Get", func() { _, _ = p.Get(1) })
	mustPanic(t, "Delete", func() { _ = p.Delete(1) })
	mustPanic(t, "Len", func() { _ = p.Len() })
	mustPanic(t, "All", func() { _ = p.All() })
	mustPanic(t, "Range", func() { _ = p.Range(1, 2) })
	mustPanic(t, "Floor", func() { _, _, _ = p.Floor(1) })
	mustPanic(t, "Ceil", func() { _, _, _ = p.Ceil(1) })
	mustPanic(t, "Min", func() { _, _, _ = p.Min() })
	mustPanic(t, "Max", func() { _, _, _ = p.Max() })
	mustPanic(t, "Clone", func() { _ = p.Clone() })
}

func TestSortedMapOrderingAndSet(t *testing.T) {
	m := containers.NewSortedMap[int, string]()
	for _, k := range []int{50, 1, 100, 10} { // deliberately out of order
		m.Set(k, "v")
	}
	if got, want := orderedKeys(m), []int{1, 10, 50, 100}; !slices.Equal(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}

	m.Set(50, "replaced")
	if v, _ := m.Get(50); v != "replaced" {
		t.Errorf("Set did not replace: %q", v)
	}
	if m.Len() != 4 {
		t.Errorf("replace changed Len to %d, want 4", m.Len())
	}

	if !m.Delete(50) {
		t.Error("Delete(50) reported not present")
	}
	if m.Delete(50) {
		t.Error("second Delete(50) reported present")
	}
	if got, want := orderedKeys(m), []int{1, 10, 100}; !slices.Equal(got, want) {
		t.Errorf("after Delete keys = %v, want %v", got, want)
	}
	_ = keysOf(m)
}

// The boundary conditions the container exists to get right once. ADR 0003
// names the `if !found { i-- }` adjustment as the bug hand-rolled versions hit.
func TestSortedMapOrderedLookups(t *testing.T) {
	m := containers.NewSortedMap[int, string]()
	for _, e := range []struct {
		k int
		v string
	}{{1, "a"}, {10, "b"}, {50, "c"}, {100, "d"}} {
		m.Set(e.k, e.v)
	}

	floor := []struct {
		qty    int
		wantK  int
		wantOK bool
	}{
		{0, 0, false}, // below everything
		{1, 1, true},  // exact, first
		{7, 1, true},  // between
		{50, 50, true},
		{999, 100, true}, // above everything
	}
	for _, tc := range floor {
		if k, _, ok := m.Floor(tc.qty); k != tc.wantK || ok != tc.wantOK {
			t.Errorf("Floor(%d) = %d,%v want %d,%v", tc.qty, k, ok, tc.wantK, tc.wantOK)
		}
	}

	ceil := []struct {
		qty    int
		wantK  int
		wantOK bool
	}{
		{0, 1, true},
		{1, 1, true}, // exact
		{7, 10, true},
		{100, 100, true},
		{101, 0, false}, // above everything
	}
	for _, tc := range ceil {
		if k, _, ok := m.Ceil(tc.qty); k != tc.wantK || ok != tc.wantOK {
			t.Errorf("Ceil(%d) = %d,%v want %d,%v", tc.qty, k, ok, tc.wantK, tc.wantOK)
		}
	}

	if k, _, ok := m.Min(); k != 1 || !ok {
		t.Errorf("Min = %d,%v want 1,true", k, ok)
	}
	if k, _, ok := m.Max(); k != 100 || !ok {
		t.Errorf("Max = %d,%v want 100,true", k, ok)
	}

	// Single-entry map: Min and Max are the same entry.
	one := containers.NewSortedMap[int, string]()
	one.Set(5, "x")
	kmin, _, _ := one.Min()
	kmax, _, _ := one.Max()
	if kmin != 5 || kmax != 5 {
		t.Errorf("single entry Min=%d Max=%d, want 5,5", kmin, kmax)
	}
}

func TestSortedMapRange(t *testing.T) {
	m := containers.NewSortedMap[int, string]()
	for _, k := range []int{1, 10, 50, 100} {
		m.Set(k, "v")
	}
	collect := func(lo, hi int) []int {
		var out []int
		for k := range m.Range(lo, hi) {
			out = append(out, k)
		}
		return out
	}

	cases := []struct {
		name   string
		lo, hi int
		want   []int
	}{
		{"middle", 10, 100, []int{10, 50}},
		{"all", 0, 1000, []int{1, 10, 50, 100}},
		{"empty span", 2, 9, nil},
		{"above everything", 500, 1000, nil},
		{"lower bound inclusive, upper exclusive", 100, 101, []int{100}},
		{"inverted bounds are empty, not a panic", 100, 10, nil},
		{"equal bounds", 10, 10, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := collect(tc.lo, tc.hi); !slices.Equal(got, tc.want) {
				t.Errorf("Range(%d,%d) = %v, want %v", tc.lo, tc.hi, got, tc.want)
			}
		})
	}
}

// ADR 0002: iterators bind at call time, not iteration time.
func TestSortedMapIteratorsBindAtCallTime(t *testing.T) {
	var m containers.SortedMap[int, string]
	m.Set(10, "a")

	all := m.All()
	rng := m.Range(0, 100)
	m.Set(20, "b") // may reallocate the backing array

	var allKeys []int
	for k := range all {
		allKeys = append(allKeys, k)
	}
	if got, want := allKeys, []int{10}; !slices.Equal(got, want) {
		t.Errorf("All saw post-call writes: %v, want %v", got, want)
	}
	var rngKeys []int
	for k := range rng {
		rngKeys = append(rngKeys, k)
	}
	if got, want := rngKeys, []int{10}; !slices.Equal(got, want) {
		t.Errorf("Range saw post-call writes: %v, want %v", got, want)
	}
}

func TestSortedMapCloneIsIndependent(t *testing.T) {
	orig := containers.NewSortedMap[int, string]()
	orig.Set(1, "a")
	orig.Set(2, "b")

	clone := orig.Clone()
	clone.Set(3, "c")
	clone.Set(1, "changed")
	orig.Delete(2)

	if got, want := orderedKeys(orig), []int{1}; !slices.Equal(got, want) {
		t.Errorf("orig keys = %v, want %v", got, want)
	}
	if v, _ := orig.Get(1); v != "a" {
		t.Errorf("clone mutation leaked into orig: %q", v)
	}
	if got, want := orderedKeys(clone), []int{1, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("clone keys = %v, want %v", got, want)
	}

	var zero containers.SortedMap[int, string]
	if zero.Clone().Len() != 0 {
		t.Error("Clone of zero value should be empty")
	}
}
