package viewiface

import "testing"

func BenchmarkOrderedToBaseConversion(b *testing.B) {
	d, vw, _ := fixture()
	sv := ViewWrapSorted[*Item, *Item, string, ItemView](d, vw)

	b.Run("Convert", func(b *testing.B) {
		for b.Loop() {
			sinkWrapI = sv.Dict()
		}
	})
	b.Run("ConvertAndCall", func(b *testing.B) {
		for b.Loop() {
			sinkInt = takesBaseNoInline(sv.Dict())
		}
	})
}

//go:noinline
func takesBaseNoInline(v wrapIForm) int { return v.Len() }
