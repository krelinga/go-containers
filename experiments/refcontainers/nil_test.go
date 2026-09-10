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
