package iteration

import (
	"iter"
	"testing"
)

// How much is a size hint worth?
//
// Proposal A in ADR 0017 builds a whole primitive -- a sealed Collector with
// SizeHint, a CanLen assertion, and two Sized constructors -- whose only job is
// to move a length from a source to a consumer. Proposals B and C give that up
// to buy other things. ADR 0006 says a missing hint costs an allocation and
// never a wrong result, which says nothing about how much the allocation costs.
//
// These benchmarks price it directly: the same build, with and without the
// capacity the hint would have supplied.

func counting(n int) iter.Seq[int] {
	return func(y func(int) bool) {
		for i := range n {
			if !y(i) {
				return
			}
		}
	}
}

var sinkInts []int
var sinkSet map[int]struct{}

func buildSlice(hint int, s iter.Seq[int]) []int {
	var o []int
	if hint > 0 {
		o = make([]int, 0, hint)
	}
	for v := range s {
		o = append(o, v)
	}
	return o
}

func buildMap(hint int, s iter.Seq[int]) map[int]struct{} {
	o := make(map[int]struct{}, hint)
	for v := range s {
		o[v] = struct{}{}
	}
	return o
}

func presize(b *testing.B, size int) {
	b.Run("slice/unsized", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInts = buildSlice(0, counting(size))
		}
	})
	b.Run("slice/sized", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInts = buildSlice(size, counting(size))
		}
	})
	b.Run("map/unsized", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSet = buildMap(0, counting(size))
		}
	})
	b.Run("map/sized", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSet = buildMap(size, counting(size))
		}
	})
}

func BenchmarkPresize8(b *testing.B)    { presize(b, 8) }
func BenchmarkPresize1024(b *testing.B) { presize(b, 1024) }

// A hint that is wrong rather than absent. ADR 0006 claims this is safe; these
// bound what "safe" costs when the guess is bad in each direction.
func BenchmarkPresizeWrong(b *testing.B) {
	const size = 1024
	b.Run("under/16", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInts = buildSlice(16, counting(size))
		}
	})
	b.Run("over/16x", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInts = buildSlice(size*16, counting(size))
		}
	})
}
