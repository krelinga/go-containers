package sealing

import (
	"reflect"
	"testing"
	"unsafe"
)

// Every route out of a sealed read-only handle, enumerated. Two of these look
// like they should work and do not; one looks like it should not and does.
//
// The mechanism: the SEAL is an unexported method on the interface, and the
// container does NOT implement it. Only an unexported wrapper does. So the
// compiler rejects the assertion as IMPOSSIBLE, and laundering through any()
// finds a type with no mutators behind the interface.

func TestSealedRoutesOut(t *testing.T) {
	s, vw := fixture()
	v := NewIfaceView(s, vw)

	// 1. The direct assertion does not compile. See badcast/ for the error;
	//    it cannot be written here without breaking the build.
	t.Log("1. v.(MapSet[*Item]) -- rejected at COMPILE time (see badcast/)")

	// 2. Launder through any() to get past the compiler.
	if _, ok := any(v).(MapSet[*Item]); ok {
		t.Error("2. HOLE: any() recovered the container")
	} else {
		t.Log("2. any(v).(MapSet[*Item]) -- fails at runtime")
	}

	// 3. Assert to an ad-hoc MUTATION interface declared locally.
	if _, ok := any(v).(interface{ Add(*Item) }); ok {
		t.Error("3. HOLE: ad-hoc mutation interface matched")
	} else {
		t.Log("3. any(v).(interface{ Add(*Item) }) -- fails at runtime")
	}

	// 4. Ad-hoc READ interface: succeeds, and is harmless.
	if w, ok := any(v).(interface{ Len() int }); ok {
		t.Logf("4. ad-hoc read interface matches (Len=%d) -- harmless", w.Len())
	}

	// 5. reflect can see the field but cannot hand it back.
	rv := reflect.ValueOf(v)
	f := rv.Field(0)
	t.Logf("5. reflect: dynamic type %v, field CanSet=%v CanInterface=%v",
		rv.Type(), f.CanSet(), f.CanInterface())
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("5. HOLE: reflect returned the field")
			} else {
				t.Logf("5. reflect Interface() panics: %v", r)
			}
		}()
		_ = f.Interface()
	}()

	_ = unsafe.Sizeof(v) // unsafe defeats every Go guarantee, not just this one
}

// A sealed interface can still be SATISFIED from outside by embedding it, which
// promotes the unexported method. It grants nothing writable.
type embedder struct {
	IfaceView[string]
	label string
}

func TestEmbeddingSatisfiesButGrantsNothing(t *testing.T) {
	s, vw := fixture()
	var w IfaceView[string] = embedder{NewIfaceView(s, vw), "mine"}
	t.Logf("embedding satisfies the seal: Len=%d", w.Len())
	if _, ok := any(w).(interface{ Add(*Item) }); ok {
		t.Error("HOLE: embedder exposed Add")
	}
	if _, ok := any(w).(MapSet[*Item]); ok {
		t.Error("HOLE: embedder asserted to the container")
	}
	t.Log("...and exposes no mutator, so mocking works and the guarantee holds")
}

// THE TRAP: sealing an interface the CONTAINER also satisfies changes nothing.
// This is the shape a naive "just seal the shape interfaces" would produce.
type looseSealed interface {
	Len() int
	looseToken()
}

func (s MapSet[T]) looseToken() {} // the container can supply it

func TestSealingAloneIsTheater(t *testing.T) {
	s, _ := fixture()
	var k looseSealed = s
	if w, ok := any(k).(MapSet[*Item]); ok {
		w.Add(&Item{Name: "injected"})
		t.Logf("HOLE SURVIVES the seal: wrote through a sealed interface, Len now %d", s.Len())
	} else {
		t.Error("expected the hole to survive")
	}
	t.Log("=> a seal only helps if the CONTAINER is excluded from it")
}

// Sealed interfaces do not COMPOSE unless they share one token: an
// interface-typed value satisfies another sealed interface only if it DECLARES
// the same unexported method, whatever its dynamic type has.
type tokenA interface {
	Len() int
	viewOnly()
}

func TestOneSharedTokenIsRequired(t *testing.T) {
	s, vw := fixture()
	var v IfaceView[string] = NewIfaceView(s, vw)
	var a tokenA = v // compiles ONLY because both declare viewOnly()
	t.Logf("IfaceView -> tokenA: Len=%d (same token, so the tiers compose)", a.Len())
	t.Log("with distinct tokens this assignment would not compile, even though")
	t.Log("the dynamic type has both methods -- see RESULTS.md finding 2")
}

// Shallowness: the seal protects the STRUCTURE, never the contents. Go cannot
// express "contains no pointers" as a constraint, so this is not closable.
func TestSealDoesNotFreezeContents(t *testing.T) {
	it := &Item{Name: "original"}
	s := NewMapSet(it)
	v := NewIfaceView(s, stateful{})

	for name := range v.Keys() {
		_ = name
	}
	for p := range s.m { // the same pointers the view hands out
		p.Name = "MUTATED"
	}
	t.Logf("through a sealed, un-castable view the item reads %q", it.Name)
	t.Log("=> the STRUCTURE is sealed; the reachable object graph is not")
}
