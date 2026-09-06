package sortedsetbacking

import (
	"cmp"
	"fmt"
	"iter"
	"math/rand"
	"slices"
	"testing"
	"unsafe"
)

// entryVLast is SortedMap's layout today: zero-sized value field last, so the
// struct is padded.
type entryVLast[T cmp.Ordered] struct {
	k T
	v struct{}
}

// entryVFirst reorders the same fields. No padding.
type entryVFirst[T cmp.Ordered] struct {
	v struct{}
	k T
}

var sizes = []int{64, 1024, 16384, 262144}

func randomKeys(n int) []int {
	ks := make([]int, n)
	for i := range ks {
		ks[i] = i
	}
	rand.New(rand.NewSource(1)).Shuffle(n, func(i, j int) { ks[i], ks[j] = ks[j], ks[i] })
	return ks
}

// Typed sinks.
var (
	sinkBare   []int
	sinkVLast  []entryVLast[int]
	sinkVFirst []entryVFirst[int]
	sinkInt    int
	sinkBool   bool
)

// ---- build: time and, more importantly, bytes ----------------------------

func BenchmarkBuild(b *testing.B) {
	for _, n := range sizes {
		keys := randomKeys(n)
		b.Run(fmt.Sprintf("bare/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				out := make([]int, 0, n)
				for _, k := range keys {
					i, _ := slices.BinarySearch(out, k)
					out = slices.Insert(out, i, k)
				}
				sinkBare = out
			}
		})
		b.Run(fmt.Sprintf("vlast/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				out := make([]entryVLast[int], 0, n)
				for _, k := range keys {
					i, _ := slices.BinarySearchFunc(out, k, func(e entryVLast[int], k int) int { return cmp.Compare(e.k, k) })
					out = slices.Insert(out, i, entryVLast[int]{k: k})
				}
				sinkVLast = out
			}
		})
		b.Run(fmt.Sprintf("vfirst/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				out := make([]entryVFirst[int], 0, n)
				for _, k := range keys {
					i, _ := slices.BinarySearchFunc(out, k, func(e entryVFirst[int], k int) int { return cmp.Compare(e.k, k) })
					out = slices.Insert(out, i, entryVFirst[int]{k: k})
				}
				sinkVFirst = out
			}
		})
	}
}

// ---- lookup ---------------------------------------------------------------

func BenchmarkLookup(b *testing.B) {
	for _, n := range sizes {
		bare := make([]int, n)
		vlast := make([]entryVLast[int], n)
		vfirst := make([]entryVFirst[int], n)
		for i := range n {
			bare[i], vlast[i].k, vfirst[i].k = i, i, i
		}
		probe := n / 2
		b.Run(fmt.Sprintf("bare/%d", n), func(b *testing.B) {
			for b.Loop() {
				sinkInt, sinkBool = slices.BinarySearch(bare, probe)
			}
		})
		b.Run(fmt.Sprintf("vlast/%d", n), func(b *testing.B) {
			for b.Loop() {
				sinkInt, sinkBool = slices.BinarySearchFunc(vlast, probe, func(e entryVLast[int], k int) int { return cmp.Compare(e.k, k) })
			}
		})
		b.Run(fmt.Sprintf("vfirst/%d", n), func(b *testing.B) {
			for b.Loop() {
				sinkInt, sinkBool = slices.BinarySearchFunc(vfirst, probe, func(e entryVFirst[int], k int) int { return cmp.Compare(e.k, k) })
			}
		})
	}
}

// ---- iteration, including the Seq2 adapter a delegating SortedSet pays ----

func seqBare(es []int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for _, k := range es {
			if !yield(k) {
				return
			}
		}
	}
}

func seq2VFirst(es []entryVFirst[int]) iter.Seq2[int, struct{}] {
	return func(yield func(int, struct{}) bool) {
		for _, e := range es {
			if !yield(e.k, e.v) {
				return
			}
		}
	}
}

// keysOf is what SortedSet.All would be if it delegated to SortedMap.All.
func keysOf(s iter.Seq2[int, struct{}]) iter.Seq[int] {
	return func(yield func(int) bool) {
		for k := range s {
			if !yield(k) {
				return
			}
		}
	}
}

func BenchmarkIterate(b *testing.B) {
	for _, n := range sizes {
		bare := make([]int, n)
		vlast := make([]entryVLast[int], n)
		vfirst := make([]entryVFirst[int], n)
		for i := range n {
			bare[i], vlast[i].k, vfirst[i].k = i, i, i
		}
		b.Run(fmt.Sprintf("bare/%d", n), func(b *testing.B) {
			for b.Loop() {
				t := 0
				for k := range seqBare(bare) {
					t += k
				}
				sinkInt = t
			}
		})
		b.Run(fmt.Sprintf("vlast-raw/%d", n), func(b *testing.B) {
			for b.Loop() {
				t := 0
				for _, e := range vlast {
					t += e.k
				}
				sinkInt = t
			}
		})
		b.Run(fmt.Sprintf("vfirst-raw/%d", n), func(b *testing.B) {
			for b.Loop() {
				t := 0
				for _, e := range vfirst {
					t += e.k
				}
				sinkInt = t
			}
		})
		// Delegation: Seq2 wrapped in an adapter that discards the value.
		b.Run(fmt.Sprintf("vfirst-via-seq2/%d", n), func(b *testing.B) {
			for b.Loop() {
				t := 0
				for k := range keysOf(seq2VFirst(vfirst)) {
					t += k
				}
				sinkInt = t
			}
		})
	}
}

// ---- zero-cost baseline ---------------------------------------------------

func BenchmarkNoOp(b *testing.B) {
	bare := make([]int, 64)
	for b.Loop() {
		sinkBare = bare
	}
}

func TestSizes(t *testing.T) {
	t.Logf("bare int:              %2d bytes", unsafe.Sizeof(int(0)))
	t.Logf("entryVLast[int]:       %2d bytes", unsafe.Sizeof(entryVLast[int]{}))
	t.Logf("entryVFirst[int]:      %2d bytes", unsafe.Sizeof(entryVFirst[int]{}))
	t.Logf("entryVLast[string]:    %2d bytes", unsafe.Sizeof(entryVLast[string]{}))
	t.Logf("entryVFirst[string]:   %2d bytes", unsafe.Sizeof(entryVFirst[string]{}))
}
