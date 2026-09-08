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

func TestViewBlocksDowncast(t *testing.T) {
	d := containers.NewHashDict[string, int]()
	d.Set("a", 1)

	// A bare contract protects nothing: the container satisfies it structurally.
	var bare containers.Dict[string, int] = d
	if c, ok := bare.(*containers.HashDict[string, int]); ok {
		c.Set("b", 2)
	}
	if d.Len() != 2 {
		t.Error("expected the bare contract to permit mutation, for contrast")
	}

	// A view does not.
	var sealed any = containers.ViewHashDictIdentity(d)
	if _, ok := sealed.(*containers.HashDict[string, int]); ok {
		t.Error("view was assertable back to its container")
	}
}

// ADR 0012's motivating hole: comparable admits pointers, so a value-only view
// leaked mutable keys. A key viewer closes it.
func TestViewConvertsKeys(t *testing.T) {
	k := &item{Name: "alpha"}
	d := containers.NewHashDict[*item, *item]()
	d.Set(k, &item{Name: "payload"})

	vw := itemViewer{itemKeys: itemKeys{known: map[string]*item{"alpha": k}}}
	v := containers.ViewHashDict(d, vw)

	// Keys come out as strings; the raw *item never escapes.
	for gotKey, gotVal := range v.All() {
		if gotKey != "alpha" {
			t.Errorf("key = %v, want %q", gotKey, "alpha")
		}
		if gotVal.Name() != "payload" {
			t.Errorf("value = %q", gotVal.Name())
		}
	}

	// And the view satisfies a contract naming neither raw type.
	var _ containers.Dict[string, itemView] = v
}

// A key that does not convert back cannot be present.
func TestFromKeyViewFailureIsAMiss(t *testing.T) {
	k := &item{Name: "alpha"}
	d := containers.NewHashDict[*item, *item]()
	d.Set(k, &item{Name: "payload"})

	vw := itemViewer{itemKeys: itemKeys{known: map[string]*item{"alpha": k}}}
	v := containers.ViewHashDict(d, vw)

	if _, ok := v.Get("alpha"); !ok {
		t.Error("a convertible, present key should hit")
	}
	got, ok := v.Get("not-a-known-key")
	if ok || got != (itemView{}) {
		t.Errorf("unconvertible key = %v,%v; want zero,false", got, ok)
	}
}

// A miss must not call the value projection.
func TestMissDoesNotProject(t *testing.T) {
	d := containers.NewSortedDict[int, *item]()
	called := false
	v := containers.ViewSortedDict(d, valueCounter{&called})
	if _, ok := v.Get(99); ok {
		t.Error("empty dict should miss")
	}
	if called {
		t.Error("projection ran for a missing key")
	}
}

type valueCounter struct{ called *bool }

func (c valueCounter) ToValueView(i *item) itemView { *c.called = true; return itemView{i} }

// ---- ordered containers convert values only (ADR 0012 decision 3) ----------

func TestSortedViewsConvertValuesOnly(t *testing.T) {
	sd := containers.NewSortedDict[int, *item]()
	sd.Set(1, &item{Name: "one"})
	sd.Set(3, &item{Name: "three"})
	v := containers.ViewSortedDict(sd, itemValues{})

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
	ss := containers.NewSortedSet(3, 1, 2)
	sv := containers.ViewSortedSet(ss)
	if mn, ok := sv.Min(); mn != 1 || !ok {
		t.Errorf("SortedSetView.Min = %v,%v", mn, ok)
	}
	if got := slices.Collect(sv.Range(2, 4)); !slices.Equal(got, []int{2, 3}) {
		t.Errorf("SortedSetView.Range = %v", got)
	}
}

// ---- shape rules -----------------------------------------------------------

func TestZeroViewPanics(t *testing.T) {
	var hsv containers.HashSetView[int, int]
	var hdv containers.HashDictView[int, string, int, string]
	var sdv containers.SortedDictView[int, string, string]
	var ssv containers.SortedSetView[int]

	mustPanic(t, "HashSetView.Len", func() { _ = hsv.Len() })
	mustPanic(t, "HashSetView.Has", func() { _ = hsv.Has(1) })
	mustPanic(t, "HashSetView.All", func() { _ = hsv.All() })
	mustPanic(t, "HashDictView.Get", func() { _, _ = hdv.Get(1) })
	mustPanic(t, "HashDictView.All", func() { _ = hdv.All() })
	mustPanic(t, "SortedDictView.Min", func() { _, _, _ = sdv.Min() })
	mustPanic(t, "SortedDictView.All", func() { _ = sdv.All() })
	mustPanic(t, "SortedSetView.Len", func() { _ = ssv.Len() })
}

// TestEveryContainerHasAView is the guardrail for ADR 0011's rule, which ADR
// 0012 preserved while changing its shape: a container is not finished until it
// has view constructors.
func TestEveryContainerHasAView(t *testing.T) {
	hs := containers.NewHashSet(1)
	ss := containers.NewSortedSet(1)
	hd := containers.NewHashDict[string, int]()
	sd := containers.NewSortedDict[string, int]()
	m := containers.Map[string, int]{}

	var (
		_ containers.Set[int]          = containers.ViewHashSetIdentity(hs)
		_ containers.Set[int]          = containers.ViewSortedSet(ss)
		_ containers.Dict[string, int] = containers.ViewHashDictIdentity(hd)
		_ containers.Dict[string, int] = containers.ViewSortedDictIdentity(sd)
		_ containers.Dict[string, int] = containers.ViewMapIdentity(m)
	)

	// A projecting set view satisfies Set[NT], which ADR 0011 recorded as lost
	// and ADR 0012 restored: Has takes NT and All yields NT.
	reg := &item{Name: "x"}
	hsp := containers.NewHashSet(reg)
	pv := containers.ViewHashSet(hsp, itemKeys{known: map[string]*item{"x": reg}})
	var _ containers.Set[string] = pv
	if !pv.Has("x") || pv.Has("nope") {
		t.Error("projecting set view membership is wrong")
	}
}
