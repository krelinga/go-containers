package sealing

import (
	"fmt"
	"testing"
	"unsafe"
)

const n = 64

var (
	sinkInt int
	sinkRK  ReadKeys[string]
	sinkSV  StructView[string]
	sinkIV  IfaceView[string]
	sinkPV  PtrView[string]
)

func fixture() (MapSet[*Item], stateful) {
	items := make([]*Item, n)
	known := make(map[string]*Item, n)
	for i := range items {
		items[i] = &Item{Name: fmt.Sprintf("k%d", i)}
		known[items[i].Name] = items[i]
	}
	return NewMapSet(items...), stateful{known: known}
}

//go:noinline
func consume[NT any](k ReadKeys[NT]) int { sinkRK = any(k).(ReadKeys[string]); return k.Len() }

func TestWidthsAndZero(t *testing.T) {
	s, vw := fixture()
	t.Logf("StructView (2-word): %d B", unsafe.Sizeof(NewStructView(s, vw)))
	t.Logf("IfaceView:           %d B", unsafe.Sizeof(NewIfaceView(s, vw)))
	t.Logf("PtrView (1-word):    %d B", unsafe.Sizeof(NewPtrView(s, vw)))

	var zs StructView[string]
	var zp PtrView[string]
	t.Logf("zero StructView.Len() = %d (no panic)", zs.Len())
	t.Logf("zero PtrView.Len()    = %d (no panic)", zp.Len())
	var zi IfaceView[string]
	func() {
		defer func() { t.Logf("zero IfaceView.Len() PANICS: %v", recover()) }()
		_ = zi.Len()
		t.Error("expected panic")
	}()
}

func TestAllocs(t *testing.T) {
	s, vw := fixture()
	sv, iv, pv := NewStructView(s, vw), NewIfaceView(s, vw), NewPtrView(s, vw)

	r := func(name string, f func()) { t.Logf("%-38s %.1f allocs/op", name, testing.AllocsPerRun(300, f)) }
	r("construct StructView (escaping)", func() { sinkSV = NewStructView(s, vw) })
	r("construct IfaceView  (escaping)", func() { sinkIV = NewIfaceView(s, vw) })
	r("construct PtrView    (escaping)", func() { sinkPV = NewPtrView(s, vw) })
	r("StructView -> ReadKeys param", func() { sinkInt = consume[string](sv) })
	r("IfaceView  -> ReadKeys param", func() { sinkInt = consume[string](iv) })
	r("PtrView    -> ReadKeys param", func() { sinkInt = consume[string](pv) })
	r("StructView.Len()", func() { sinkInt = sv.Len() })
	r("IfaceView.Len()", func() { sinkInt = iv.Len() })
	r("PtrView.Len()", func() { sinkInt = pv.Len() })
	r("StructView iterate", func() {
		c := 0
		for range sv.Keys() {
			c++
		}
		sinkInt = c
	})
	r("IfaceView  iterate", func() {
		c := 0
		for range iv.Keys() {
			c++
		}
		sinkInt = c
	})
	r("PtrView    iterate", func() {
		c := 0
		for range pv.Keys() {
			c++
		}
		sinkInt = c
	})
}

func BenchmarkAll(b *testing.B) {
	s, vw := fixture()
	sv, iv, pv := NewStructView(s, vw), NewIfaceView(s, vw), NewPtrView(s, vw)

	b.Run("point-read/StructView", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = sv.Len()
		}
	})
	b.Run("point-read/IfaceView", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = iv.Len()
		}
	})
	b.Run("point-read/PtrView", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = pv.Len()
		}
	})

	b.Run("iterate/StructView", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range sv.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("iterate/IfaceView", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range iv.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("iterate/PtrView", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range pv.Keys() {
				c++
			}
			sinkInt = c
		}
	})

	b.Run("boundary/StructView", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = consume[string](sv)
		}
	})
	b.Run("boundary/IfaceView", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = consume[string](iv)
		}
	})
	b.Run("boundary/PtrView", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = consume[string](pv)
		}
	})

	b.Run("construct/StructView", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSV = NewStructView(s, vw)
		}
	})
	b.Run("construct/IfaceView", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkIV = NewIfaceView(s, vw)
		}
	})
	b.Run("construct/PtrView", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkPV = NewPtrView(s, vw)
		}
	})
}

// The case I overlooked: a read-only API that takes the CONCRETE view type
// rather than a shape interface. There is no interface to box into, so the
// struct wrapper costs nothing at all.
//
//go:noinline
func consumeConcrete(v StructView[string]) int { sinkSV = v; return v.Len() }

func TestConcreteParameter(t *testing.T) {
	s, vw := fixture()
	sv := NewStructView(s, vw)
	t.Logf("StructView -> CONCRETE param:  %.1f allocs/op",
		testing.AllocsPerRun(300, func() { sinkInt = consumeConcrete(sv) }))
	t.Logf("StructView -> ReadKeys param:  %.1f allocs/op",
		testing.AllocsPerRun(300, func() { sinkInt = consume[string](sv) }))
}

func BenchmarkConcreteParameter(b *testing.B) {
	s, vw := fixture()
	sv := NewStructView(s, vw)
	b.Run("concrete-param", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = consumeConcrete(sv)
		}
	})
	b.Run("shape-iface-param", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = consume[string](sv)
		}
	})
}

var sinkSVI StructView[*Item]

func TestIdentityViewIsFree(t *testing.T) {
	s, _ := fixture()
	t.Logf("s.View() escaping (struct view, identity impl): %.1f allocs/op",
		testing.AllocsPerRun(300, func() { sinkSVI = s.View() }))
	v := s.View()
	t.Logf("  Len=%d", v.Len())
	var z StructView[*Item]
	t.Logf("  zero value still reads empty: Len=%d", z.Len())
}

func BenchmarkIdentityView(b *testing.B) {
	s, _ := fixture()
	b.ReportAllocs()
	for b.Loop() {
		sinkSVI = s.View()
	}
}
