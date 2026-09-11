package containers_test

import (
	"slices"
	"testing"

	"github.com/krelinga/go-containers"
)

func sorted[T interface{ ~string | ~int }](s containers.MapSet[T]) []T {
	return slices.Sorted(s.Keys())
}

func mustPanic(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: expected panic, got none", name)
		}
	}()
	f()
}

// A zero MapSet reads as empty and panics only on a write. The rule is the
// builtin's, and TestZeroValueMatchesTheBuiltin asserts it for every container
// at once (ADR 0018), so there is nothing MapSet-specific left to check here.

// A MapSet's iterator is backed by the live map, so it is NOT a snapshot --
// which is the same thing ranging a builtin map gives you, and why modifying
// during iteration is unsupported.
func TestKeysBindsTheLiveMap(t *testing.T) {
	s := containers.NewMapSet("a")
	it := s.Keys()
	s.Add("b")
	if got := slices.Sorted(it); len(got) != 2 {
		t.Errorf("iterator = %v, want both elements (it binds the live map)", got)
	}

	// A zero MapSet iterates zero times rather than panicking, and stays
	// unwritable -- the lazy-creation that used to make this work is gone
	// (ADR 0018).
	var zero containers.MapSet[string]
	n := 0
	for range zero.Keys() {
		n++
	}
	if n != 0 {
		t.Errorf("zero MapSet iterated %d times", n)
	}
	mustPanic(t, "zero MapSet.Add", func() { zero.Add("a") })
}

func TestAddRemoveIdempotent(t *testing.T) {
	s := containers.NewMapSet("a")
	s.Add("a")
	if s.Len() != 1 {
		t.Errorf("re-Add changed Len to %d, want 1", s.Len())
	}
	s.Delete("absent")
	if s.Len() != 1 {
		t.Errorf("Remove of absent value changed Len to %d, want 1", s.Len())
	}
	s.Delete("a")
	if s.Len() != 0 {
		t.Errorf("Len after Remove = %d, want 0", s.Len())
	}
	// Remove on a zero value is a no-op, not a panic.
	z := containers.NewMapSet[string]()
	z.Delete("x")
}

// ADR 0002 consequences: Clone is independent. A shallow struct copy would
// share the map; go vet rejects that at the definition, and this asserts the
// behaviour callers depend on.
func TestCloneIsIndependent(t *testing.T) {
	orig := containers.NewMapSet("a", "b")
	clone := orig.Clone()
	clone.Add("c")
	orig.Delete("a")

	if got, want := sorted(orig), []string{"b"}; !slices.Equal(got, want) {
		t.Errorf("orig = %v, want %v", got, want)
	}
	if got, want := sorted(clone), []string{"a", "b", "c"}; !slices.Equal(got, want) {
		t.Errorf("clone = %v, want %v", got, want)
	}

	zero := containers.NewMapSet[int]()
	if zero.Clone().Len() != 0 {
		t.Error("Clone of zero value should be empty")
	}
}

func TestAlgebra(t *testing.T) {
	a := containers.NewMapSet("read", "write", "list")
	b := containers.NewMapSet("write", "list", "admin")

	if got, want := sorted(a.Union(b)), []string{"admin", "list", "read", "write"}; !slices.Equal(got, want) {
		t.Errorf("Union = %v, want %v", got, want)
	}
	if got, want := sorted(a.Intersect(b)), []string{"list", "write"}; !slices.Equal(got, want) {
		t.Errorf("Intersect = %v, want %v", got, want)
	}
	if got, want := sorted(a.Difference(b)), []string{"read"}; !slices.Equal(got, want) {
		t.Errorf("Difference = %v, want %v", got, want)
	}

	// Intersect iterates the smaller operand; the result must not depend on order.
	big := containers.NewMapSet("a", "b", "c", "d", "e")
	small := containers.NewMapSet("c", "z")
	if got, want := sorted(big.Intersect(small)), []string{"c"}; !slices.Equal(got, want) {
		t.Errorf("big.Intersect(small) = %v, want %v", got, want)
	}
	if got, want := sorted(small.Intersect(big)), []string{"c"}; !slices.Equal(got, want) {
		t.Errorf("small.Intersect(big) = %v, want %v", got, want)
	}

	// Operands are unchanged.
	if a.Len() != 3 || b.Len() != 3 {
		t.Errorf("algebra mutated operands: len(a)=%d len(b)=%d", a.Len(), b.Len())
	}

	// The result is independent of both operands, not a view onto either.
	u := a.Union(b)
	u.Add("extra")
	u.Delete("read")
	if !a.Has("read") || a.Len() != 3 {
		t.Errorf("mutating the result changed a: %v", sorted(a))
	}
	if b.Has("extra") || b.Len() != 3 {
		t.Errorf("mutating the result changed b: %v", sorted(b))
	}
}

func TestAlgebraOnZeroValues(t *testing.T) {
	empty := containers.NewMapSet[string]()
	full := containers.NewMapSet("a")

	if got := sorted(full.Union(empty)); !slices.Equal(got, []string{"a"}) {
		t.Errorf("Union with empty = %v", got)
	}
	if full.Intersect(empty).Len() != 0 {
		t.Error("Intersect with empty should be empty")
	}
	if got := sorted(full.Difference(empty)); !slices.Equal(got, []string{"a"}) {
		t.Errorf("Difference with empty = %v", got)
	}
}

// The interface is satisfied by the pointer, and from a value only via &.
func TestMutableSetSatisfaction(t *testing.T) {
	s := containers.NewMapSet[int]()
	var _ containers.MutableKeys[int] = s
	var _ containers.MutableKeys[int] = containers.NewMapSet[int]()
}
