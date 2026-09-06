package sortedbacking

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/google/btree"
)

type entry struct {
	k int
	v string
}

func less(a, b entry) bool { return a.k < b.k }

// ---- sorted-slice map -----------------------------------------------------

type sliceMap []entry

func (m *sliceMap) find(k int) (int, bool) {
	return slices.BinarySearchFunc(*m, k, func(e entry, k int) int { return e.k - k })
}

func (m *sliceMap) Set(k int, v string) {
	if i, found := m.find(k); found {
		(*m)[i].v = v
	} else {
		*m = slices.Insert(*m, i, entry{k, v})
	}
}

func (m *sliceMap) Get(k int) (string, bool) {
	if i, found := m.find(k); found {
		return (*m)[i].v, true
	}
	return "", false
}

// Range returns a sub-slice: no copy, no allocation.
func (m *sliceMap) Range(lo, hi int) []entry {
	i, _ := m.find(lo)
	j, _ := m.find(hi)
	return (*m)[i:j]
}

// ---- fixtures -------------------------------------------------------------

var buildSizes = []int{8, 64, 512, 4096, 16384, 65536}
var querySizes = []int{8, 64, 512, 4096, 65536, 262144}

// randomKeys returns a deterministic shuffle of 0..n-1.
func randomKeys(n int) []int {
	ks := make([]int, n)
	for i := range ks {
		ks[i] = i
	}
	rand.New(rand.NewSource(1)).Shuffle(n, func(i, j int) { ks[i], ks[j] = ks[j], ks[i] })
	return ks
}

func seqKeys(n int) []int {
	ks := make([]int, n)
	for i := range ks {
		ks[i] = i
	}
	return ks
}

func buildSlice(keys []int) *sliceMap {
	m := make(sliceMap, 0, len(keys))
	for _, k := range keys {
		m.Set(k, "v")
	}
	return &m
}

func buildTree(keys []int) *btree.BTreeG[entry] {
	t := btree.NewG(32, less)
	for _, k := range keys {
		t.ReplaceOrInsert(entry{k, "v"})
	}
	return t
}

// Typed sinks: an `any` sink would box and add an allocation per measurement.
var (
	sinkSlice  *sliceMap
	sinkTree   *btree.BTreeG[entry]
	sinkString string
	sinkBool   bool
	sinkInt    int
	sinkEntry  []entry
)

// ---- build ----------------------------------------------------------------

func BenchmarkBuildRandom(b *testing.B) {
	for _, n := range buildSizes {
		keys := randomKeys(n)
		b.Run(fmt.Sprintf("slice/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkSlice = buildSlice(keys)
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/insert")
		})
		b.Run(fmt.Sprintf("btree/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkTree = buildTree(keys)
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/insert")
		})
	}
}

func BenchmarkBuildSequential(b *testing.B) {
	for _, n := range buildSizes {
		keys := seqKeys(n)
		b.Run(fmt.Sprintf("slice/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkSlice = buildSlice(keys)
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/insert")
		})
		b.Run(fmt.Sprintf("btree/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkTree = buildTree(keys)
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/insert")
		})
	}
}

// ---- lookup ---------------------------------------------------------------

func BenchmarkLookup(b *testing.B) {
	for _, n := range querySizes {
		keys := randomKeys(n)
		sm, bt := buildSlice(keys), buildTree(keys)
		probe := n / 2
		b.Run(fmt.Sprintf("slice/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkString, sinkBool = sm.Get(probe)
			}
		})
		b.Run(fmt.Sprintf("btree/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var e entry
				e, sinkBool = bt.Get(entry{k: probe})
				sinkString = e.v
			}
		})
	}
}

// ---- full ordered iteration ----------------------------------------------

func BenchmarkIterateAll(b *testing.B) {
	for _, n := range querySizes {
		keys := randomKeys(n)
		sm, bt := buildSlice(keys), buildTree(keys)
		b.Run(fmt.Sprintf("slice/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				t := 0
				for _, e := range *sm {
					t += e.k
				}
				sinkInt = t
			}
		})
		b.Run(fmt.Sprintf("btree/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				t := 0
				bt.Ascend(func(e entry) bool { t += e.k; return true })
				sinkInt = t
			}
		})
	}
}

// ---- range scan over 1% of the keys ---------------------------------------

func BenchmarkRangeScan(b *testing.B) {
	for _, n := range querySizes {
		keys := randomKeys(n)
		sm, bt := buildSlice(keys), buildTree(keys)
		lo := n / 4
		hi := lo + max(1, n/100)
		// Both sides must consume every matched entry, or this compares a
		// sub-slice reslice against a full tree walk.
		b.Run(fmt.Sprintf("slice/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				t := 0
				for _, e := range sm.Range(lo, hi) {
					t += e.k
				}
				sinkInt = t
			}
		})
		b.Run(fmt.Sprintf("btree/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				t := 0
				bt.AscendRange(entry{k: lo}, entry{k: hi}, func(e entry) bool { t += e.k; return true })
				sinkInt = t
			}
		})
	}
}

// ---- zero-cost baseline: detects harness overhead leaking into the numbers --

func BenchmarkNoOp(b *testing.B) {
	keys := randomKeys(64)
	sm := buildSlice(keys)
	b.ReportAllocs()
	for b.Loop() {
		sinkSlice = sm
	}
}

// BenchmarkRangeLocate measures only finding the range bounds, without
// consuming the entries. A sorted slice can hand back a sub-slice in O(log n);
// a tree has no equivalent, so this is the one place the structures differ in
// kind rather than in constant factor.
func BenchmarkRangeLocate(b *testing.B) {
	for _, n := range querySizes {
		keys := randomKeys(n)
		sm := buildSlice(keys)
		lo := n / 4
		hi := lo + max(1, n/100)
		b.Run(fmt.Sprintf("slice/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkEntry = sm.Range(lo, hi)
			}
		})
	}
}
