package witness

import (
	"fmt"
	"testing"
)

// Where exactly does a callback have to live to cost nothing?
//
// The literal-at-the-call-site form allocates twice when it crosses a dynamic
// call: the closure, and the local it captures. Neither is the container's
// allocation -- both belong to the caller -- so the question is what the caller
// has to do about it, and how far the callback has to move.

const hoistN = 64

// --- accumulator shapes a caller might use ---

type counter struct{ n int }

func (c *counter) addString(string) bool { c.n++; return true }

var globalCounter counter

func globalAdd(string) bool { globalCounter.n++; return true }

func BenchmarkHoisting(b *testing.B) {
	s, sv := eachFixture(hoistN)
	iv := viewEach(s, sv)

	// A. The naive form: a literal written at the call site, capturing a local.
	b.Run("A/literal-at-callsite", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			n := 0
			iv.Each(func(string) bool { n++; return true })
			sinkInt = n
		}
	})

	// B. The SAME closure, capturing the SAME local, moved just outside the
	// hot loop. The capture is still there; it happens once.
	b.Run("B/closure-hoisted-one-level", func(b *testing.B) {
		n := 0
		f := func(string) bool { n++; return true }
		b.ReportAllocs()
		for b.Loop() {
			n = 0
			iv.Each(f)
			sinkInt = n
		}
	})

	// C. A pointer to a local accumulator, captured once outside the loop.
	// Lets the callback stay a closure while the state stays addressable.
	b.Run("C/closure-over-pointer", func(b *testing.B) {
		var acc counter
		f := func(string) bool { acc.n++; return true }
		b.ReportAllocs()
		for b.Loop() {
			acc.n = 0
			iv.Each(f)
			sinkInt = acc.n
		}
	})

	// D. A METHOD VALUE on a pointer receiver, taken at the call site. A method
	// value is itself a closure over the receiver -- does taking it allocate?
	b.Run("D/method-value-at-callsite", func(b *testing.B) {
		var acc counter
		b.ReportAllocs()
		for b.Loop() {
			acc.n = 0
			iv.Each(acc.addString)
			sinkInt = acc.n
		}
	})

	// E. The same method value, bound once outside the loop.
	b.Run("E/method-value-hoisted", func(b *testing.B) {
		var acc counter
		f := acc.addString
		b.ReportAllocs()
		for b.Loop() {
			acc.n = 0
			iv.Each(f)
			sinkInt = acc.n
		}
	})

	// F. Package-level func over package-level state: "all the way up".
	b.Run("F/package-level-func", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			globalCounter.n = 0
			iv.Each(globalAdd)
			sinkInt = globalCounter.n
		}
	})

	// G. Baseline: range-over-func, for the comparison.
	b.Run("G/range-Keys", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			n := 0
			for range iv.Keys() {
				n++
			}
			sinkInt = n
		}
	})
}

// The trap: hoisting one level is only enough if the loop is RIGHT THERE. When
// the Each call sits inside a helper that is itself called in a hot loop, the
// closure is rebuilt on every call to the helper, and hoisting has to go up to
// wherever the loop actually is.
func countVia(v eachView[string]) int {
	n := 0
	v.Each(func(string) bool { n++; return true }) // rebuilt per call
	return n
}

func countViaParam(v eachView[string], f func(string) bool) { v.Each(f) }

func BenchmarkHoistingAcrossAFunction(b *testing.B) {
	s, sv := eachFixture(hoistN)
	iv := viewEach(s, sv)

	b.Run("helper-builds-the-closure", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = countVia(iv)
		}
	})
	b.Run("caller-owns-the-closure", func(b *testing.B) {
		n := 0
		f := func(string) bool { n++; return true }
		b.ReportAllocs()
		for b.Loop() {
			n = 0
			countViaParam(iv, f)
			sinkInt = n
		}
	})
}

// On a CONCRETE container the literal is already free: escape analysis can see
// set.Each and prove the closure does not outlive the call. Only the dynamic
// call forces it to the heap. Worth knowing which situation you are in.
func BenchmarkHoistingConcreteVsDynamic(b *testing.B) {
	s, _ := eachFixture(hoistN)

	b.Run("concrete/literal-at-callsite", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			n := 0
			s.Each(func(*item) bool { n++; return true })
			sinkInt = n
		}
	})
}

// How much the caller's own closure costs, across sizes: it is a FIXED cost,
// so it is a rounding error on a big container and the whole bill on a small
// one -- the same shape as every other per-call cost in this harness.
func BenchmarkHoistingBySize(b *testing.B) {
	for _, sz := range []int{0, 1, 8, 64, 1024} {
		s, sv := eachFixture(sz)
		iv := viewEach(s, sv)
		b.Run(fmt.Sprintf("n=%d", sz), func(b *testing.B) {
			b.Run("literal-at-callsite", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					n := 0
					iv.Each(func(string) bool { n++; return true })
					sinkInt = n
				}
			})
			b.Run("hoisted-one-level", func(b *testing.B) {
				n := 0
				f := func(string) bool { n++; return true }
				b.ReportAllocs()
				for b.Loop() {
					n = 0
					iv.Each(f)
					sinkInt = n
				}
			})
		})
		_ = s
	}
}

func TestHoistingAllocs(t *testing.T) {
	s, sv := eachFixture(hoistN)
	iv := viewEach(s, sv)

	report := func(name string, f func()) {
		t.Logf("%-34s %4.1f allocs/op", name, testing.AllocsPerRun(200, f))
	}
	report("A literal-at-callsite", func() {
		n := 0
		iv.Each(func(string) bool { n++; return true })
		sinkInt = n
	})
	{
		n := 0
		f := func(string) bool { n++; return true }
		report("B closure-hoisted-one-level", func() { n = 0; iv.Each(f); sinkInt = n })
	}
	{
		var acc counter
		f := func(string) bool { acc.n++; return true }
		report("C closure-over-pointer", func() { acc.n = 0; iv.Each(f); sinkInt = acc.n })
	}
	{
		var acc counter
		report("D method-value-at-callsite", func() { acc.n = 0; iv.Each(acc.addString); sinkInt = acc.n })
	}
	{
		var acc counter
		f := acc.addString
		report("E method-value-hoisted", func() { acc.n = 0; iv.Each(f); sinkInt = acc.n })
	}
	report("F package-level-func", func() { globalCounter.n = 0; iv.Each(globalAdd); sinkInt = globalCounter.n })
	report("helper-builds-the-closure", func() { sinkInt = countVia(iv) })
	report("concrete/literal-at-callsite", func() {
		n := 0
		s.Each(func(*item) bool { n++; return true })
		sinkInt = n
	})
}
