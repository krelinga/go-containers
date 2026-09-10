package refcontainers

import "testing"

// What a nil container interface actually does. The appeal of the all-interface
// shape is that `== nil` works on containers and views alike; the price is what
// happens next.
func TestNilInterfaceContainer(t *testing.T) {
	var s ifaceSet[string]

	if s != nil {
		t.Fatal("a zero container interface should be nil")
	}
	t.Log("== nil works: one spelling for containers and views")

	// But every method panics, including reads -- a nil interface has no
	// dynamic type, so there is nothing to dispatch to. A nil builtin map reads
	// fine; this does not.
	for name, f := range map[string]func(){
		"Len":      func() { _ = s.Len() },
		"Has":      func() { _ = s.Has("x") },
		"Keys":     func() { _ = s.Keys() },
		"KeySlice": func() { _ = s.KeySlice() },
		"Add":      func() { s.Add("x") },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s on a nil container interface did not panic", name)
				}
			}()
			f()
		}()
	}
	t.Log("every method panics on the zero value, reads included")
}

// The seal still separates containers from views: each has its own unexported
// method, so neither satisfies the other.
func TestSealSurvives(t *testing.T) {
	var _ ifaceSet[string] = newIfaceSet[string]()
	var _ ifaceSetView[string] = viewIface(newPtrSet[string]())
	t.Log("containers and views are disjoint sealed interfaces")
}

// Option (b)'s zero values, on both sides. A reference container's zero value
// reads as empty (ADR 0014 decision 3); the question is whether a struct view's
// does the same without help.
func TestOptionBZeroValues(t *testing.T) {
	var c nilRefSet[string]
	if c.Len() != 0 || c.Has("x") || c.KeySlice() != nil {
		t.Error("zero reference container should read as empty")
	}
	n := 0
	for range c.Keys() {
		n++
	}
	if n != 0 {
		t.Error("zero reference container should iterate zero times")
	}
	t.Log("zero container: total reads, as designed")

	// A struct view with NO nil handling: the naive shape.
	var bare wrapSetView[string]
	func() {
		defer func() {
			if recover() == nil {
				t.Error("bare struct view did NOT panic on a zero value")
			}
		}()
		_ = bare.Len()
	}()
	t.Log("zero struct view WITHOUT checks: panics, unlike the container")

	// With the checks, it matches.
	var checked nilWrapSetView[string]
	if !checked.IsZero() || checked.Len() != 0 || checked.Has("x") || checked.KeySlice() != nil {
		t.Error("checked zero struct view should read as empty")
	}
	m := 0
	for range checked.Keys() {
		m++
	}
	if m != 0 {
		t.Error("checked zero struct view should iterate zero times")
	}
	t.Log("zero struct view WITH checks: matches the container")
}
