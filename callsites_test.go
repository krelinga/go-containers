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
// Tasks correspond to those in the ADR that proposed each container: A-C from
// 0002, D-G from 0003 and 0009, H-I from 0015.
package containers_test

import (
	"cmp"
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
	missing := containers.NewHashSet(requested...).Difference(containers.NewHashSet(granted...))
	return slices.Sorted(missing.Keys())
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
	return slices.Sorted(containers.NewHashSet(a...).Intersect(containers.NewHashSet(b...)).Keys())
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
	return slices.Sorted(containers.NewHashSet(in...).Keys())
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

func ExampleHashSet() {
	var granted containers.HashSet[string]
	granted.AddAll("read", "write")

	fmt.Println(granted.Has("read"), granted.Has("delete"), granted.Len())
	// Output: true false 2
}

func ExampleHashSet_Difference() {
	requested := containers.NewHashSet("read", "delete", "admin")
	granted := containers.NewHashSet("read", "write")

	fmt.Println(slices.Sorted(requested.Difference(granted).Keys()))
	// Output: [admin delete]
}

// ===========================================================================
// SortedDict tasks, per docs/adr/0003-sorted-map.md.
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

func tiersInOrderContainer(m *containers.SortedDict[int, string]) []Tier {
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

func tiersInRangeContainer(m *containers.SortedDict[int, string], lo, hi int) []Tier {
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

func tierForContainer(m *containers.SortedDict[int, string], qty int) (Tier, bool) {
	k, v, ok := m.Floor(qty)
	return Tier{k, v}, ok
}

// ---------------------------------------------------------------------------
// Cases, shared by the stdlib and (later) container implementations.
// ---------------------------------------------------------------------------

var rateTable = []Tier{{1, "1.00"}, {10, "0.90"}, {50, "0.75"}, {100, "0.60"}}

func newSortedDict(tiers ...Tier) *containers.SortedDict[int, string] {
	m := containers.NewSortedDict[int, string]()
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
	if got := tiersInOrderContainer(newSortedDict(rateTable...)); !slices.Equal(got, rateTable) {
		t.Errorf("container: got %v, want %v", got, rateTable)
	}
}

func TestTiersInRange(t *testing.T) {
	tbl := newTable(rateTable...)
	sm := newSortedDict(rateTable...)
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
	sm := newSortedDict(rateTable...)
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

func ExampleSortedDict_Floor() {
	rates := containers.NewSortedDict[int, string]()
	rates.Set(1, "1.00")
	rates.Set(10, "0.90")
	rates.Set(50, "0.75")

	minQty, price, ok := rates.Floor(37)
	fmt.Println(minQty, price, ok)
	// Output: 10 0.90 true
}

func ExampleSortedDict_Range() {
	rates := containers.NewSortedDict[int, string]()
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

func bulkLoadContainer(src map[int]string) []Tier {
	var out []Tier
	for k, v := range containers.NewSortedDict(pairsOf(maps.All(src))...).All() {
		out = append(out, Tier{k, v})
	}
	return out
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
		t.Run("container/"+tc.name, func(t *testing.T) {
			if got := bulkLoadContainer(tc.src); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func ExampleNewSortedDict() {
	rates := containers.NewSortedDict(pairsOf(maps.All(map[int]string{
		100: "0.60", 1: "1.00", 50: "0.75", 10: "0.90",
	}))...)
	for k, v := range rates.All() {
		fmt.Println(k, v)
	}
	// Output:
	// 1 1.00
	// 10 0.90
	// 50 0.75
	// 100 0.60
}

// ===========================================================================
// SortedSet tasks, per docs/adr/0005-sorted-set.md.
//
// Domain: deployed release version numbers.
//
// Two baselines are plausible and both appear below. The fair one, matching the
// standard ADR 0003 held SortedDict to, is a hand-maintained sorted []int. The
// other is what a user of THIS library would reach for today -- HashSet[int] plus
// slices.Sorted -- which is O(n log n) per query because it re-sorts every time.
// ===========================================================================

type Versions []int

func (v *Versions) Add(n int) {
	if i, found := slices.BinarySearch(*v, n); !found {
		*v = slices.Insert(*v, i, n)
	}
}

func newVersions(ns ...int) Versions {
	var v Versions
	for _, n := range ns {
		v.Add(n)
	}
	return v
}

// Task H — all versions in order. The baseline is already sorted.
func versionsInOrderStdlib(v Versions) []int { return v }

func versionsInOrderContainer(s *containers.SortedSet[int]) []int {
	return slices.Collect(s.Keys())
}

// Task I — versions with lo <= n < hi.
func versionsInRangeStdlib(v Versions, lo, hi int) []int {
	i, _ := slices.BinarySearch(v, lo)
	j, _ := slices.BinarySearch(v, hi)
	return v[i:j]
}

func versionsInRangeContainer(s *containers.SortedSet[int], lo, hi int) []int {
	return slices.Collect(s.Range(lo, hi))
}

// Task J — newest version at or before n.
func versionAtOrBeforeStdlib(v Versions, n int) (int, bool) {
	i, found := slices.BinarySearch(v, n)
	if !found {
		i--
	}
	if i < 0 {
		return 0, false
	}
	return v[i], true
}

func versionAtOrBeforeContainer(s *containers.SortedSet[int], n int) (int, bool) {
	return s.Floor(n)
}

// The same task using the Set this library already ships. Correct, and
// O(n log n) per call because it re-sorts the whole set to answer one question.
func versionAtOrBeforeViaSet(s *containers.HashSet[int], n int) (int, bool) {
	vs := slices.Sorted(s.Keys())
	i, found := slices.BinarySearch(vs, n)
	if !found {
		i--
	}
	if i < 0 {
		return 0, false
	}
	return vs[i], true
}

var releaseVersions = []int{3, 7, 12, 40}

var versionRangeCases = []struct {
	name   string
	lo, hi int
	want   []int
}{
	{"middle", 7, 40, []int{7, 12}},
	{"all", 0, 100, releaseVersions},
	{"empty span", 4, 6, nil},
	{"above everything", 50, 99, nil},
	{"inverted is empty", 40, 7, nil},
}

var versionFloorCases = []struct {
	name   string
	n      int
	want   int
	wantOK bool
}{
	{"below all", 1, 0, false},
	{"exact first", 3, 3, true},
	{"between", 9, 7, true},
	{"exact last", 40, 40, true},
	{"above all", 999, 40, true},
}

func TestVersionsInOrder(t *testing.T) {
	if got := versionsInOrderStdlib(newVersions(40, 3, 12, 7, 3)); !slices.Equal(got, releaseVersions) {
		t.Errorf("stdlib: got %v, want %v", got, releaseVersions)
	}
	if got := versionsInOrderContainer(containers.NewSortedSet(40, 3, 12, 7, 3)); !slices.Equal(got, releaseVersions) {
		t.Errorf("container: got %v, want %v", got, releaseVersions)
	}
}

func TestVersionsInRange(t *testing.T) {
	v := newVersions(releaseVersions...)
	ss := containers.NewSortedSet(releaseVersions...)
	for _, tc := range versionRangeCases {
		t.Run("stdlib/"+tc.name, func(t *testing.T) {
			lo, hi := tc.lo, tc.hi
			if hi < lo {
				hi = lo // the baseline would panic on inverted bounds
			}
			if got := versionsInRangeStdlib(v, lo, hi); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
		// No workaround needed: the container treats inverted bounds as empty.
		t.Run("container/"+tc.name, func(t *testing.T) {
			if got := versionsInRangeContainer(ss, tc.lo, tc.hi); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestVersionAtOrBefore(t *testing.T) {
	v := newVersions(releaseVersions...)
	s := containers.NewHashSet(releaseVersions...)
	ss := containers.NewSortedSet(releaseVersions...)
	for _, tc := range versionFloorCases {
		t.Run("stdlib/"+tc.name, func(t *testing.T) {
			if got, ok := versionAtOrBeforeStdlib(v, tc.n); got != tc.want || ok != tc.wantOK {
				t.Errorf("got %v,%v want %v,%v", got, ok, tc.want, tc.wantOK)
			}
		})
		t.Run("viaSet/"+tc.name, func(t *testing.T) {
			if got, ok := versionAtOrBeforeViaSet(s, tc.n); got != tc.want || ok != tc.wantOK {
				t.Errorf("got %v,%v want %v,%v", got, ok, tc.want, tc.wantOK)
			}
		})
		t.Run("container/"+tc.name, func(t *testing.T) {
			if got, ok := versionAtOrBeforeContainer(ss, tc.n); got != tc.want || ok != tc.wantOK {
				t.Errorf("got %v,%v want %v,%v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func ExampleSortedSet_Floor() {
	deployed := containers.NewSortedSet(3, 7, 12, 40)
	v, ok := deployed.Floor(9)
	fmt.Println(v, ok)
	// Output: 7 true
}

func ExampleSortedSet_Range() {
	deployed := containers.NewSortedSet(3, 7, 12, 40)
	fmt.Println(slices.Collect(deployed.Range(7, 40)))
	// Output: [7 12]
}

// ===========================================================================
// Generic-map tasks, per docs/adr/0007-map.md.
//
// Task K is the duplication the contract is meant to remove: the same
// operation written once per backing, because a builtin map has no methods.
//
// Note the two are NOT the same code. The Go spec permits deleting from a map
// while ranging over it, so the map version deletes in place. SortedDict.All
// binds its backing slice and Delete shifts within it, so doing the same there
// corrupts the iteration -- it revisits keys and skips others. The sorted
// version must collect first.
// ===========================================================================

func pruneMapStdlib[K comparable, V any](m map[K]V, keep func(V) bool) {
	for k, v := range m {
		if !keep(v) {
			delete(m, k) // safe: the spec permits deletion during map iteration
		}
	}
}

func pruneSortedStdlib[K cmp.Ordered, V any](m *containers.SortedDict[K, V], keep func(V) bool) {
	var drop []K
	for k, v := range m.All() {
		if !keep(v) {
			drop = append(drop, k) // must not delete while iterating
		}
	}
	for _, k := range drop {
		m.Delete(k)
	}
}

// The container version: one function body for both backings. It uses the more
// conservative iteration discipline, so it collects keys where pruneMapStdlib
// deletes in place -- the cost of working against both.
func pruneContainer[K comparable, V any](m containers.MutableDict[K, V], keep func(V) bool) {
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

var pruneCases = []struct {
	name string
	in   map[int]int
	want []int // keys remaining, ascending
}{
	{"drops odds", map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 4}, []int{0, 2, 4}},
	{"drops nothing", map[int]int{0: 0, 2: 2}, []int{0, 2}},
	{"drops everything", map[int]int{1: 1, 3: 3}, nil},
	{"empty", map[int]int{}, nil},
}

func keepEven(v int) bool { return v%2 == 0 }

func TestPrune(t *testing.T) {
	for _, tc := range pruneCases {
		t.Run("stdlib-map/"+tc.name, func(t *testing.T) {
			m := maps.Clone(tc.in)
			pruneMapStdlib(m, keepEven)
			if got := slices.Sorted(maps.Keys(m)); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
		t.Run("stdlib-sorted/"+tc.name, func(t *testing.T) {
			sm := containers.NewSortedDict[int, int]()
			sm.SetAll(pairsOf(maps.All(tc.in))...)
			pruneSortedStdlib(sm, keepEven)
			if got := slices.Sorted(maps.Keys(maps.Collect(sm.All()))); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
		// One body, both backings.
		t.Run("container-map/"+tc.name, func(t *testing.T) {
			m := containers.Map[int, int](maps.Clone(tc.in))
			pruneContainer[int, int](m, keepEven)
			if got := slices.Sorted(maps.Keys(m)); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
		// HashDict takes the pointer, like every other container -- the
		// asymmetry ADR 0007 recorded for Map, now avoidable.
		t.Run("container-hashdict/"+tc.name, func(t *testing.T) {
			hd := containers.NewHashDict[int, int]()
			hd.SetAll(pairsOf(maps.All(tc.in))...)
			pruneContainer[int, int](hd, keepEven)
			if got := slices.Sorted(maps.Keys(maps.Collect(hd.All()))); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
		t.Run("container-sorted/"+tc.name, func(t *testing.T) {
			sm := containers.NewSortedDict[int, int]()
			sm.SetAll(pairsOf(maps.All(tc.in))...)
			pruneContainer[int, int](sm, keepEven)
			if got := slices.Sorted(maps.Keys(maps.Collect(sm.All()))); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func ExampleMap() {
	limits := containers.Map[string, int](map[string]int{"cpu": 4, "mem": 16})
	limits.Set("disk", 100)

	// The conversion aliased the original, and builtin syntax still works.
	fmt.Println(limits.Len(), limits["cpu"], slices.Sorted(maps.Keys(limits)))
	// Output: 3 4 [cpu disk mem]
}

func ExampleHashDict() {
	// The zero value is usable, unlike Map's.
	var limits containers.HashDict[string, int]
	limits.Set("cpu", 4)
	limits.Set("mem", 16)

	fmt.Println(limits.Len(), slices.Sorted(maps.Keys(maps.Collect(limits.All()))))
	// Output: 2 [cpu mem]
}

func ExampleNewHashDict() {
	// Building one dict from another presizes from the source's length.
	src := containers.NewSortedDict[string, int]()
	src.SetAll(pairsOf(maps.All(map[string]int{"b": 2, "a": 1, "c": 3}))...)

	byHash := containers.NewHashDict(src.AllSlice()...)
	fmt.Println(byHash.Len(), slices.Sorted(maps.Keys(maps.Collect(byHash.All()))))
	// Output: 3 [a b c]
}

// ---------------------------------------------------------------------------
// Task H — a type that owns a growing sequence and exposes it
//
// ADR 0001's accessor problem in its sequence form. A []T field cannot be
// handed out read-only, so the accessor either copies on every call -- O(n)
// each time, O(n²) in a caller's loop -- or returns the interior and lets the
// caller write into the log. This baseline takes the safe horn.
// ---------------------------------------------------------------------------

type auditLogStdlib struct{ entries []string }

func (l *auditLogStdlib) Record(e string) { l.entries = append(l.entries, e) }

// Safe, and O(n) on every call.
func (l *auditLogStdlib) Entries() []string { return slices.Clone(l.entries) }

type auditLogContainer struct{ entries containers.Vector[string] }

func (l *auditLogContainer) Record(e string) { l.entries.Append(e) }

// Safe, O(1), and no allocation: the view cannot write, so there is nothing to
// defend against by copying.
func (l *auditLogContainer) Entries() containers.IndexedView[string] {
	return containers.ViewVectorIdentity(&l.entries)
}

// ---------------------------------------------------------------------------
// Task I — accumulating across a function boundary
//
// The callee cannot append in place: a slice header is a value, so it must
// return the slice and the caller must reassign. Forgetting is silent.
// ---------------------------------------------------------------------------

func addDefaultsStdlib(out []string, defaults ...string) []string {
	return append(out, defaults...)
}

// The callee appends in place. There is no return value to forget.
func addDefaultsContainer(out *containers.Vector[string], defaults ...string) {
	out.AppendAll(slices.Collect(slices.Values(defaults))...)
}

// ---------------------------------------------------------------------------
// Cases, shared by the stdlib and (later) container implementations.
// ---------------------------------------------------------------------------

var sequenceCases = []struct {
	name   string
	record []string
	want   []string
}{
	{"empty", nil, nil},
	{"one", []string{"login"}, []string{"login"}},
	{"several", []string{"login", "read", "write"}, []string{"login", "read", "write"}},
	{"duplicates are kept", []string{"read", "read"}, []string{"read", "read"}},
}

func TestTaskHStdlib(t *testing.T) {
	for _, tc := range sequenceCases {
		t.Run(tc.name, func(t *testing.T) {
			var l auditLogStdlib
			for _, e := range tc.record {
				l.Record(e)
			}
			if got := l.Entries(); !slices.Equal(got, tc.want) {
				t.Errorf("Entries() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTaskHContainer(t *testing.T) {
	for _, tc := range sequenceCases {
		t.Run(tc.name, func(t *testing.T) {
			var l auditLogContainer
			for _, e := range tc.record {
				l.Record(e)
			}
			if got := l.Entries().ValueSlice(); !slices.Equal(got, tc.want) {
				t.Errorf("Entries() = %v, want %v", got, tc.want)
			}
		})
	}
}

// The property the container version must also have: what the accessor hands
// back cannot be used to corrupt the log.
func TestTaskHStdlibAccessorDoesNotLeak(t *testing.T) {
	var l auditLogStdlib
	l.Record("login")

	got := l.Entries()
	got[0] = "TAMPERED"

	if after := l.Entries(); after[0] != "login" {
		t.Errorf("log was mutated through its accessor: %v", after)
	}
}

// ADR 0001's guardrail, and the whole point of Task H. The stdlib accessor must
// copy to be safe, so it allocates on every call and the cost grows with the
// log; the container's hands out a view and allocates nothing, at any size.
func TestTaskHAccessorAllocations(t *testing.T) {
	var sl auditLogStdlib
	var cn auditLogContainer
	for i := range 64 {
		e := fmt.Sprint(i)
		sl.Record(e)
		cn.Record(e)
	}

	var sinkSlice []string
	if got := testing.AllocsPerRun(100, func() { sinkSlice = sl.Entries() }); got == 0 {
		t.Errorf("expected the copying accessor to allocate, got %v", got)
	}
	_ = sinkSlice

	var sinkView containers.IndexedView[string]
	if got := testing.AllocsPerRun(100, func() { sinkView = cn.Entries() }); got != 0 {
		t.Errorf("view accessor allocated %v times, want 0", got)
	}
	if sinkView.Len() != 64 {
		t.Errorf("Len = %d, want 64", sinkView.Len())
	}
}

// The stdlib version is safe only because it copies. The container version is
// safe because the view has no way to write at all -- there is no Set, no
// Append, and no path back to the Vector.
func TestTaskHContainerAccessorCannotWrite(t *testing.T) {
	var l auditLogContainer
	l.Record("login")

	got := l.Entries()
	if _, ok := any(got).(interface{ Set(int, string) }); ok {
		t.Error("view exposed Set")
	}
	if _, ok := any(got).(*containers.Vector[string]); ok {
		t.Error("view was assertable back to the Vector")
	}
	if got.At(0) != "login" {
		t.Errorf("At(0) = %q", got.At(0))
	}
}

func TestTaskIStdlib(t *testing.T) {
	out := []string{"explicit"}
	out = addDefaultsStdlib(out, "a", "b")

	want := []string{"explicit", "a", "b"}
	if !slices.Equal(out, want) {
		t.Errorf("addDefaultsStdlib = %v, want %v", out, want)
	}
}

func TestTaskIContainer(t *testing.T) {
	out := containers.NewVector("explicit")
	addDefaultsContainer(out, "a", "b")

	want := []string{"explicit", "a", "b"}
	if got := out.ValueSlice(); !slices.Equal(got, want) {
		t.Errorf("addDefaultsContainer = %v, want %v", got, want)
	}
}

// The footgun the container version removes: with a slice the callee's work is
// lost unless the caller reassigns, and nothing at the call site says so. With
// a Vector there is no return value to forget.
func TestTaskIForgettingTheAssignment(t *testing.T) {
	sl := make([]string, 0, 8) // spare capacity, so nothing reallocates
	addDefaultsStdlib(sl, "a", "b")
	if len(sl) != 0 {
		t.Fatalf("expected the stdlib append to be lost, got %v", sl)
	}

	cn := containers.NewVector[string]()
	addDefaultsContainer(cn, "a", "b")
	if cn.Len() != 2 {
		t.Errorf("container append was lost: Len = %d, want 2", cn.Len())
	}
}

// A Vector accessor keeps working across the reallocations that would split
// two holders of a slice header.
func ExampleVector() {
	var log containers.Vector[string]
	view := containers.ViewVectorIdentity(&log)

	for _, e := range []string{"login", "read", "write"} {
		log.Append(e) // reallocates as it grows
	}

	// The view was taken before any of those appends, and sees all of them.
	fmt.Println(view.Len(), view.ValueSlice())
	// Output: 3 [login read write]
}

// ---------------------------------------------------------------------------
// Task N — exposing a slice field read-only WITHOUT changing its type
//
// Task H's answer was to migrate the field to a Vector. That is right when the
// type wants reallocation hidden, and heavy when it does not: append, range,
// indexing and encoding/json all stop working on the field. ADR 0016's answer
// keeps the field a []string and changes only the accessor.
//
// The stdlib baseline is auditLogStdlib above, unchanged.
// ---------------------------------------------------------------------------

type auditLogSliceView struct{ entries []string }

func (l *auditLogSliceView) Record(e string) { l.entries = append(l.entries, e) }

func (l *auditLogSliceView) Entries() containers.IndexedView[string] {
	return containers.ViewSliceIdentity(l.entries)
}

func TestTaskNContainer(t *testing.T) {
	for _, tc := range sequenceCases {
		t.Run(tc.name, func(t *testing.T) {
			var l auditLogSliceView
			for _, e := range tc.record {
				l.Record(e)
			}
			if got := l.Entries().ValueSlice(); !slices.Equal(got, tc.want) {
				t.Errorf("Entries() = %v, want %v", got, tc.want)
			}
		})
	}
}

// The point of Task N: the accessor does not copy, so its cost does not grow
// with the log, and the caller still cannot write through it.
//
// Not copying IS the O(1) proof -- an accessor that returns without copying
// cannot be linear in the log's size. Measured for the record and then removed
// from the suite, because it cost 4.8s against 0.011s for everything else:
// the copying accessor allocates 64 B at n=4 and 65 536 B at n=4096, while the
// view allocates 24 B at both. Both are one allocation; only one of them grows.
func TestTaskNAccessorDoesNotCopy(t *testing.T) {
	var l auditLogSliceView
	l.Record("login")

	view := l.Entries()
	// Not a copy: a write to the backing slice shows through.
	l.entries[0] = "rewritten"
	if got := view.At(0); got != "rewritten" {
		t.Errorf("accessor copied: At(0) = %q", got)
	}
	// But the caller has no way to write.
	if _, ok := any(view).(interface{ Set(int, string) }); ok {
		t.Error("view exposed a mutator")
	}
	if _, ok := any(view).([]string); ok {
		t.Error("view was assertable back to the slice")
	}
}
