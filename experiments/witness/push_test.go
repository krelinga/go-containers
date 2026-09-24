package witness

import (
	"fmt"
	"iter"
	"testing"
)

// iter.Seq[T] IS func(yield func(T) bool), so a caller can invoke the sequence
// directly instead of ranging it:
//
//	v.Keys()(f)     // push, by hand
//	v.Each(f)       // the proposed hatch
//
// Those have the same shape. If the by-hand form already avoids the range
// machinery, the hatch buys only the iterator closure and is much harder to
// justify. This is the adversarial check on the whole proposal.
func BenchmarkPushForms(b *testing.B) {
	for _, sz := range []int{0, 8, 64, 1024} {
		s, sv := eachFixture(sz)
		iv := viewEach(s, sv)

		b.Run(fmt.Sprintf("n=%d", sz), func(b *testing.B) {
			b.Run("range-Keys", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					n := 0
					for range iv.Keys() {
						n++
					}
					sinkInt = n
				}
			})
			// Call the Seq directly. No range statement, so none of the range
			// machinery -- but Keys() still returns a closure through a
			// dynamic call.
			b.Run("Keys()(f)/hoisted-f", func(b *testing.B) {
				n := 0
				f := func(string) bool { n++; return true }
				b.ReportAllocs()
				for b.Loop() {
					n = 0
					iv.Keys()(f)
					sinkInt = n
				}
			})
			b.Run("Keys()(f)/literal", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					n := 0
					iv.Keys()(func(string) bool { n++; return true })
					sinkInt = n
				}
			})
			// iter.Pull: the stdlib's own escape from push to pull.
			b.Run("iter.Pull", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					n := 0
					next, stop := iter.Pull(iv.Keys())
					for {
						_, ok := next()
						if !ok {
							break
						}
						n++
					}
					stop()
					sinkInt = n
				}
			})
			b.Run("Each/hoisted-f", func(b *testing.B) {
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
	}
}

// Early exit, where the fixed cost is the whole cost.
func BenchmarkPushFormsEarlyExit(b *testing.B) {
	s, sv := eachFixture(1024)
	iv := viewEach(s, sv)
	stopAt1 := func(k string) bool { sinkStr = k; return false }

	b.Run("range-Keys+break", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for k := range iv.Keys() {
				sinkStr = k
				break
			}
		}
	})
	b.Run("Keys()(f)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			iv.Keys()(stopAt1)
		}
	})
	b.Run("Each", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			iv.Each(stopAt1)
		}
	})
}

func TestPushFormAllocs(t *testing.T) {
	s, sv := eachFixture(64)
	iv := viewEach(s, sv)
	n := 0
	f := func(string) bool { n++; return true }

	report := func(name string, fn func()) {
		t.Logf("%-28s %4.1f allocs/op", name, testing.AllocsPerRun(200, fn))
	}
	report("range Keys()", func() {
		n = 0
		for range iv.Keys() {
			n++
		}
		sinkInt = n
	})
	report("Keys()(f)", func() { n = 0; iv.Keys()(f); sinkInt = n })
	report("iter.Pull(Keys())", func() {
		next, stop := iter.Pull(iv.Keys())
		c := 0
		for {
			if _, ok := next(); !ok {
				break
			}
			c++
		}
		stop()
		sinkInt = c
	})
	report("Each(f)", func() { n = 0; iv.Each(f); sinkInt = n })
	_ = s
}
