package witness

import (
	"fmt"
	"testing"
)

// At what size does the fixed per-call overhead of iterating through a view
// interface stop mattering?
func BenchmarkScale(b *testing.B) {
	for _, size := range []int{0, 1, 2, 4, 8, 16, 32, 64, 128, 256, 1024, 4096, 16384} {
		items := make([]*item, size)
		for i := range items {
			items[i] = &item{Name: fmt.Sprint(i)}
		}
		ps := newParamSet[*item, string, statelessViewer](items...)
		pv := ps.View()

		b.Run(fmt.Sprintf("n=%05d/concrete", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				c := 0
				for range ps.Keys() {
					c++
				}
				sinkInt = c
			}
		})
		b.Run(fmt.Sprintf("n=%05d/view-iface", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				c := 0
				for range pv.Keys() {
					c++
				}
				sinkInt = c
			}
		})
	}
}
