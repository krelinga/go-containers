package witness

import (
	"fmt"
	"iter"
	"testing"
)

// The hatch has to beat the alternatives a caller already has, or it is not
// worth an API. Two exist today:
//
//	hoisted  -- call Keys() once OUTSIDE the hot loop and range the same Seq
//	            repeatedly. Costs nothing to add: it is just calling code.
//	slice    -- KeySlice(), then walk a plain slice. One allocation sized by n,
//	            plus a copy of every element.
func BenchmarkEscapeHatchAlternatives(b *testing.B) {
	for _, sz := range []int{0, 8, 64, 1024} {
		s, sv := eachFixture(sz)
		iv := viewEach(s, sv)

		b.Run(fmt.Sprintf("n=%d", sz), func(b *testing.B) {
			b.Run("range-Keys(per-call)", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					c := 0
					for range iv.Keys() {
						c++
					}
					sinkInt = c
				}
			})
			b.Run("range-Keys(hoisted)", func(b *testing.B) {
				var seq iter.Seq[string] = iv.Keys()
				b.ReportAllocs()
				for b.Loop() {
					c := 0
					for range seq {
						c++
					}
					sinkInt = c
				}
			})
			b.Run("KeySlice(per-call)", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					c := 0
					for range iv.KeySlice() {
						c++
					}
					sinkInt = c
				}
			})
			b.Run("Each(non-capturing)", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					iv.Each(countFree)
				}
			})
		})
	}
}
