// Call-site comparison for this library.
//
// The same tasks are written twice: once against the standard library, once
// against this library. Per the call-site convention in CLAUDE.md the stdlib
// baselines land first and alone, so that adding the container versions
// produces a reviewable diff rather than a side-by-side written after the
// container was already paid for.
//
// The package is `containers_test`, not `containers`, on purpose: an internal
// test file could reach unexported identifiers and would give a flattering,
// dishonest view of ergonomics. The external test package forces the real
// public import path.
//
// Tasks correspond to those in docs/adr/0002-set.md.
package containers_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/krelinga/go-containers"
)

// ---------------------------------------------------------------------------
// Task A — which requested permissions are not granted? (difference)
// ---------------------------------------------------------------------------

func unauthorizedStdlib(granted, requested []string) []string {
	g := make(map[string]struct{}, len(granted))
	for _, p := range granted {
		g[p] = struct{}{}
	}
	var missing []string
	for _, p := range requested {
		if _, ok := g[p]; !ok {
			missing = append(missing, p)
		}
	}
	slices.Sort(missing)
	return slices.Compact(missing) // requested may contain dupes
}

func unauthorizedContainer(granted, requested []string) []string {
	missing := containers.NewSet(requested...).Difference(containers.NewSet(granted...))
	return slices.Sorted(missing.All())
}

// ---------------------------------------------------------------------------
// Task B — permissions held by both users (intersection)
// ---------------------------------------------------------------------------

func sharedStdlib(a, b []string) []string {
	bs := make(map[string]struct{}, len(b))
	for _, p := range b {
		bs[p] = struct{}{}
	}
	out := make(map[string]struct{})
	for _, p := range a {
		if _, ok := bs[p]; ok {
			out[p] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(out))
}

func sharedContainer(a, b []string) []string {
	return slices.Sorted(containers.NewSet(a...).Intersect(containers.NewSet(b...)).All())
}

// ---------------------------------------------------------------------------
// Task C — dedup
//
// Recorded in ADR 0002 as a task a Set does NOT clearly win. This is the
// fairest stdlib version: no map at all.
// ---------------------------------------------------------------------------

func distinctStdlib(in []string) []string {
	s := slices.Clone(in)
	slices.Sort(s)
	return slices.Compact(s)
}

func distinctContainer(in []string) []string {
	return slices.Sorted(containers.NewSet(in...).All())
}

// ---------------------------------------------------------------------------
// Cases. Shared by the stdlib and (later) container implementations so both are
// held to identical behaviour.
// ---------------------------------------------------------------------------

var unauthorizedCases = []struct {
	name               string
	granted, requested []string
	want               []string
}{
	{"none granted", nil, []string{"read"}, []string{"read"}},
	{"all granted", []string{"read", "write"}, []string{"read"}, nil},
	{"dupes in requested", []string{"read", "write", "list"},
		[]string{"read", "delete", "admin", "delete"}, []string{"admin", "delete"}},
	{"nothing requested", []string{"read"}, nil, nil},
}

var sharedCases = []struct {
	name string
	a, b []string
	want []string
}{
	{"disjoint", []string{"read"}, []string{"write"}, nil},
	{"overlap", []string{"read", "write", "list"}, []string{"write", "list", "admin"},
		[]string{"list", "write"}},
	{"empty", nil, []string{"read"}, nil},
}

var distinctCases = []struct {
	name string
	in   []string
	want []string
}{
	{"with dupes", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
	{"already distinct", []string{"a", "b"}, []string{"a", "b"}},
	{"empty", nil, nil},
}

func TestUnauthorized(t *testing.T) {
	impls := map[string]func(granted, requested []string) []string{
		"stdlib":    unauthorizedStdlib,
		"container": unauthorizedContainer,
	}
	for _, impl := range slices.Sorted(maps.Keys(impls)) {
		for _, tc := range unauthorizedCases {
			t.Run(impl+"/"+tc.name, func(t *testing.T) {
				if got := impls[impl](tc.granted, tc.requested); !slices.Equal(got, tc.want) {
					t.Errorf("got %v, want %v", got, tc.want)
				}
			})
		}
	}
}

func TestShared(t *testing.T) {
	impls := map[string]func(a, b []string) []string{
		"stdlib":    sharedStdlib,
		"container": sharedContainer,
	}
	for _, impl := range slices.Sorted(maps.Keys(impls)) {
		for _, tc := range sharedCases {
			t.Run(impl+"/"+tc.name, func(t *testing.T) {
				if got := impls[impl](tc.a, tc.b); !slices.Equal(got, tc.want) {
					t.Errorf("got %v, want %v", got, tc.want)
				}
			})
		}
	}
}

func TestDistinct(t *testing.T) {
	impls := map[string]func(in []string) []string{
		"stdlib":    distinctStdlib,
		"container": distinctContainer,
	}
	for _, impl := range slices.Sorted(maps.Keys(impls)) {
		for _, tc := range distinctCases {
			t.Run(impl+"/"+tc.name, func(t *testing.T) {
				if got := impls[impl](tc.in); !slices.Equal(got, tc.want) {
					t.Errorf("got %v, want %v", got, tc.want)
				}
			})
		}
	}
}

// Examples, for godoc.

func ExampleSet() {
	var granted containers.Set[string]
	granted.Add("read", "write")

	fmt.Println(granted.Has("read"), granted.Has("delete"), granted.Len())
	// Output: true false 2
}

func ExampleSet_Difference() {
	requested := containers.NewSet("read", "delete", "admin")
	granted := containers.NewSet("read", "write")

	fmt.Println(slices.Sorted(requested.Difference(granted).All()))
	// Output: [admin delete]
}

// ===========================================================================
// SortedMap tasks, per docs/adr/0003-sorted-map.md.
//
// The baseline is a sorted slice with binary search, not `map[K]V` sorted on
// demand. The map version was written and measured first and is a strawman:
// every ordered operation re-sorts the key set, so a floor lookup sorts every
// key to answer one question. Comparing against it would flatter the container.
//
// Domain: a rate table. Key = minimum quantity for a tier, value = unit price.
// ===========================================================================

type Tier struct {
	MinQty int
	Price  string
}

// Table is the scaffolding the baseline needs before any task can be written —
// and needs again for every key/value type pair.
type Table []Tier

func (t Table) find(k int) (int, bool) {
	return slices.BinarySearchFunc(t, k, func(e Tier, k int) int { return e.MinQty - k })
}

func (t *Table) Set(k int, v string) {
	if i, found := t.find(k); found {
		(*t)[i].Price = v
	} else {
		*t = slices.Insert(*t, i, Tier{k, v})
	}
}

func newTable(tiers ...Tier) Table {
	var t Table
	for _, e := range tiers {
		t.Set(e.MinQty, e.Price)
	}
	return t
}

// ---------------------------------------------------------------------------
// Task D — all tiers in key order
//
// ADR 0003 records this as a task the container CANNOT win: a sorted slice is
// already ordered, so the baseline is the identity.
// ---------------------------------------------------------------------------

func tiersInOrderStdlib(t Table) []Tier { return t }

func tiersInOrderContainer(m *containers.SortedMap[int, string]) []Tier {
	var out []Tier
	for k, v := range m.All() {
		out = append(out, Tier{k, v})
	}
	return out
}

// ---------------------------------------------------------------------------
// Task E — tiers with lo <= key < hi
//
// Note the baseline returns a sub-slice aliasing t's backing array: callers can
// mutate the table through it. ADR 0003 rejects that shape for the container.
// ---------------------------------------------------------------------------

func tiersInRangeStdlib(t Table, lo, hi int) []Tier {
	i, _ := t.find(lo)
	j, _ := t.find(hi)
	return t[i:j]
}

func tiersInRangeContainer(m *containers.SortedMap[int, string], lo, hi int) []Tier {
	var out []Tier
	for k, v := range m.Range(lo, hi) {
		out = append(out, Tier{k, v})
	}
	return out
}

// ---------------------------------------------------------------------------
// Task F — which tier applies to a quantity (largest key <= qty)
//
// The `if !found { i-- }` adjustment and the `i < 0` guard are the off-by-one
// that hand-rolled ordered lookup gets wrong, rewritten per type.
// ---------------------------------------------------------------------------

func tierForStdlib(t Table, qty int) (Tier, bool) {
	i, found := t.find(qty)
	if !found {
		i--
	}
	if i < 0 {
		return Tier{}, false
	}
	return t[i], true
}

func tierForContainer(m *containers.SortedMap[int, string], qty int) (Tier, bool) {
	k, v, ok := m.Floor(qty)
	return Tier{k, v}, ok
}

// ---------------------------------------------------------------------------
// Cases, shared by the stdlib and (later) container implementations.
// ---------------------------------------------------------------------------

var rateTable = []Tier{{1, "1.00"}, {10, "0.90"}, {50, "0.75"}, {100, "0.60"}}

func newSortedMap(tiers ...Tier) *containers.SortedMap[int, string] {
	m := containers.NewSortedMap[int, string]()
	for _, e := range tiers {
		m.Set(e.MinQty, e.Price)
	}
	return m
}

var tiersInRangeCases = []struct {
	name   string
	lo, hi int
	want   []Tier
}{
	{"middle", 10, 100, []Tier{{10, "0.90"}, {50, "0.75"}}},
	{"all", 0, 1000, rateTable},
	{"empty span", 2, 9, nil},
	{"above everything", 500, 1000, nil},
	{"exact lower bound only", 100, 101, []Tier{{100, "0.60"}}},
}

var tierForCases = []struct {
	name   string
	qty    int
	want   Tier
	wantOK bool
}{
	{"below all", 0, Tier{}, false},
	{"exact first", 1, Tier{1, "1.00"}, true},
	{"between", 7, Tier{1, "1.00"}, true},
	{"exact middle", 50, Tier{50, "0.75"}, true},
	{"above all", 999, Tier{100, "0.60"}, true},
}

func TestTiersInOrder(t *testing.T) {
	if got := tiersInOrderStdlib(newTable(rateTable...)); !slices.Equal(got, rateTable) {
		t.Errorf("stdlib: got %v, want %v", got, rateTable)
	}
	if got := tiersInOrderContainer(newSortedMap(rateTable...)); !slices.Equal(got, rateTable) {
		t.Errorf("container: got %v, want %v", got, rateTable)
	}
}

func TestTiersInRange(t *testing.T) {
	tbl := newTable(rateTable...)
	sm := newSortedMap(rateTable...)
	for _, tc := range tiersInRangeCases {
		t.Run("stdlib/"+tc.name, func(t *testing.T) {
			if got := tiersInRangeStdlib(tbl, tc.lo, tc.hi); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
		t.Run("container/"+tc.name, func(t *testing.T) {
			if got := tiersInRangeContainer(sm, tc.lo, tc.hi); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTierFor(t *testing.T) {
	tbl := newTable(rateTable...)
	sm := newSortedMap(rateTable...)
	for _, tc := range tierForCases {
		t.Run("stdlib/"+tc.name, func(t *testing.T) {
			got, ok := tierForStdlib(tbl, tc.qty)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("got %v,%v want %v,%v", got, ok, tc.want, tc.wantOK)
			}
		})
		t.Run("container/"+tc.name, func(t *testing.T) {
			got, ok := tierForContainer(sm, tc.qty)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("got %v,%v want %v,%v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func ExampleSortedMap_Floor() {
	rates := containers.NewSortedMap[int, string]()
	rates.Set(1, "1.00")
	rates.Set(10, "0.90")
	rates.Set(50, "0.75")

	minQty, price, ok := rates.Floor(37)
	fmt.Println(minQty, price, ok)
	// Output: 10 0.90 true
}

func ExampleSortedMap_Range() {
	rates := containers.NewSortedMap[int, string]()
	for k, v := range map[int]string{1: "1.00", 10: "0.90", 50: "0.75", 100: "0.60"} {
		rates.Set(k, v)
	}
	for k, v := range rates.Range(10, 100) {
		fmt.Println(k, v)
	}
	// Output:
	// 10 0.90
	// 50 0.75
}

// ---------------------------------------------------------------------------
// Task G — load a rate table from an unordered source
//
// The baseline sorts the keys first, so every insert into the Table is an
// append rather than an O(n) memmove. That is what a careful author writes, and
// it is the fair comparison; the obvious `for k, v := range src` version is
// O(kn) and would be a strawman. See docs/adr/0004-bulk-insert.md.
// ---------------------------------------------------------------------------

func bulkLoadStdlib(src map[int]string) Table {
	var t Table
	for _, k := range slices.Sorted(maps.Keys(src)) {
		t.Set(k, src[k])
	}
	return t
}

var bulkLoadCases = []struct {
	name string
	src  map[int]string
	want []Tier
}{
	{"rate table", map[int]string{100: "0.60", 1: "1.00", 50: "0.75", 10: "0.90"}, rateTable},
	{"single", map[int]string{7: "x"}, []Tier{{7, "x"}}},
	{"empty", map[int]string{}, nil},
}

func TestBulkLoad(t *testing.T) {
	for _, tc := range bulkLoadCases {
		t.Run("stdlib/"+tc.name, func(t *testing.T) {
			if got := bulkLoadStdlib(tc.src); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
