package copycost

import (
	"fmt"
	"slices"
	"testing"
)

type Store struct{ data []int }

// Defensive returns a copy, so callers cannot mutate the Store's backing array.
func (s *Store) Defensive() []int { return slices.Clone(s.data) }

// View returns the slice header directly: O(1), but callers can mutate through it.
func (s *Store) View() []int { return s.data }

// BenchmarkAccessorInLoop is the failure mode that matters in practice. A
// caller indexing an accessor inside a loop is idiomatic and looks like O(1)
// indexing at the call site, but a copying accessor makes it O(n^2).
//
// The point is not that a single copy is expensive — it is that a copying
// accessor hides an O(n) cost behind O(1)-looking syntax.
func BenchmarkAccessorInLoop(b *testing.B) {
	for _, n := range []int{64, 1024, 16384} {
		s := &Store{data: make([]int, n)}

		b.Run(fmt.Sprintf("defensive/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				t := 0
				for i := range n {
					t += s.Defensive()[i]
				}
				sinkScalar = t
			}
		})
		b.Run(fmt.Sprintf("view/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				t := 0
				for i := range n {
					t += s.View()[i]
				}
				sinkScalar = t
			}
		})
	}
}
