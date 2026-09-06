package containers_test

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"github.com/krelinga/go-containers"
)

func mapKeys(m containers.Map[int, string]) []int { return slices.Sorted(maps.Keys(m)) }

// The conversion is not a copy: the wrapper aliases what it wraps.
func TestMapConversionAliases(t *testing.T) {
	plain := map[int]string{1: "a"}
	wrapped := containers.Map[int, string](plain)

	wrapped.Set(2, "b")
	if plain[2] != "b" || len(plain) != 2 {
		t.Errorf("write through wrapper did not reach the source: %v", plain)
	}
	plain[3] = "c"
	if v, ok := wrapped.Get(3); !ok || v != "c" {
		t.Errorf("write through the source did not reach the wrapper: %v,%v", v, ok)
	}

	// Clone breaks the aliasing.
	c := wrapped.Clone()
	c.Set(4, "d")
	if _, ok := wrapped.Get(4); ok {
		t.Error("Clone still aliases the original")
	}
}

// A defined map type keeps every builtin operation.
func TestMapBuiltinSyntax(t *testing.T) {
	m := containers.Map[int, string]{}
	m[1] = "a"
	m[2] = "b"
	if len(m) != 2 || m[1] != "a" {
		t.Errorf("index/len broken: len=%d m[1]=%q", len(m), m[1])
	}
	delete(m, 1)
	n := 0
	for range m {
		n++
	}
	if n != 1 || len(m) != 1 {
		t.Errorf("delete/range broken: n=%d len=%d", n, len(m))
	}
}

// ADR 0007: Map takes the builtin's semantics, not ADR 0002's shape rules.
// Reads on the zero value work; writes panic exactly as on a nil map.
func TestMapZeroValue(t *testing.T) {
	var m containers.Map[int, string]

	if m.Len() != 0 {
		t.Errorf("Len = %d, want 0", m.Len())
	}
	if _, ok := m.Get(1); ok {
		t.Error("Get on zero value returned ok")
	}
	if got := slices.Collect(maps.Keys(maps.Collect(m.All()))); len(got) != 0 {
		t.Errorf("All on zero value = %v, want empty", got)
	}
	m.Delete(1) // no-op, not a panic -- delete on a nil map is defined

	mustPanic(t, "Set on zero value", func() { m.Set(1, "a") })
}

func TestMapOperations(t *testing.T) {
	m := containers.Map[int, string]{}
	m.Set(2, "b")
	m.Set(1, "a")
	m.Set(2, "replaced")

	if v, ok := m.Get(2); !ok || v != "replaced" {
		t.Errorf("Set did not replace: %q,%v", v, ok)
	}
	if m.Len() != 2 {
		t.Errorf("Len = %d, want 2", m.Len())
	}
	m.Delete(1)
	m.Delete(99) // absent, no-op
	if got, want := mapKeys(m), []int{2}; !slices.Equal(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

// The value satisfies, unlike every other container here.
func TestMapInterfaceSatisfaction(t *testing.T) {
	var (
		_ containers.Elems2[int, string]      = containers.Map[int, string]{}
		_ containers.MutableDict[int, string] = containers.Map[int, string]{}
		_ containers.MutableDict[int, string] = containers.NewSortedDict[int, string]()
	)
}

// prune, written once, run against both backings -- the point of the contract.
func prune[K comparable, V any](m containers.MutableDict[K, V], keep func(V) bool) {
	var drop []K
	for k, v := range m.All() {
		if !keep(v) {
			drop = append(drop, k)
		}
	}
	for _, k := range drop {
		m.Delete(k)
	}
}

func TestMutableDictGenericOverBothBackings(t *testing.T) {
	for _, tc := range pruneCases {
		t.Run("Map/"+tc.name, func(t *testing.T) {
			m := containers.Map[int, int](maps.Clone(tc.in))
			prune[int, int](m, keepEven)
			if got := slices.Sorted(maps.Keys(m)); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
		t.Run("SortedDict/"+tc.name, func(t *testing.T) {
			sm := containers.NewSortedDict[int, int]()
			sm.SetAllSeq(maps.All(tc.in))
			prune[int, int](sm, keepEven)
			if got := slices.Sorted(maps.Keys(maps.Collect(sm.All()))); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ADR 0007 consequence: Map serializes where its key type allows, unlike the
// struct-backed containers. Recorded as a test so the asymmetry is visible.
func TestMapSerialization(t *testing.T) {
	m := containers.Map[string, int]{"a": 1}
	b, err := json.Marshal(m)
	if err != nil || string(b) != `{"a":1}` {
		t.Errorf("Marshal = %s, %v; want {\"a\":1}, nil", b, err)
	}
	var back containers.Map[string, int]
	if err := json.Unmarshal([]byte(`{"x":9}`), &back); err != nil || back["x"] != 9 {
		t.Errorf("Unmarshal = %v, %v", back, err)
	}

	// Integer keys are stringified, and still round-trip.
	mi := containers.Map[int, string]{7: "x"}
	bi, err := json.Marshal(mi)
	if err != nil || string(bi) != `{"7":"x"}` {
		t.Errorf("int-keyed Marshal = %s, %v", bi, err)
	}

	// Struct keys do not: K comparable permits them, encoding/json does not.
	type key struct{ A, B int }
	if _, err := json.Marshal(containers.Map[key, int]{{1, 2}: 3}); err == nil {
		t.Error("struct-keyed Map marshalled; expected an error")
	}

	// Meanwhile the struct-backed containers silently discard their contents.
	sm := containers.NewSortedDict[string, int]()
	sm.Set("a", 1)
	sb, err := json.Marshal(sm)
	if err != nil || string(sb) != "{}" {
		t.Errorf("SortedDict Marshal = %s, %v; want {}, nil (the known gap)", sb, err)
	}
}
