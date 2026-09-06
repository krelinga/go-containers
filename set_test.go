package containers_test

import (
	"slices"
	"testing"

	"github.com/krelinga/go-containers"
)

func sorted[T interface{ ~string | ~int }](s *containers.Set[T]) []T {
	return slices.Sorted(s.All())
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

// ADR 0002, decision 2: the zero value is usable.
func TestZeroValueIsUsable(t *testing.T) {
	var s containers.Set[string]

	if s.Len() != 0 || s.Has("read") {
		t.Errorf("fresh zero value not empty: Len=%d Has=%v", s.Len(), s.Has("read"))
	}
	if got := slices.Sorted(s.All()); len(got) != 0 {
		t.Errorf("All on zero value = %v, want empty", got)
	}

	s.Add("read", "write")
	if s.Len() != 2 || !s.Has("read") {
		t.Errorf("after Add: Len=%d Has(read)=%v", s.Len(), s.Has("read"))
	}

	// A composite literal behaves identically.
	c := containers.Set[string]{}
	c.Add("x")
	if c.Len() != 1 {
		t.Errorf("composite literal Len = %d, want 1", c.Len())
	}
}

// ADR 0002, decision 2: every method panics on a nil pointer, with no
// special-casing. All must panic at the call, not deferred to iteration.
func TestNilPointerPanicsUniformly(t *testing.T) {
	var p *containers.Set[string]

	mustPanic(t, "Add", func() { p.Add("x") })
	mustPanic(t, "Remove", func() { p.Remove("x") })
	mustPanic(t, "Has", func() { _ = p.Has("x") })
	mustPanic(t, "Len", func() { _ = p.Len() })
	mustPanic(t, "Clone", func() { _ = p.Clone() })
	// Eagerly, without iterating the result:
	mustPanic(t, "All", func() { _ = p.All() })
}

func TestAddRemoveIdempotent(t *testing.T) {
	s := containers.NewSet("a")
	s.Add("a")
	if s.Len() != 1 {
		t.Errorf("re-Add changed Len to %d, want 1", s.Len())
	}
	s.Remove("absent")
	if s.Len() != 1 {
		t.Errorf("Remove of absent value changed Len to %d, want 1", s.Len())
	}
	s.Remove("a")
	if s.Len() != 0 {
		t.Errorf("Len after Remove = %d, want 0", s.Len())
	}
	// Remove on a zero value is a no-op, not a panic.
	var z containers.Set[string]
	z.Remove("x")
}

// ADR 0002 consequences: Clone is independent. A shallow struct copy would
// share the map; go vet rejects that at the definition, and this asserts the
// behaviour callers depend on.
func TestCloneIsIndependent(t *testing.T) {
	orig := containers.NewSet("a", "b")
	clone := orig.Clone()
	clone.Add("c")
	orig.Remove("a")

	if got, want := sorted(orig), []string{"b"}; !slices.Equal(got, want) {
		t.Errorf("orig = %v, want %v", got, want)
	}
	if got, want := sorted(clone), []string{"a", "b", "c"}; !slices.Equal(got, want) {
		t.Errorf("clone = %v, want %v", got, want)
	}

	var zero containers.Set[int]
	if zero.Clone().Len() != 0 {
		t.Error("Clone of zero value should be empty")
	}
}

func TestAlgebra(t *testing.T) {
	a := containers.NewSet("read", "write", "list")
	b := containers.NewSet("write", "list", "admin")

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
	big := containers.NewSet("a", "b", "c", "d", "e")
	small := containers.NewSet("c", "z")
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
}

func TestAlgebraOnZeroValues(t *testing.T) {
	var empty containers.Set[string]
	full := containers.NewSet("a")

	if got := sorted(full.Union(&empty)); !slices.Equal(got, []string{"a"}) {
		t.Errorf("Union with empty = %v", got)
	}
	if full.Intersect(&empty).Len() != 0 {
		t.Error("Intersect with empty should be empty")
	}
	if got := sorted(full.Difference(&empty)); !slices.Equal(got, []string{"a"}) {
		t.Errorf("Difference with empty = %v", got)
	}
}

// The interface is satisfied by the pointer, and from a value only via &.
func TestSetLikeSatisfaction(t *testing.T) {
	var s containers.Set[int]
	var _ containers.SetLike[int] = &s
	var _ containers.SetLike[int] = containers.NewSet[int]()
}
