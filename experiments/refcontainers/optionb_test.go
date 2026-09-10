package refcontainers

import (
	"maps"
	"slices"
	"testing"
)

// The zero value of a reference container must behave exactly as a nil builtin
// map does. This asserts that side by side rather than by inspection.
func TestZeroValueMatchesNilMap(t *testing.T) {
	var m map[string]struct{} // nil builtin map
	var s obSet[string]       // zero reference container

	if len(m) != s.Len() {
		t.Errorf("Len: map %d, container %d", len(m), s.Len())
	}
	_, mok := m["x"]
	if mok != s.Has("x") {
		t.Errorf("lookup: map %v, container %v", mok, s.Has("x"))
	}
	if got, want := len(slices.Collect(maps.Keys(m))), len(slices.Collect(s.Keys())); got != want {
		t.Errorf("iteration: map %d, container %d", got, want)
	}
	if s.KeySlice() != nil {
		t.Error("KeySlice on a zero container should be nil, as slices.Collect of a nil map is empty")
	}
	if !s.IsZero() || m != nil {
		t.Error("both should report empty")
	}

	// And a write panics on both.
	mustPanicHere(t, "write to nil map", func() { m["x"] = struct{}{} })
	mustPanicHere(t, "write to zero container", func() { s.Add("x") })
	t.Log("zero container is indistinguishable from a nil map on every operation")
}

// The rejected alternative: mixed receivers let a write lazily construct the
// state, which preserves ADR 0002's usable zero value -- and reintroduces the
// divergence that reference semantics exists to remove.
func TestLazyInitDiverges(t *testing.T) {
	var a lazySet[string]
	a.Add("one") // works: auto-addressed, lazily constructed
	if a.Len() != 1 {
		t.Fatalf("lazy Add did not take: Len=%d", a.Len())
	}
	t.Log("mixed receivers: the zero value IS writable, as ADR 0002 requires")

	// But a copy taken before the first write does not share.
	var b lazySet[string]
	c := b // copy of a zero value
	b.Add("one")
	if b.Len() == c.Len() {
		t.Fatal("expected divergence")
	}
	t.Logf("copy taken before first write DIVERGES: original=%d copy=%d", b.Len(), c.Len())
	t.Log("this is the exact bug class noCopy exists to catch, reintroduced")
}

// Struct embedding recovers the hierarchy: promoted methods, and a field
// selector rather than a conversion method.
func TestEmbeddingRecoversSubstitution(t *testing.T) {
	sv := viewOBSorted(newPtrSet("a", "b"))

	// Promoted: the base methods are callable directly on the ordered view.
	if sv.Len() != 2 || !sv.Has("a") {
		t.Error("promoted methods do not work")
	}

	// The conversion to the base is a field selector.
	var base obSetView[string] = sv.SetView
	if base.Len() != 2 {
		t.Error("field-selector conversion does not work")
	}

	// And it composes: an ordered view can enter a slice of base views.
	list := []obSetView[string]{sv.SetView, base}
	if len(list) != 2 || list[0].Len() != 2 {
		t.Error("ordered view could not enter a []SetView")
	}
	t.Log("embedding: methods promoted, conversion is `sv.SetView`, composes into []SetView")
}

func mustPanicHere(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: expected panic, got none", name)
		}
	}()
	f()
}
