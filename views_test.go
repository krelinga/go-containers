package containers_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/krelinga/go-containers"
)

type item struct{ Name string }

// itemView is the element-level read-only type. ADR 0011: the library cannot
// invent this; the caller who owns item must write it.
type itemView struct{ i *item }

func (v itemView) Name() string { return v.i.Name }

func projectItem(i *item) itemView { return itemView{i} }

// The whole point: a view cannot be asserted back to its container.
func TestViewBlocksDowncast(t *testing.T) {
	d := containers.NewHashDict[string, int]()
	d.Set("a", 1)

	// A bare contract does not protect anything.
	var bare containers.Dict[string, int] = d
	if c, ok := bare.(*containers.HashDict[string, int]); ok {
		c.Set("b", 2)
	}
	if d.Len() != 2 {
		t.Error("expected the bare contract to permit mutation, for contrast")
	}

	// A view does.
	var sealed any = d.View()
	if _, ok := sealed.(*containers.HashDict[string, int]); ok {
		t.Error("view was assertable back to its container")
	}
}

// ADR 0011: a projection denies writes through the view.
func TestViewProjectsElements(t *testing.T) {
	d := containers.NewHashDict[string, *item]()
	d.Set("a", &item{Name: "orig"})

	shallow := d.View()
	raw, _ := shallow.Get("a")
	raw.Name = "mutated" // the shallow view hands back the mutable element
	if got, _ := d.Get("a"); got.Name != "mutated" {
		t.Error("shallow view should hand back the mutable element")
	}

	pv := containers.ViewHashDict(d, projectItem)
	iv, ok := pv.Get("a")
	if !ok || iv.Name() != "mutated" {
		t.Errorf("projected Get = %v,%v", iv, ok)
	}
	// iv exposes only Name(); there is no path from it to the *item's fields.
}

func TestViewsForwardReads(t *testing.T) {
	hs := containers.NewHashSet(1, 2, 3)
	if v := hs.View(); v.Len() != 3 || !v.Has(2) || len(slices.Sorted(v.All())) != 3 {
		t.Errorf("HashSetView: Len=%d Has(2)=%v", v.Len(), v.Has(2))
	}

	ss := containers.NewSortedSet(1, 2, 3)
	sv := ss.View()
	if mn, ok := sv.Min(); mn != 1 || !ok {
		t.Errorf("SortedSetView.Min = %v,%v", mn, ok)
	}
	if mx, ok := sv.Max(); mx != 3 || !ok {
		t.Errorf("SortedSetView.Max = %v,%v", mx, ok)
	}
	if f, ok := sv.Floor(2); f != 2 || !ok {
		t.Errorf("SortedSetView.Floor(2) = %v,%v", f, ok)
	}
	if c, ok := sv.Ceil(0); c != 1 || !ok {
		t.Errorf("SortedSetView.Ceil(0) = %v,%v", c, ok)
	}
	if got, want := slices.Collect(sv.Range(2, 4)), []int{2, 3}; !slices.Equal(got, want) {
		t.Errorf("SortedSetView.Range = %v, want %v", got, want)
	}

	sd := containers.NewSortedDict[int, string]()
	sd.SetAllSeq(maps.All(map[int]string{1: "a", 2: "b", 3: "c"}))
	dv := sd.View()
	if k, val, ok := dv.Min(); k != 1 || val != "a" || !ok {
		t.Errorf("SortedDictView.Min = %v,%v,%v", k, val, ok)
	}
	if k, val, ok := dv.Ceil(2); k != 2 || val != "b" || !ok {
		t.Errorf("SortedDictView.Ceil = %v,%v,%v", k, val, ok)
	}
	if got, want := slices.Sorted(maps.Keys(maps.Collect(dv.Range(2, 4)))), []int{2, 3}; !slices.Equal(got, want) {
		t.Errorf("SortedDictView.Range = %v, want %v", got, want)
	}

	m := containers.Map[string, int]{"x": 9}
	mv := m.View()
	if got, ok := mv.Get("x"); got != 9 || !ok || mv.Len() != 1 {
		t.Errorf("MapView.Get = %v,%v Len=%d", got, ok, mv.Len())
	}
}

// A miss must produce the zero projected value, not call the projection.
func TestViewMissDoesNotProject(t *testing.T) {
	d := containers.NewHashDict[string, *item]()
	called := false
	pv := containers.ViewHashDict(d, func(i *item) itemView { called = true; return itemView{i} })
	got, ok := pv.Get("absent")
	if ok || got != (itemView{}) {
		t.Errorf("miss = %v,%v; want zero,false", got, ok)
	}
	if called {
		t.Error("projection was called for a missing key")
	}
}

// ADR 0002's rules: a zero view panics, on a path that always executes.
func TestZeroViewPanics(t *testing.T) {
	var hsv containers.HashSetView[int, int]
	var sdv containers.SortedDictView[int, string, string]

	mustPanic(t, "HashSetView.Len", func() { _ = hsv.Len() })
	mustPanic(t, "HashSetView.All", func() { _ = hsv.All() })
	mustPanic(t, "SortedDictView.Get", func() { _, _ = sdv.Get(1) })
	mustPanic(t, "SortedDictView.Min", func() { _, _, _ = sdv.Min() })
	mustPanic(t, "SortedDictView.Range", func() { _ = sdv.Range(1, 2) })
}

// Identity views satisfy the read contracts, so they compose with generic code.
func TestIdentityViewsSatisfyContracts(t *testing.T) {
	hs := containers.NewHashSet(1)
	ss := containers.NewSortedSet(1)
	hd := containers.NewHashDict[string, int]()
	sd := containers.NewSortedDict[string, int]()
	m := containers.Map[string, int]{}

	var (
		_ containers.Set[int]          = hs.View()
		_ containers.Set[int]          = ss.View()
		_ containers.Dict[string, int] = hd.View()
		_ containers.Dict[string, int] = sd.View()
		_ containers.Dict[string, int] = m.View()
	)

	// ADR 0011 consequence: a PROJECTING set view satisfies Elems[R] but not
	// Set[R], because Has takes the element type while All yields the projection.
	sp := containers.ViewHashSet(containers.NewHashSet(&item{}), projectItem)
	var _ containers.Elems[itemView] = sp
}
