package copycost

import (
	"runtime"
	"testing"
)

// BenchmarkGCCycle measures the cost of one forced GC cycle while a fixed
// number of copied elements stays reachable. ns/op is therefore the price of a
// single collection, not of the copy.
//
// This is the cost a benchmark of slices.Clone cannot see: a retained copy is
// not charged once, it is charged on every cycle for as long as it is held —
// and only if its elements contain pointers. Spans with no pointers are not
// scanned at all, so pointer-free copies should be indistinguishable from
// retaining nothing.
func BenchmarkGCCycle(b *testing.B) {
	const n = 200_000
	const chunks = 100

	cases := []struct {
		name  string
		setup func() any
	}{
		{"retained=none", func() any { return nil }},
		{"retained=ptrfree", func() any {
			out := make([][]int64, chunks)
			for i := range out {
				out[i] = make([]int64, n/chunks)
			}
			return out
		}},
		{"retained=ptrful", func() any {
			out := make([][]Ptrful, chunks)
			for i := range out {
				out[i] = makePtrful(n / chunks)
			}
			return out
		}},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			retained := tc.setup()
			runtime.GC() // settle the heap before measuring
			for b.Loop() {
				runtime.GC()
			}
			b.StopTimer()
			runtime.KeepAlive(retained)
			sinkAny = retained
		})
	}
}
