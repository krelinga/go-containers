package viewiface

import "testing"

type (
	bareForm  = BareIface[string, ItemView]
	wrapIForm = WrapIface[string, ItemView]
	wrapPForm = WrapPtr[string, ItemView]
)

var (
	sinkBare     bareForm
	sinkWrapI    wrapIForm
	sinkWrapP    wrapPForm
	sinkIterable Iterable[string, ItemView]
)

//go:noinline
func takeBare(v bareForm, k string) ItemView { r, _ := v.Get(k); return r }

//go:noinline
func takeWrapI(v wrapIForm, k string) ItemView { r, _ := v.Get(k); return r }

//go:noinline
func takeWrapP(v wrapPForm, k string) ItemView { r, _ := v.Get(k); return r }

// A boundary declared with a foreign interface -- the Elems2 case in the real
// library, which the sized constructors take.
//
//go:noinline
func takeIterable(v Iterable[string, ItemView]) int { return v.Len() }

func BenchmarkWrapConstruct(b *testing.B) {
	d, vw, _ := fixture()

	b.Run("BareInterface", func(b *testing.B) {
		for b.Loop() {
			sinkBare = ViewBare[*Item, *Item, string, ItemView](d, vw)
		}
	})
	b.Run("StructWrappingIface", func(b *testing.B) {
		for b.Loop() {
			sinkWrapI = ViewWrapIface[*Item, *Item, string, ItemView](d, vw)
		}
	})
	b.Run("StructWrappingPtr", func(b *testing.B) {
		for b.Loop() {
			sinkWrapP = ViewWrapPtr[*Item, *Item, string, ItemView](d, vw)
		}
	})
}

func BenchmarkWrapGet(b *testing.B) {
	d, vw, probe := fixture()
	bare := ViewBare[*Item, *Item, string, ItemView](d, vw)
	wi := ViewWrapIface[*Item, *Item, string, ItemView](d, vw)
	wp := ViewWrapPtr[*Item, *Item, string, ItemView](d, vw)

	b.Run("BareInterface", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = takeBare(bare, probe)
		}
	})
	b.Run("StructWrappingIface", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = takeWrapI(wi, probe)
		}
	})
	b.Run("StructWrappingPtr", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = takeWrapP(wp, probe)
		}
	})
}

// Iteration, where ADR 0013's unmeasured cost lives. Does a wrapper change it?
func BenchmarkWrapAll(b *testing.B) {
	d, vw, _ := fixture()
	bare := ViewBare[*Item, *Item, string, ItemView](d, vw)
	wi := ViewWrapIface[*Item, *Item, string, ItemView](d, vw)
	wp := ViewWrapPtr[*Item, *Item, string, ItemView](d, vw)

	b.Run("BareInterface", func(b *testing.B) {
		for b.Loop() {
			n := 0
			for range bare.All() {
				n++
			}
			sinkInt = n
		}
	})
	b.Run("StructWrappingIface", func(b *testing.B) {
		for b.Loop() {
			n := 0
			for range wi.All() {
				n++
			}
			sinkInt = n
		}
	})
	b.Run("StructWrappingPtr", func(b *testing.B) {
		for b.Loop() {
			n := 0
			for range wp.All() {
				n++
			}
			sinkInt = n
		}
	})
}

// Passing a view onward to a boundary typed with a FOREIGN interface. Today
// that is an interface-to-interface conversion; a wrapper has to box.
func BenchmarkWrapPassToForeignInterface(b *testing.B) {
	d, vw, _ := fixture()
	bare := ViewBare[*Item, *Item, string, ItemView](d, vw)
	wi := ViewWrapIface[*Item, *Item, string, ItemView](d, vw)
	wp := ViewWrapPtr[*Item, *Item, string, ItemView](d, vw)

	b.Run("BareInterface", func(b *testing.B) {
		for b.Loop() {
			sinkInt = takeIterable(bare)
		}
	})
	b.Run("StructWrappingIface", func(b *testing.B) {
		for b.Loop() {
			sinkInt = takeIterable(wi)
		}
	})
	b.Run("StructWrappingPtr", func(b *testing.B) {
		for b.Loop() {
			sinkInt = takeIterable(wp)
		}
	})
}

func BenchmarkReportWrapShapes(b *testing.B) {
	d, vw, _ := fixture()
	wi := ViewWrapIface[*Item, *Item, string, ItemView](d, vw)
	wp := ViewWrapPtr[*Item, *Item, string, ItemView](d, vw)

	var zeroBare bareForm
	var zeroWI wrapIForm
	var zeroWP wrapPForm

	b.Logf("widths: bare interface=%d  struct{iface}=%d  struct{ptr}=%d",
		sizeOf(sinkBare), sizeOf(wi), sizeOf(wp))
	b.Logf("bare interface: nil-comparable=%v", zeroBare == nil)
	b.Logf("struct{iface}:  IsNil=%v  (== nil does not compile)", zeroWI.IsNil())
	b.Logf("struct{ptr}:    IsNil=%v  (== nil does not compile)", zeroWP.IsNil())
	b.Logf("zero wrapper, method call panics=%v", func() bool {
		defer func() { recover() }()
		_ = zeroWI.Len()
		return false
	}() == false)
	for b.Loop() {
		sinkInt++
	}
}
