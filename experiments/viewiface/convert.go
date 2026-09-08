package viewiface

import "iter"

// If structs cannot subtype, substitution has to become an explicit conversion.
// This measures whether that conversion is free.

type innerSorted[K, NV any] interface {
	innerView[K, NV]
	Min() (K, NV, bool)
}

type WrapSorted[K, NV any] struct{ impl innerSorted[K, NV] }

// Dict re-wraps the same interface value in the base struct. No allocation
// should be needed: the value is already boxed.
func (v WrapSorted[K, NV]) Dict() WrapIface[K, NV] { return WrapIface[K, NV]{v.impl} }

func (v WrapSorted[K, NV]) Len() int              { return v.impl.Len() }
func (v WrapSorted[K, NV]) Get(k K) (NV, bool)    { return v.impl.Get(k) }
func (v WrapSorted[K, NV]) All() iter.Seq2[K, NV] { return v.impl.All() }
func (v WrapSorted[K, NV]) Min() (K, NV, bool)    { return v.impl.Min() }

type sortedImpl[K comparable, V, NK, NV any] struct{ FieldView[K, V, NK, NV] }

func (s sortedImpl[K, V, NK, NV]) Min() (NK, NV, bool) { var k NK; var v NV; return k, v, false }

func ViewWrapSorted[K comparable, V, NK, NV any](d *Dict[K, V], vw Viewer[K, NK, V, NV]) WrapSorted[NK, NV] {
	return WrapSorted[NK, NV]{sortedImpl[K, V, NK, NV]{FieldView[K, V, NK, NV]{d, vw}}}
}
