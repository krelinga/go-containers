package containers_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/krelinga/go-containers"
)

func hdKeys(d *containers.HashDict[int, string]) []int {
	return slices.Sorted(maps.Keys(maps.Collect(d.All())))
}

// ADR 0002's shape rules, which are the whole point of the type: these are the
// four things ADR 0007 recorded that Map cannot do.
func TestHashDictZeroValueIsUsable(t *testing.T) {
	var d containers.HashDict[int, string]

	if d.Len() != 0 {
		t.Errorf("Len = %d, want 0", d.Len())
	}
	if _, ok := d.Get(1); ok {
		t.Error("Get on zero value returned ok")
	}
	if got := hdKeys(&d); got != nil {
		t.Errorf("All on zero value = %v, want empty", got)
	}
	d.Delete(1) // no-op, not a panic

	// The difference from Map: this works instead of panicking.
	d.Set(1, "a")
	if v, ok := d.Get(1); !ok || v != "a" {
		t.Errorf("Set on zero value: %q,%v", v, ok)
	}
}

func TestHashDictNilPanicsUniformly(t *testing.T) {
	var p *containers.HashDict[int, string]
	src := containers.NewHashDict[int, string]()
	empty := func(func(int, string) bool) {}

	mustPanic(t, "Get", func() { _, _ = p.Get(1) })
	mustPanic(t, "Set", func() { p.Set(1, "a") })
	mustPanic(t, "Delete", func() { p.Delete(1) })
	mustPanic(t, "Len", func() { _ = p.Len() })
	mustPanic(t, "All", func() { _ = p.All() })
	mustPanic(t, "Clone", func() { _ = p.Clone() })
	mustPanic(t, "SetAll", func() { p.SetAll(src) })
	mustPanic(t, "SetAllSeq", func() { p.SetAllSeq(empty) })
	// The eager-dereference rule: an empty input must still panic.
	mustPanic(t, "SetAllSeq empty", func() { p.SetAllSeq(empty) })
	mustPanic(t, "SetAll nil src", func() {
		var d containers.HashDict[int, string]
		d.SetAll(nil)
	})
}

func TestHashDictOperations(t *testing.T) {
	d := containers.NewHashDict[int, string]()
	d.Set(2, "b")
	d.Set(1, "a")
	d.Set(2, "replaced")

	if v, ok := d.Get(2); !ok || v != "replaced" {
		t.Errorf("Set did not replace: %q,%v", v, ok)
	}
	if d.Len() != 2 {
		t.Errorf("Len = %d, want 2", d.Len())
	}
	d.Delete(1)
	d.Delete(99) // absent, no-op
	if got, want := hdKeys(d), []int{2}; !slices.Equal(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

func TestHashDictCloneIsIndependent(t *testing.T) {
	orig := containers.NewHashDict[int, string]()
	orig.Set(1, "a")
	orig.Set(2, "b")

	clone := orig.Clone()
	clone.Set(3, "c")
	clone.Set(1, "changed")
	orig.Delete(2)

	if got, want := hdKeys(orig), []int{1}; !slices.Equal(got, want) {
		t.Errorf("orig = %v, want %v", got, want)
	}
	if v, _ := orig.Get(1); v != "a" {
		t.Errorf("clone mutation leaked into orig: %q", v)
	}
	if got, want := hdKeys(clone), []int{1, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("clone = %v, want %v", got, want)
	}

	// Clone of the zero value is usable, not merely empty.
	var zero containers.HashDict[int, string]
	z := zero.Clone()
	if z.Len() != 0 {
		t.Error("Clone of zero value should be empty")
	}
	z.Set(1, "a")
	if z.Len() != 1 {
		t.Error("Clone of zero value should still be writable")
	}
}

func TestHashDictBulk(t *testing.T) {
	src := containers.NewHashDict[int, string]()
	src.SetAllSeq(maps.All(map[int]string{1: "a", 2: "b", 3: "c"}))

	// Sized and bare forms agree.
	sized := containers.CollectHashDict(src)
	bare := containers.CollectHashDictSeq(src.All())
	if !slices.Equal(hdKeys(sized), hdKeys(bare)) {
		t.Errorf("CollectHashDict %v vs Seq %v", hdKeys(sized), hdKeys(bare))
	}
	if v, _ := sized.Get(2); v != "b" {
		t.Errorf("CollectHashDict lost a value: %q", v)
	}

	// SetAll merges into an existing dict, last write winning.
	d := containers.NewHashDict[int, string]()
	d.Set(1, "old")
	d.Set(9, "kept")
	d.SetAll(src)
	if got, want := hdKeys(d), []int{1, 2, 3, 9}; !slices.Equal(got, want) {
		t.Errorf("after SetAll = %v, want %v", got, want)
	}
	if v, _ := d.Get(1); v != "a" {
		t.Errorf("SetAll did not replace key 1: %q", v)
	}
	if v, _ := d.Get(9); v != "kept" {
		t.Errorf("SetAll disturbed key 9: %q", v)
	}

	// SetAll on a zero value presizes rather than failing.
	var z containers.HashDict[int, string]
	z.SetAll(src)
	if z.Len() != 3 {
		t.Errorf("SetAll on zero value Len = %d, want 3", z.Len())
	}

	// Self-reference is well defined.
	d.SetAll(d)
	if got, want := hdKeys(d), []int{1, 2, 3, 9}; !slices.Equal(got, want) {
		t.Errorf("d.SetAll(d) = %v, want %v", got, want)
	}
}

// Len is a hint, never a correctness input -- ADR 0006 decision 3.
func TestHashDictLenIsOnlyAHint(t *testing.T) {
	vals := map[int]string{1: "a", 5: "e", 9: "i"}
	want := []int{1, 5, 9}
	for _, n := range []int{0, 1, 2, 1000, -7} {
		src := lyingElems2{vals: vals, n: n}
		if got := hdKeys(containers.CollectHashDict(src)); !slices.Equal(got, want) {
			t.Errorf("CollectHashDict with Len()=%d = %v, want %v", n, got, want)
		}
		var d containers.HashDict[int, string]
		d.SetAll(src)
		if got := hdKeys(&d); !slices.Equal(got, want) {
			t.Errorf("SetAll with Len()=%d = %v, want %v", n, got, want)
		}
	}
}

// HashDict resolves ADR 0007's asymmetry: it satisfies through a pointer, like
// every other container, so generic call sites read the same way.
func TestHashDictSatisfiesContractsThroughPointer(t *testing.T) {
	var d containers.HashDict[int, string]
	var (
		_ containers.Elems2[int, string]      = &d
		_ containers.Dict[int, string]        = &d
		_ containers.MutableDict[int, string] = &d
	)
	d.Set(1, "a")
	if n := lookupAll[int, string](&d, 1, 2); n != 1 {
		t.Errorf("lookupAll = %d, want 1", n)
	}
}
