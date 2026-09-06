package sortedsetbacking

import (
	"cmp"
	"fmt"
	"slices"
	"testing"
)

// Same bare []int, but searched through a comparator closure instead of
// slices.BinarySearch's specialised path.
func BenchmarkLookupBareViaFunc(b *testing.B) {
	for _, n := range []int{1024, 262144} {
		bare := make([]int, n)
		for i := range n {
			bare[i] = i
		}
		probe := n / 2
		b.Run(fmt.Sprintf("direct/%d", n), func(b *testing.B) {
			for b.Loop() {
				sinkInt, sinkBool = slices.BinarySearch(bare, probe)
			}
		})
		b.Run(fmt.Sprintf("viafunc/%d", n), func(b *testing.B) {
			for b.Loop() {
				sinkInt, sinkBool = slices.BinarySearchFunc(bare, probe, func(a, b int) int { return cmp.Compare(a, b) })
			}
		})
	}
}
