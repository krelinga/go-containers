package refcontainers

import "testing"

// The design's central claim: containers AND views satisfy the capability
// interface, so generic read-only code accepts either.
func TestBothSatisfyTheCapabilityInterface(t *testing.T) {
	c := newPtrSet("a", "b")
	v := viewIdentity(c)

	var _ Set[string] = c // the container
	var _ Set[string] = v // the view

	if countThrough[string](c) != 2 || countThrough[string](v) != 2 {
		t.Error("generic boundary should accept both")
	}
	t.Log("container and view both satisfy Set[T]; generic code takes either")
}

// The hierarchy survives in the interface layer -- substitution stays implicit,
// which struct-embedded views gave up.
func TestInterfaceHierarchySurvives(t *testing.T) {
	var os OrderedSet[string] = orderedIdentity{viewIdentity(newPtrSet("a"))}
	var s Set[string] = os // implicit, no conversion written
	if s.Len() != 1 {
		t.Error("ordered view did not substitute for the base")
	}
	list := []Set[string]{os, viewIdentity(newPtrSet("b"))}
	if len(list) != 2 {
		t.Error("ordered view could not enter a []Set")
	}
	t.Log("OrderedSet substitutes for Set implicitly, and composes into []Set")
}

type orderedIdentity struct{ identityView[string] }

func (orderedIdentity) Min() (string, bool) { return "", false }

// The wart the design cannot remove: the capability interface is unsealed, so
// a value put into it can be asserted back. If that value was a CONTAINER, the
// assertion recovers write access.
func TestCapabilityInterfaceIsAssertable(t *testing.T) {
	c := newPtrSet("a")

	var s Set[string] = c
	if back, ok := s.(*ptrSet[string]); ok {
		back.Add("smuggled") // write access recovered through the read interface
		t.Log("a CONTAINER in Set[T] asserts back to a writable container")
	} else {
		t.Fatal("expected the assertion to succeed")
	}
	if !c.Has("smuggled") {
		t.Error("the write did not land")
	}

	// A VIEW in the same interface asserts back only to a view, which is still
	// read-only -- so the guarantee depends on what the caller passed.
	var sv Set[string] = viewIdentity(c)
	if _, ok := sv.(*ptrSet[string]); ok {
		t.Error("a view should not assert back to its container")
	}
	t.Log("a VIEW in Set[T] cannot be asserted back to anything writable")
}
