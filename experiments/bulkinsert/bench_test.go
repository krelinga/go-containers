package bulkinsert

import (
	"cmp"
	"fmt"
	"math/rand"
	"slices"
	"testing"
)

type ent struct {
	k int
	v string
}

func find(es []ent, k int) (int, bool) {
	return slices.BinarySearchFunc(es, k, func(e ent, k int) int { return cmp.Compare(e.k, k) })
}

// naive: what a caller writes today -- repeated Set.
func naive(base []ent, add []ent) []ent {
	out := slices.Clone(base)
	for _, e := range add {
		if i, found := find(out, e.k); found {
			out[i].v = e.v
		} else {
			out = slices.Insert(out, i, e)
		}
	}
	return out
}

// bulk: sort the additions once, then merge two sorted runs.
func bulk(base []ent, add []ent) []ent {
	s := slices.Clone(add)
	slices.SortStableFunc(s, func(a, b ent) int { return cmp.Compare(a.k, b.k) })
	// NOTE: CompactFunc keeps the FIRST of each run, so this prototype gives
	// first-write-wins for duplicate keys within the input, while repeated Set
	// gives last-write-wins. ADR 0004 settles that as last-wins; the divergence
	// is left here because it does not affect the timings and is worth seeing.
	s = slices.CompactFunc(s, func(a, b ent) bool { return a.k == b.k })

	out := make([]ent, 0, len(base)+len(s))
	i, j := 0, 0
	for i < len(base) && j < len(s) {
		switch cmp.Compare(base[i].k, s[j].k) {
		case -1:
			out = append(out, base[i])
			i++
		case 1:
			out = append(out, s[j])
			j++
		default:
			out = append(out, s[j]) // replace
			i++
			j++
		}
	}
	return append(append(out, base[i:]...), s[j:]...)
}

func fixture(n, k int) ([]ent, []ent) {
	r := rand.New(rand.NewSource(1))
	base := make([]ent, n)
	for i := range base {
		base[i] = ent{i * 2, "old"} // even keys
	}
	add := make([]ent, k)
	for i := range add {
		add[i] = ent{r.Intn(n*2 + 1), "new"} // random, interleaved, may collide
	}
	return base, add
}

var sink []ent

func BenchmarkBulk(b *testing.B) {
	for _, n := range []int{1000, 10000, 100000} {
		for _, k := range []int{1, 4, 16, 64, 256, 1024} {
			base, add := fixture(n, k)
			b.Run(fmt.Sprintf("naive/n=%d/k=%d", n, k), func(b *testing.B) {
				for b.Loop() {
					sink = naive(base, add)
				}
			})
			b.Run(fmt.Sprintf("bulk/n=%d/k=%d", n, k), func(b *testing.B) {
				for b.Loop() {
					sink = bulk(base, add)
				}
			})
		}
	}
}

func TestAgree(t *testing.T) {
	for _, n := range []int{0, 1, 10, 500} {
		for _, k := range []int{0, 1, 7, 100} {
			base, add := fixture(n, k)
			a, b2 := naive(base, add), bulk(base, add)
			if !slices.Equal(a, b2) {
				t.Errorf("n=%d k=%d disagree:\n naive=%v\n bulk =%v", n, k, a, b2)
			}
		}
	}
}

func BenchmarkSortCost(b *testing.B) {
	cmpf := func(a, b ent) int { return cmp.Compare(a.k, b.k) }
	for _, k := range []int{16, 256, 4096} {
		asc := make([]ent, k)
		for i := range asc {
			asc[i] = ent{i, "v"}
		}
		rnd := slices.Clone(asc)
		rand.New(rand.NewSource(1)).Shuffle(k, func(i, j int) { rnd[i], rnd[j] = rnd[j], rnd[i] })

		b.Run(fmt.Sprintf("sorted/%d", k), func(b *testing.B) {
			buf := make([]ent, k)
			for b.Loop() {
				copy(buf, asc)
				slices.SortStableFunc(buf, cmpf)
			}
			sink = buf
		})
		b.Run(fmt.Sprintf("random/%d", k), func(b *testing.B) {
			buf := make([]ent, k)
			for b.Loop() {
				copy(buf, rnd)
				slices.SortStableFunc(buf, cmpf)
			}
			sink = buf
		})
		b.Run(fmt.Sprintf("copyonly/%d", k), func(b *testing.B) {
			buf := make([]ent, k)
			for b.Loop() {
				copy(buf, asc)
			}
			sink = buf
		})
	}
}

// BenchmarkNoOp is the zero-cost baseline: it detects harness overhead leaking
// into the numbers above.
func BenchmarkNoOp(b *testing.B) {
	base, _ := fixture(64, 1)
	for b.Loop() {
		sink = base
	}
}
