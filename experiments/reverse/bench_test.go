package reverse

import "testing"

const n = 1024

var (
	sinkInt int
	sinkSet setView[int]
	sinkSpn span[int]
)

func fixture() sortedSet[int] {
	es := make([]int, n)
	for i := range es {
		es[i] = i
	}
	return newSortedSet(es)
}

// Baseline: what the forward walk costs, so the reverse numbers have a floor.
func BenchmarkBaseline(b *testing.B) {
	s := fixture()
	b.Run("noop", func(b *testing.B) {
		for b.Loop() {
			sinkInt++
		}
	})
	b.Run("A/forward-full", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.A_Keys() {
				c++
			}
			sinkInt = c
		}
	})
}

// 1. Full-container reverse, three shapes plus the one that must not ship.
func BenchmarkReverseFull(b *testing.B) {
	s := fixture()

	b.Run("A/method-pair", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.A_KeysBackward() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("B/view.Backward", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.B_View().Backward().Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("C/span.Backward", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.C_Span().Backward().Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("naive/materialise", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.Naive_Backward() {
				c++
			}
			sinkInt = c
		}
	})
}

// 2. Sub-range, forward. A 64-element window out of 1024 -- the case where
// materialising looks cheapest and is still the wrong shape.
func BenchmarkSubRangeForward(b *testing.B) {
	s := fixture()
	const lo, hi = 500, 564

	b.Run("A/method-pair", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.A_RangeKeys(lo, hi) {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("B/view", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.B_Range(lo, hi).Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("C/span", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.C_Range(lo, hi).Keys() {
				c++
			}
			sinkInt = c
		}
	})
}

// 3. Sub-range, backward -- the composition that is the whole point.
func BenchmarkSubRangeBackward(b *testing.B) {
	s := fixture()
	const lo, hi = 500, 564

	b.Run("A/method-pair", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.A_RangeKeysBackward(lo, hi) {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("B/view.Backward", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.B_Range(lo, hi).Backward().Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("C/span.Backward", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.C_Range(lo, hi).Backward().Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("naive/materialise", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.Naive_RangeBackward(lo, hi) {
				c++
			}
			sinkInt = c
		}
	})
}

// 4. Constructing the intermediate value, with no walk at all. This is where a
// composable shape either is or is not free.
func BenchmarkConstructOnly(b *testing.B) {
	s := fixture()
	const lo, hi = 500, 564

	b.Run("B/view", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSet = s.B_Range(lo, hi)
		}
	})
	b.Run("B/view.Backward", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSet = s.B_Range(lo, hi).Backward()
		}
	})
	b.Run("C/span", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSpn = s.C_Range(lo, hi)
		}
	})
	b.Run("C/span.Backward", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSpn = s.C_Range(lo, hi).Backward()
		}
	})
}
