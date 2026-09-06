package copycost

import (
	"fmt"
	"maps"
	"slices"
	"testing"
)

var sizes = []int{0, 1, 8, 64, 512, 4096, 65536, 1 << 20}

var smallSizes = []int{8, 512, 4096, 65536}

// BenchmarkCloneInt measures the cost of cloning a pointer-free 8-byte element
// type across four orders of magnitude. The shape to look for: a fixed
// allocation floor at small n, then memory bandwidth at large n.
func BenchmarkCloneInt(b *testing.B) {
	for _, n := range sizes {
		src := make([]Small, n)
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(n * 8))
			for b.Loop() {
				sinkInt = slices.Clone(src)
			}
		})
	}
}

// BenchmarkCloneBig64B holds element count fixed against BenchmarkCloneInt and
// varies element width, isolating bandwidth from per-element overhead.
func BenchmarkCloneBig64B(b *testing.B) {
	for _, n := range smallSizes {
		src := make([]Big, n)
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(n * 64))
			for b.Loop() {
				sinkBig = slices.Clone(src)
			}
		})
	}
}

// BenchmarkClonePtrful clones a pointer-bearing element type. The copy itself
// is unremarkable; the cost this type carries shows up in BenchmarkGCCycle.
func BenchmarkClonePtrful(b *testing.B) {
	for _, n := range smallSizes {
		src := makePtrful(n)
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkPtrful = slices.Clone(src)
			}
		})
	}
}

// BenchmarkCloneMap is the counterpart to BenchmarkCloneInt at matching entry
// counts. There is no memmove available to a map clone: it allocates buckets
// and rehashes, which puts it in a different cost class from a slice clone.
func BenchmarkCloneMap(b *testing.B) {
	for _, n := range smallSizes {
		src := make(map[int]int, n)
		for i := range n {
			src[i] = i
		}
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkMap = maps.Clone(src)
			}
		})
	}
}

// BenchmarkNoCopy is the zero-copy baseline: hand back the slice header. It
// should report 0 allocs and near-zero time; any drift here means measurement
// overhead has crept into the other benchmarks.
func BenchmarkNoCopy(b *testing.B) {
	src := make([]Small, 4096)
	b.ReportAllocs()
	for b.Loop() {
		sinkInt = src
	}
}
