package viewiface

import "iter"

// Views as lightweight structs wrapping a sealed interface.
//
// ADR 0013 hands out a sealed interface directly. That makes a view
// nil-comparable, which is what ADR 0014 objects to: containers would spell the
// emptiness check IsNil() and views would spell it == nil, two spellings for one
// idea. Wrapping the interface in a struct with an unexported field gives views
// the same spelling as containers.
//
// Two wrappings are possible and they are not equivalent.

// The inner interface, sealed exactly as ADR 0013 seals DictView.
type innerView[NK, NV any] interface {
	Len() int
	Get(NK) (NV, bool)
	All() iter.Seq2[NK, NV]
	sealedInner()
}

func (v FieldView[K, V, NK, NV]) sealedInner() {}
func (v FieldView[K, V, NK, NV]) All() iter.Seq2[NK, NV] {
	seq, vw := v.d.All(), v.viewer
	return func(yield func(NK, NV) bool) {
		for k, raw := range seq {
			if !yield(vw.ToKeyView(k), vw.ToValueView(raw)) {
				return
			}
		}
	}
}

// ---- (a) struct wrapping the interface: two words ------------------------

type WrapIface[NK, NV any] struct{ impl innerView[NK, NV] }

func ViewWrapIface[K comparable, V, NK, NV any](d *Dict[K, V], vw Viewer[K, NK, V, NV]) WrapIface[NK, NV] {
	return WrapIface[NK, NV]{FieldView[K, V, NK, NV]{d, vw}}
}

func (v WrapIface[NK, NV]) IsNil() bool            { return v.impl == nil }
func (v WrapIface[NK, NV]) Len() int               { return v.impl.Len() }
func (v WrapIface[NK, NV]) Get(nk NK) (NV, bool)   { return v.impl.Get(nk) }
func (v WrapIface[NK, NV]) All() iter.Seq2[NK, NV] { return v.impl.All() }

// ---- (b) struct wrapping a pointer to a body holding the interface -------
// One word, so it boxes into a foreign interface for free -- at the cost of an
// allocation at construction and a second indirection.

type viewBody[NK, NV any] struct{ impl innerView[NK, NV] }

type WrapPtr[NK, NV any] struct{ b *viewBody[NK, NV] }

func ViewWrapPtr[K comparable, V, NK, NV any](d *Dict[K, V], vw Viewer[K, NK, V, NV]) WrapPtr[NK, NV] {
	return WrapPtr[NK, NV]{&viewBody[NK, NV]{FieldView[K, V, NK, NV]{d, vw}}}
}

func (v WrapPtr[NK, NV]) IsNil() bool            { return v.b == nil }
func (v WrapPtr[NK, NV]) Len() int               { return v.b.impl.Len() }
func (v WrapPtr[NK, NV]) Get(nk NK) (NV, bool)   { return v.b.impl.Get(nk) }
func (v WrapPtr[NK, NV]) All() iter.Seq2[NK, NV] { return v.b.impl.All() }

// ---- today's shape, for the All comparison -------------------------------

type BareIface[NK, NV any] interface {
	Len() int
	Get(NK) (NV, bool)
	All() iter.Seq2[NK, NV]
	sealedInner()
}

func ViewBare[K comparable, V, NK, NV any](d *Dict[K, V], vw Viewer[K, NK, V, NV]) BareIface[NK, NV] {
	return FieldView[K, V, NK, NV]{d, vw}
}

// ---- a foreign interface a view might be passed to, e.g. Elems2 ----------

type Iterable[NK, NV any] interface {
	Len() int
	All() iter.Seq2[NK, NV]
}
