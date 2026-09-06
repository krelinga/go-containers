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
