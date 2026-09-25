package sealing

import (
	"testing"
	"unsafe"
)

const coN = 64

// Non-allocating conversions. An allocating viewer (fmt.Sprint) buries every
// difference under ~60 allocations per walk; the first run of this harness did
// exactly that and showed nothing.
type coDouble struct{}

func (coDouble) Convert(i int) int { return i * 2 }

type coBump struct{}

func (coBump) Convert(i int) int { return i + 1 }

var (
	coSinkInt int
	coSinkV   coView
)

func TestComposeAllocs(t *testing.T) {
	s := newCoSet(coN)
	r := func(name string, f func()) {
		t.Logf("%-44s %.1f allocs/op", name, testing.AllocsPerRun(300, f))
	}
	r("construct: baseline (container source)", func() { coSinkV = viewFromContainer(s, coDouble{}) })
	r("construct: A (concrete view source)", func() { coSinkV = viewFromView(coIdentity(s), coDouble{}) })
	r("construct: B (sealed tier source)", func() { coSinkV = viewFromTier(coIdentity(s), coDouble{}) })

	one := viewFromContainer(s, coDouble{})
	r("compose 2 deep: A", func() { coSinkV = viewFromView(one, coBump{}) })
	r("compose 2 deep: B", func() { coSinkV = viewFromTier(one, coBump{}) })
	two := viewFromView(one, coBump{})
	r("compose 3 deep: A", func() { coSinkV = viewFromView(two, coBump{}) })
}

func BenchmarkCompose(b *testing.B) {
	s := newCoSet(coN)
	base := viewFromContainer(s, coDouble{})
	a1 := viewFromView(coIdentity(s), coDouble{})
	b1 := viewFromTier(coIdentity(s), coDouble{})
	a2 := viewFromView(base, coBump{})
	b2 := viewFromTier(base, coBump{})
	a3 := viewFromView(a2, coBump{})

	run := func(name string, v coView) {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				c := 0
				for range v.Keys() {
					c++
				}
				coSinkInt = c
			}
		})
	}
	run("1-deep/baseline-container", base)
	run("1-deep/A-concrete-view", a1)
	run("1-deep/B-sealed-tier", b1)
	run("2-deep/A-concrete-view", a2)
	run("2-deep/B-sealed-tier", b2)
	run("3-deep/A-concrete-view", a3)
}

var (
	coSinkSlice coSlice[int]
	coSinkSView coSliceView[int]
)

// ADR 0023 part 3. A slice header is three words where every other container is
// one, so the one-word-boxes-free rule (ADR 0022) does not reach it.
func TestSliceAdapterCosts(t *testing.T) {
	raw := []int{1, 2, 3}

	t.Logf("width: defined []T = %d B, a one-word container = 8 B", unsafe.Sizeof(coSlice[int](nil)))

	cs := CastCoSlice(raw) // T inferred; a conversion could not
	t.Logf("CastSlice infers: %T", cs)
	t.Logf("CastSlice:        %.1f allocs/op", testing.AllocsPerRun(300, func() { coSinkSlice = CastCoSlice(raw) }))
	t.Logf("Slice.View():     %.1f allocs/op  <- three words cannot box free",
		testing.AllocsPerRun(300, func() { coSinkSView = CastCoSlice(raw).View() }))

	// distinct view types, shared plumbing, zero value still reads empty
	var zero coSliceView[int]
	t.Logf("zero SliceView reads empty: Len=%d", zero.Len())
	var _ coVectorView[int] // distinct type; a coSliceView is not assignable to it
}
