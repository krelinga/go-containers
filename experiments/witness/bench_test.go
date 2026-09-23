package witness

import (
	"testing"
	"unsafe"
)

const n = 64

var (
	sinkInt  int
	sinkKeys Keys[string]
)

func fixture() (set[*item], statefulViewer) {
	items := make([]*item, n)
	known := make(map[string]*item, n)
	for i := range items {
		items[i] = &item{Name: string(rune('a'+i%26)) + string(rune('a'+i/26))}
		known[items[i].Name] = items[i]
	}
	return newSet(items...), statefulViewer{known: known}
}

func TestWidths(t *testing.T) {
	s, sv := fixture()
	t.Logf("iface view:            %d B", unsafe.Sizeof(viewIface[*item, string](s, sv)))
	t.Logf("pure witness:          %d B", unsafe.Sizeof(viewPure[*item, string, statelessViewer](s)))
	t.Logf("carried, stateless vw: %d B", unsafe.Sizeof(viewCarried[*item, string](s, statelessViewer{})))
	t.Logf("carried, stateful vw:  %d B", unsafe.Sizeof(viewCarried[*item, string](s, sv)))
}

// THE question: does a witness fix ADR 0013's erratum -- three allocations per
// iteration through a view?
func BenchmarkIterate(b *testing.B) {
	s, sv := fixture()
	iv := viewIface[*item, string](s, sv)
	pw := viewPure[*item, string, statelessViewer](s)
	cwStateless := viewCarried[*item, string](s, statelessViewer{})
	cwStateful := viewCarried[*item, string](s, sv)

	b.Run("container(no view)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("view/interface(today)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range iv.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("view/pure-witness", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range pw.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("view/carried-witness(stateless)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range cwStateless.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("view/carried-witness(stateful)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range cwStateful.Keys() {
				c++
			}
			sinkInt = c
		}
	})
}

func BenchmarkPointRead(b *testing.B) {
	s, sv := fixture()
	iv := viewIface[*item, string](s, sv)
	pw := viewPure[*item, string, statelessViewer](s)
	cw := viewCarried[*item, string](s, sv)

	b.Run("view/interface(today)", func(b *testing.B) {
		for b.Loop() {
			sinkInt = iv.Len()
		}
	})
	b.Run("view/pure-witness", func(b *testing.B) {
		for b.Loop() {
			sinkInt = pw.Len()
		}
	})
	b.Run("view/carried-witness", func(b *testing.B) {
		for b.Loop() {
			sinkInt = cw.Len()
		}
	})
}

// Boxing into a shape interface -- the cost that made today's 2-word view
// allocate at every generic boundary.
func BenchmarkBoxIntoShapeIface(b *testing.B) {
	s, sv := fixture()
	iv := viewIface[*item, string](s, sv)
	pw := viewPure[*item, string, statelessViewer](s)
	cwStateless := viewCarried[*item, string](s, statelessViewer{})
	cwStateful := viewCarried[*item, string](s, sv)

	b.Run("interface(today)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkKeys = iv
		}
	})
	b.Run("pure-witness", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkKeys = pw
		}
	})
	b.Run("carried(stateless)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkKeys = cwStateless
		}
	})
	b.Run("carried(stateful)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkKeys = cwStateful
		}
	})
}

func TestFieldOrder(t *testing.T) {
	s, sv := fixture()
	t.Logf("carried, viewer LAST,  stateless: %d B", unsafe.Sizeof(viewCarried[*item, string](s, statelessViewer{})))
	t.Logf("carried, viewer FIRST, stateless: %d B", unsafe.Sizeof(viewCarriedFront[*item, string](s, statelessViewer{})))
	t.Logf("carried, viewer FIRST, stateful:  %d B", unsafe.Sizeof(viewCarriedFront[*item, string](s, sv)))
}

func BenchmarkFieldOrderBoxing(b *testing.B) {
	s, sv := fixture()
	last := viewCarried[*item, string](s, statelessViewer{})
	front := viewCarriedFront[*item, string](s, statelessViewer{})
	frontStateful := viewCarriedFront[*item, string](s, sv)

	b.Run("viewer-last/stateless", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkKeys = last
		}
	})
	b.Run("viewer-first/stateless", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkKeys = front
		}
	})
	b.Run("viewer-first/stateful", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkKeys = frontStateful
		}
	})
}

// BenchmarkNoOp is the zero-cost baseline: it is how harness overhead leaking
// into the numbers above would show up.
func BenchmarkNoOp(b *testing.B) {
	for b.Loop() {
		sinkInt++
	}
}
