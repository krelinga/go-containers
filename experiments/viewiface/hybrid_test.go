package viewiface

import "testing"

//go:noinline
func takeHBase(v HDictView[string, ItemView], k string) ItemView { r, _ := v.Get(k); return r }

//go:noinline
func takeHSortedStruct(v HSortedDictView[string, ItemView], k string) ItemView {
	r, _ := v.Get(k)
	return r
}

//go:noinline
func takeHSortedPtr(v HSortedPtrView[string, ItemView], k string) ItemView {
	r, _ := v.Get(k)
	return r
}

func BenchmarkHybrid(b *testing.B) {
	d, vw, probe := fixture()
	hash := NewHDictView[*Item, *Item, string, ItemView](d, vw)
	ord := NewHSortedDictView[*Item, *Item, string, ItemView](d, vw)
	ordP := NewHSortedPtrView[*Item, *Item, string, ItemView](d, vw)

	// Construction.
	b.Run("Construct/HashAsInterface", func(b *testing.B) {
		for b.Loop() {
			sinkHDict = NewHDictView[*Item, *Item, string, ItemView](d, vw)
		}
	})
	b.Run("Construct/OrderedAsStruct", func(b *testing.B) {
		for b.Loop() {
			sinkHSorted = NewHSortedDictView[*Item, *Item, string, ItemView](d, vw)
		}
	})
	b.Run("Construct/OrderedAsPtrStruct", func(b *testing.B) {
		for b.Loop() {
			sinkHSortedP = NewHSortedPtrView[*Item, *Item, string, ItemView](d, vw)
		}
	})

	// Calling on the type you were handed -- no conversion.
	b.Run("Call/HashAsInterface", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = takeHBase(hash, probe)
		}
	})
	b.Run("Call/OrderedAsStruct", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = takeHSortedStruct(ord, probe)
		}
	})
	b.Run("Call/OrderedAsPtrStruct", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = takeHSortedPtr(ordP, probe)
		}
	})

	// THE SUBSTITUTION: an ordered view handed to a base-typed boundary.
	b.Run("Substitute/OrderedStructAsBase", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = takeHBase(ord, probe)
		}
	})
	b.Run("Substitute/OrderedPtrStructAsBase", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = takeHBase(ordP, probe)
		}
	})
	// For contrast: today, where both are interfaces, substitution is an
	// interface-to-interface conversion.
	b.Run("Substitute/InterfaceAsBase", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = takeHBase(hash, probe)
		}
	})
}

var (
	sinkHDict    HDictView[string, ItemView]
	sinkHSorted  HSortedDictView[string, ItemView]
	sinkHSortedP HSortedPtrView[string, ItemView]
)

func BenchmarkReportHybrid(b *testing.B) {
	var zeroHash HDictView[string, ItemView]
	var zeroOrd HSortedDictView[string, ItemView]

	b.Logf("hash view (interface): spelled `v == nil`   -> %v", zeroHash == nil)
	b.Logf("ordered view (struct): spelled `v.IsNil()`  -> %v", zeroOrd.IsNil())
	b.Logf("--> the two halves of the API still disagree; the split moved, it did not close")
	b.Logf("widths: base interface=%d  ordered struct=%d  ordered ptr-struct=%d",
		sizeOf(sinkHDict), sizeOf(sinkHSorted), sizeOf(sinkHSortedP))
	for b.Loop() {
		sinkInt++
	}
}

// Does a zero ordered-view STRUCT, once it substitutes into the base INTERFACE,
// still read as nil? This is Go's typed-nil trap, and the hybrid is exactly the
// shape that produces it.
func BenchmarkReportHybridTypedNil(b *testing.B) {
	var zeroOrd HSortedDictView[string, ItemView]

	// The struct knows it is empty.
	b.Logf("zero ordered struct: IsNil()=%v", zeroOrd.IsNil())

	// Substituted into the base interface, it is a non-nil interface holding a
	// zero struct.
	var asBase HDictView[string, ItemView] = zeroOrd
	b.Logf("same value as the base interface: asBase == nil -> %v", asBase == nil)

	panicked := func() (p bool) {
		defer func() { p = recover() != nil }()
		_ = asBase.Len()
		return false
	}()
	b.Logf("...and calling it panics -> %v", panicked)
	b.Logf("so `v == nil` reports FALSE for a view that is empty and unusable")

	// Today, where both are interfaces, a nil view is nil at every static type.
	var todayNil HDictView[string, ItemView]
	b.Logf("today (both interfaces): nil view == nil -> %v", todayNil == nil)

	for b.Loop() {
		sinkInt++
	}
}
