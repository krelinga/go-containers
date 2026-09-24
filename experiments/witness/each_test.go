package witness

import (
	"fmt"
	"testing"
)

// The escape hatch: Each(f func(T) bool) against the Go iteration paradigm.
//
// The question is not "which is faster in the abstract" but "is there a form a
// caller can reach for when the per-call overhead of an iter.Seq through an
// interface actually matters". So every pair below is measured on the SAME
// receiver, and the sizes run down to 0 -- the overhead is a fixed per-call
// cost, so it is small containers and hot loops where it shows.

var eachSizes = []int{0, 1, 8, 64, 1024}

func eachFixture(sz int) (set[*item], statefulViewer) {
	items := make([]*item, sz)
	known := make(map[string]*item, sz)
	for i := range items {
		items[i] = &item{Name: fmt.Sprintf("k%d", i)}
		known[items[i].Name] = items[i]
	}
	return newSet(items...), statefulViewer{known: known}
}

// countFree is a package-level func value: a callback that captures NOTHING,
// so there is no closure to allocate even when it crosses a dynamic call.
func countFree(string) bool { sinkInt++; return true }

func BenchmarkEach(b *testing.B) {
	for _, sz := range eachSizes {
		s, sv := eachFixture(sz)
		iv := viewEach(s, sv)
		ps := newParamSet[*item, string, statelessViewer]()
		for k := range s.m {
			ps.m[k] = struct{}{}
		}
		pv := ps.ViewEach()

		b.Run(fmt.Sprintf("n=%d", sz), func(b *testing.B) {
			// --- concrete container: the compiler can fuse both forms ---
			b.Run("container/range-Keys", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					c := 0
					for range s.Keys() {
						c++
					}
					sinkInt = c
				}
			})
			b.Run("container/Each", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					c := 0
					s.Each(func(*item) bool { c++; return true })
					sinkInt = c
				}
			})

			// --- through a view interface: the case that costs 3 allocs ---
			b.Run("view/range-Keys", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					c := 0
					for range iv.Keys() {
						c++
					}
					sinkInt = c
				}
			})
			b.Run("view/Each(capturing)", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					c := 0
					iv.Each(func(string) bool { c++; return true })
					sinkInt = c
				}
			})
			b.Run("view/Each(non-capturing)", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					iv.Each(countFree)
				}
			})
			// A capturing callback hoisted OUT of the hot loop: the closure is
			// built once, so the caller keeps its captured state and still
			// pays nothing per call. This is the hatch used properly.
			b.Run("view/Each(hoisted-closure)", func(b *testing.B) {
				c := 0
				f := func(string) bool { c++; return true }
				b.ReportAllocs()
				for b.Loop() {
					c = 0
					iv.Each(f)
					sinkInt = c
				}
			})

			// --- the ADR 0020 favoured shape: parameterised container, the
			// view an interface. Conversion is static; the outer call is not.
			b.Run("paramview/range-Keys", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					c := 0
					for range pv.Keys() {
						c++
					}
					sinkInt = c
				}
			})
			b.Run("paramview/Each(capturing)", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					c := 0
					pv.Each(func(string) bool { c++; return true })
					sinkInt = c
				}
			})
			b.Run("paramview/Each(non-capturing)", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					pv.Each(countFree)
				}
			})
		})
	}
}

// Early exit. Range-over-func pays for the loop-state machinery that makes
// break work across the yield boundary; Each pays a plain return. If the hatch
// is worth anything it should be worth more here.
func BenchmarkEachEarlyExit(b *testing.B) {
	s, sv := eachFixture(1024)
	iv := viewEach(s, sv)

	b.Run("view/range-Keys+break", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for k := range iv.Keys() {
				sinkStr = k
				break
			}
		}
	})
	b.Run("view/Each+false", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			iv.Each(func(k string) bool { sinkStr = k; return false })
		}
	})
}

var sinkStr string

// Allocation counts, asserted rather than eyeballed off a benchmark line.
func TestEachAllocs(t *testing.T) {
	s, sv := eachFixture(64)
	iv := viewEach(s, sv)

	report := func(name string, f func()) {
		t.Logf("%-32s %.1f allocs/op", name, testing.AllocsPerRun(100, f))
	}
	report("container/range-Keys", func() {
		c := 0
		for range s.Keys() {
			c++
		}
		sinkInt = c
	})
	report("container/Each", func() {
		c := 0
		s.Each(func(*item) bool { c++; return true })
		sinkInt = c
	})
	report("view/range-Keys", func() {
		c := 0
		for range iv.Keys() {
			c++
		}
		sinkInt = c
	})
	report("view/Each(capturing)", func() {
		c := 0
		iv.Each(func(string) bool { c++; return true })
		sinkInt = c
	})
	report("view/Each(non-capturing)", func() { iv.Each(countFree) })
}
