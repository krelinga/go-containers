package witness

import (
	"iter"
	"testing"
)

// Does the range-shaped hatch need its own method, or can it come from ADR
// 0019's spans?
//
//	m.Range(lo, hi).EachKey(f)     // span carries the hatch -- no new name
//	m.EachKeyInRange(lo, hi, f)    // a direct method -- a new name per shape
//
// If spans merge with views (ADR 0020's favoured shape) then Range returns an
// INTERFACE, so the span value is boxed. A span is a container plus two bounds,
// which is wider than one word, so boxing it should allocate -- and that
// allocation lands on the hatch the span was supposed to supply for free.

type span struct {
	m      map[string]struct{}
	lo, hi string
}

func (s span) EachKey(f func(string) bool) {
	for k := range s.m {
		if k >= s.lo && k < s.hi && !f(k) {
			return
		}
	}
}

type spanIface interface {
	EachKey(func(string) bool)
	Keys() iter.Seq[string]
}

func (s span) Keys() iter.Seq[string] {
	m, lo, hi := s.m, s.lo, s.hi
	return func(yield func(string) bool) {
		for k := range m {
			if k >= lo && k < hi && !yield(k) {
				return
			}
		}
	}
}

func (s spanPtr) Keys() iter.Seq[string] { return s.st.Keys() }

type rangeable struct{ m map[string]struct{} }

// Range through an interface: the shape a merged span/view has.
func (r rangeable) Range(lo, hi string) spanIface { return span{r.m, lo, hi} }

// The direct method: no span value, nothing to box.
func (r rangeable) EachKeyInRange(lo, hi string, f func(string) bool) {
	for k := range r.m {
		if k >= lo && k < hi && !f(k) {
			return
		}
	}
}

// A one-word span, for contrast: state behind a pointer boxes free.
type spanPtr struct{ st *span }

func (s spanPtr) EachKey(f func(string) bool) { s.st.EachKey(f) }

func (r rangeable) RangePtr(lo, hi string) spanIface {
	return spanPtr{&span{r.m, lo, hi}} // the pointer itself must be allocated
}

// The realistic case: Range is itself called on a view INTERFACE, so the
// compiler cannot see which Range runs and cannot prove the returned span
// does not escape.
type rangeableIface interface {
	Range(lo, hi string) spanIface
	EachKeyInRange(lo, hi string, f func(string) bool)
}

// A second implementation and a runtime-selected branch, so the compiler cannot
// devirtualize the interface. Without this every measurement below reads 0 --
// with one visible implementation assigned locally, the call is not really
// dynamic at all. That mistake is easy to make and it flatters every design.
type otherRangeable struct{ m map[string]struct{} }

func (o otherRangeable) Range(lo, hi string) spanIface { return span{o.m, lo, hi} }
func (o otherRangeable) EachKeyInRange(lo, hi string, f func(string) bool) {
	for k := range o.m {
		if k >= lo && k < hi && !f(k) {
			return
		}
	}
}

// set at init from an environment-independent but opaque source
var useOther = len(sinkStr) > 1<<30

func opaqueRangeable(r rangeable) rangeableIface {
	if useOther {
		return otherRangeable{r.m}
	}
	return r
}

func BenchmarkRangeHatchViaSpan(b *testing.B) {
	s, _ := eachFixture(64)
	m := make(map[string]struct{}, 64)
	for k := range s.m {
		m[k.Name] = struct{}{}
	}
	r := rangeable{m}
	f := func(string) bool { sinkInt++; return true }

	b.Run("span-boxed/Range().EachKey", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			r.Range("k1", "k5").EachKey(f)
		}
	})
	b.Run("span-behind-pointer/Range().EachKey", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			r.RangePtr("k1", "k5").EachKey(f)
		}
	})
	b.Run("direct/EachKeyInRange", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			r.EachKeyInRange("k1", "k5", f)
		}
	})

	ri := opaqueRangeable(r)
	b.Run("iface/Range().EachKey", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			ri.Range("k1", "k5").EachKey(f)
		}
	})
	b.Run("iface/EachKeyInRange", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			ri.EachKeyInRange("k1", "k5", f)
		}
	})
	b.Run("iface/Range()+range-over-func", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for range ri.Range("k1", "k5").Keys() {
				sinkInt++
			}
		}
	})
}

func TestRangeHatchAllocs(t *testing.T) {
	s, _ := eachFixture(64)
	m := make(map[string]struct{}, 64)
	for k := range s.m {
		m[k.Name] = struct{}{}
	}
	r := rangeable{m}
	f := func(string) bool { sinkInt++; return true }

	t.Logf("span boxed into iface:   %.1f allocs/op", testing.AllocsPerRun(200, func() {
		r.Range("k1", "k5").EachKey(f)
	}))
	t.Logf("span behind a pointer:   %.1f allocs/op", testing.AllocsPerRun(200, func() {
		r.RangePtr("k1", "k5").EachKey(f)
	}))
	t.Logf("direct EachKeyInRange:   %.1f allocs/op", testing.AllocsPerRun(200, func() {
		r.EachKeyInRange("k1", "k5", f)
	}))

	ri := opaqueRangeable(r)
	t.Logf("IFACE Range().EachKey:    %.1f allocs/op", testing.AllocsPerRun(200, func() {
		ri.Range("k1", "k5").EachKey(f)
	}))
	t.Logf("IFACE EachKeyInRange:     %.1f allocs/op", testing.AllocsPerRun(200, func() {
		ri.EachKeyInRange("k1", "k5", f)
	}))
	t.Logf("IFACE Range()+range-func: %.1f allocs/op", testing.AllocsPerRun(200, func() {
		for range ri.Range("k1", "k5").Keys() {
			sinkInt++
		}
	}))
}
