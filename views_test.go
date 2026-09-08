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
//	var _ containers.DictView[string, int] = containers.NewHashDict[string, int]()
//	  -> *HashDict[string, int] does not implement DictView[string, int]
//	     (missing method sealedView)
//
//	v := containers.ViewHashDictIdentity(d)
//	_ = v.(*containers.HashDict[string, int])
//	  -> impossible type assertion
//
// What is left to check at runtime is that laundering through any() does not
// recover the container either.
func TestViewDoesNotLeakItsContainer(t *testing.T) {
	d := containers.NewHashDict[string, int]()
	d.Set("a", 1)
	v := containers.ViewHashDictIdentity(d)

	if _, ok := any(v).(*containers.HashDict[string, int]); ok {
		t.Error("view was assertable back to its container through any()")
	}
	if _, ok := any(v).(containers.MutableDict[string, int]); ok {
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
	d := containers.NewHashDict[*item, *item]()
	d.Set(k, &item{Name: "payload"})

	vw := itemViewer{itemKeys: itemKeys{known: map[string]*item{"alpha": k}}}
	var v containers.DictView[string, itemView] = containers.ViewHashDict(d, vw)

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

// A miss must not call the value conversion.
func TestMissDoesNotConvert(t *testing.T) {
	d := containers.NewSortedDict[int, *item]()
	called := false
	v := containers.ViewSortedDict(d, valueCounter{&called})
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
	sv := containers.ViewSortedSet(containers.NewSortedSet(3, 1, 2))
	if mn, ok := sv.Min(); mn != 1 || !ok {
		t.Errorf("SortedSetView.Min = %v,%v", mn, ok)
	}
	if got := slices.Collect(sv.Range(2, 4)); !slices.Equal(got, []int{2, 3}) {
		t.Errorf("SortedSetView.Range = %v", got)
	}
}

// ---- the hierarchy (ADR 0013 decision 2) -----------------------------------

// An ordered view substitutes for the unordered one, so a consumer can be
// written against DictView without naming an implementation.
func TestOrderedViewsSubstituteForBase(t *testing.T) {
	sd := containers.NewSortedDict[string, int]()
	sd.Set("a", 1)
	hd := containers.NewHashDict[string, int]()
	hd.Set("a", 1)
	m := containers.Map[string, int]{"a": 1}

	// All three are the same type at this boundary.
	for name, d := range map[string]containers.DictView[string, int]{
		"SortedDict": containers.ViewSortedDictIdentity(sd),
		"HashDict":   containers.ViewHashDictIdentity(hd),
		"Map":        containers.ViewMapIdentity(m),
	} {
		if got, ok := d.Get("a"); !ok || got != 1 {
			t.Errorf("%s: Get(a) = %v,%v", name, got, ok)
		}
	}

	ss := containers.NewSortedSet(1, 2)
	hs := containers.NewHashSet(1, 2)
	for name, s := range map[string]containers.SetView[int]{
		"SortedSet": containers.ViewSortedSet(ss),
		"HashSet":   containers.ViewHashSetIdentity(hs),
	} {
		if !s.Has(1) {
			t.Errorf("%s: Has(1) = false", name)
		}
	}
}

// ---- shape rules -----------------------------------------------------------

// A nil view panics when used, exactly as a zero container does, and nothing in
// the package special-cases it (ADR 0013 decision 8, following ADR 0002).
func TestNilViewPanics(t *testing.T) {
	var sv containers.SetView[int]
	var dv containers.DictView[int, string]
	var ssv containers.SortedSetView[int]
	var sdv containers.SortedDictView[int, string]

	if sv != nil || dv != nil || ssv != nil || sdv != nil {
		t.Error("a zero view should be a nil interface")
	}
	mustPanic(t, "SetView.Len", func() { _ = sv.Len() })
	mustPanic(t, "SetView.Has", func() { _ = sv.Has(1) })
	mustPanic(t, "DictView.Get", func() { _, _ = dv.Get(1) })
	mustPanic(t, "DictView.All", func() { _ = dv.All() })
	mustPanic(t, "SortedSetView.Min", func() { _, _ = ssv.Min() })
	mustPanic(t, "SortedDictView.Range", func() { _ = sdv.Range(1, 2) })
}

// ADR 0002's eager-dereference rule: a view over a nil container must panic at
// the call, not at iteration. Violated three times in this package's history,
// so it is asserted rather than assumed.
func TestViewAllDereferencesEagerly(t *testing.T) {
	vw := itemViewer{itemKeys: itemKeys{known: map[string]*item{}}}

	mustPanic(t, "hashSetView.All", func() {
		_ = containers.ViewHashSet[*item, string](nil, itemKeys{}).All()
	})
	mustPanic(t, "hashSetIdentityView.All", func() {
		_ = containers.ViewHashSetIdentity[int](nil).All()
	})
	mustPanic(t, "sortedSetView.All", func() {
		_ = containers.ViewSortedSet[int](nil).All()
	})
	mustPanic(t, "hashDictView.All", func() {
		_ = containers.ViewHashDict[*item, *item, string, itemView](nil, vw).All()
	})
	mustPanic(t, "hashDictIdentityView.All", func() {
		_ = containers.ViewHashDictIdentity[int, string](nil).All()
	})
	mustPanic(t, "sortedDictView.All", func() {
		_ = containers.ViewSortedDict[int, *item, itemView](nil, itemValues{}).All()
	})
	mustPanic(t, "sortedDictIdentityView.All", func() {
		_ = containers.ViewSortedDictIdentity[int, string](nil).All()
	})
}

// TestEveryContainerHasAView is the guardrail for ADR 0011's rule, which ADRs
// 0012 and 0013 preserved while changing its shape twice: a container is not
// finished until it has view constructors returning a sealed interface.
func TestEveryContainerHasAView(t *testing.T) {
	hs := containers.NewHashSet(1)
	ss := containers.NewSortedSet(1)
	hd := containers.NewHashDict[string, int]()
	sd := containers.NewSortedDict[string, int]()
	m := containers.Map[string, int]{}

	var (
		_ containers.SetView[int]                = containers.ViewHashSetIdentity(hs)
		_ containers.SortedSetView[int]          = containers.ViewSortedSet(ss)
		_ containers.DictView[string, int]       = containers.ViewHashDictIdentity(hd)
		_ containers.SortedDictView[string, int] = containers.ViewSortedDictIdentity(sd)
		_ containers.DictView[string, int]       = containers.ViewMapIdentity(m)
	)

	// And the converting forms, which is what makes a view more than a wrapper.
	reg := &item{Name: "x"}
	hsp := containers.NewHashSet(reg)
	pv := containers.ViewHashSet(hsp, itemKeys{known: map[string]*item{"x": reg}})
	if !pv.Has("x") || pv.Has("nope") {
		t.Error("converting set view membership is wrong")
	}
	if got := slices.Collect(pv.All()); !slices.Equal(got, []string{"x"}) {
		t.Errorf("converting set view All = %v", got)
	}
}
