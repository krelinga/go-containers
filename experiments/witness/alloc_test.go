package witness

import (
	"iter"
	"testing"
)

var sinkSeq iter.Seq[string]

// Split the three allocations: obtaining the iterator, versus ranging it.
func TestAllocAttribution(t *testing.T) {
	s, _ := fixture()
	items := make([]*item, 0, n)
	for x := range s.Keys() {
		items = append(items, x)
	}
	ps := newParamSet[*item, string, statelessViewer](items...)
	pv := ps.View() // the view interface

	t.Logf("A. obtain the iterator ONLY, through the interface: %v allocs",
		testing.AllocsPerRun(200, func() { sinkSeq = pv.Keys() }))

	t.Logf("B. obtain it CONCRETELY (no interface):             %v allocs",
		testing.AllocsPerRun(200, func() { sinkSeq = ps.Keys() }))

	t.Logf("C. obtain + range, through the interface:           %v allocs",
		testing.AllocsPerRun(200, func() {
			c := 0
			for range pv.Keys() {
				c++
			}
			sinkInt = c
		}))

	t.Logf("D. obtain + range, concretely:                      %v allocs",
		testing.AllocsPerRun(200, func() {
			c := 0
			for range ps.Keys() {
				c++
			}
			sinkInt = c
		}))

	// Range a PRE-OBTAINED iterator: the call is out of the picture, so what
	// remains is purely the range machinery over an opaque func value.
	seq := pv.Keys()
	t.Logf("E. range a pre-obtained iterator:                   %v allocs",
		testing.AllocsPerRun(200, func() {
			c := 0
			for range seq {
				c++
			}
			sinkInt = c
		}))
}
