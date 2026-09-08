package viewiface

import "iter"

// The hybrid: base view types stay pure interfaces, the most-derived (ordered)
// view types become structs wrapping a sealed inner interface.
//
// The question this answers is whether a STRUCT can satisfy the base INTERFACE.
// It can -- which matters, because it restores implicitly the substitution that
// struct embedding lost.

// Base: a pure sealed interface, exactly as ADR 0013 has it.
type HDictView[NK, NV any] interface {
	Len() int
	Get(NK) (NV, bool)
	All() iter.Seq2[NK, NV]
	sealedHView()
}

// The inner interface the ordered struct wraps.
type innerOrdered[K, NV any] interface {
	Len() int
	Get(K) (NV, bool)
	All() iter.Seq2[K, NV]
	Min() (K, NV, bool)
	sealedHView()
}

// Most-derived: a struct, so it can carry IsNil.
type HSortedDictView[K, NV any] struct{ impl innerOrdered[K, NV] }

func (v HSortedDictView[K, NV]) IsNil() bool           { return v.impl == nil }
func (v HSortedDictView[K, NV]) sealedHView()          {}
func (v HSortedDictView[K, NV]) Len() int              { return v.impl.Len() }
func (v HSortedDictView[K, NV]) Get(k K) (NV, bool)    { return v.impl.Get(k) }
func (v HSortedDictView[K, NV]) All() iter.Seq2[K, NV] { return v.impl.All() }
func (v HSortedDictView[K, NV]) Min() (K, NV, bool)    { return v.impl.Min() }

// A one-word variant, to see whether the substitution boxing can be avoided.
type hOrderedBody[K, NV any] struct{ impl innerOrdered[K, NV] }

type HSortedPtrView[K, NV any] struct{ b *hOrderedBody[K, NV] }

func (v HSortedPtrView[K, NV]) IsNil() bool           { return v.b == nil }
func (v HSortedPtrView[K, NV]) sealedHView()          {}
func (v HSortedPtrView[K, NV]) Len() int              { return v.b.impl.Len() }
func (v HSortedPtrView[K, NV]) Get(k K) (NV, bool)    { return v.b.impl.Get(k) }
func (v HSortedPtrView[K, NV]) All() iter.Seq2[K, NV] { return v.b.impl.All() }
func (v HSortedPtrView[K, NV]) Min() (K, NV, bool)    { return v.b.impl.Min() }

// The implementation behind both.
type hOrderedImpl[K comparable, V, NK, NV any] struct{ FieldView[K, V, NK, NV] }

func (h hOrderedImpl[K, V, NK, NV]) sealedHView()        {}
func (h hOrderedImpl[K, V, NK, NV]) Min() (NK, NV, bool) { var k NK; var v NV; return k, v, false }

func NewHSortedDictView[K comparable, V, NK, NV any](d *Dict[K, V], vw Viewer[K, NK, V, NV]) HSortedDictView[NK, NV] {
	return HSortedDictView[NK, NV]{hOrderedImpl[K, V, NK, NV]{FieldView[K, V, NK, NV]{d, vw}}}
}

func NewHSortedPtrView[K comparable, V, NK, NV any](d *Dict[K, V], vw Viewer[K, NK, V, NV]) HSortedPtrView[NK, NV] {
	return HSortedPtrView[NK, NV]{&hOrderedBody[NK, NV]{hOrderedImpl[K, V, NK, NV]{FieldView[K, V, NK, NV]{d, vw}}}}
}

// A hash view stays a pure interface.
func NewHDictView[K comparable, V, NK, NV any](d *Dict[K, V], vw Viewer[K, NK, V, NV]) HDictView[NK, NV] {
	return hOrderedImpl[K, V, NK, NV]{FieldView[K, V, NK, NV]{d, vw}}
}

// THE CLAIM: an ordered view struct is usable where the base interface is
// wanted, with no explicit conversion. This line is the test.
var (
	_ HDictView[string, ItemView] = HSortedDictView[string, ItemView]{}
	_ HDictView[string, ItemView] = HSortedPtrView[string, ItemView]{}
)
