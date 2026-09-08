package viewiface

import "iter"

// The container, and the mutable element type that makes conversion necessary.

type Item struct{ Name string }

// ItemView is the read-only handle a viewer converts an *Item into.
type ItemView struct{ i *Item }

func (v ItemView) Name() string { return v.i.Name }

type Dict[K comparable, V any] struct{ m map[K]V }

func NewDict[K comparable, V any]() *Dict[K, V] { return &Dict[K, V]{m: map[K]V{}} }

func (d *Dict[K, V]) Len() int     { return len(d.m) }
func (d *Dict[K, V]) Set(k K, v V) { d.m[k] = v }
func (d *Dict[K, V]) Get(k K) (V, bool) {
	v, ok := d.m[k]
	return v, ok
}

func (d *Dict[K, V]) All() iter.Seq2[K, V] {
	m := d.m
	return func(yield func(K, V) bool) {
		for k, v := range m {
			if !yield(k, v) {
				return
			}
		}
	}
}

// Viewer is ADR 0012's shape: keys convert both ways, values outbound only.
type Viewer[K, NK, V, NV any] interface {
	ToKeyView(K) NK
	FromKeyView(NK) (K, bool)
	ToValueView(V) NV
}

// itemViewer is deliberately STATEFUL. Converting a key back from its view
// generally needs a registry -- a string cannot be turned into the *Item it
// names without one. This is why the type-parameter witness from the views
// experiment, which materialises its converter's zero value, cannot express
// ADR 0012's viewers.
type itemViewer struct{ known map[string]*Item }

func (v itemViewer) ToKeyView(i *Item) string { return i.Name }
func (v itemViewer) FromKeyView(s string) (*Item, bool) {
	i, ok := v.known[s]
	return i, ok
}
func (v itemViewer) ToValueView(i *Item) ItemView { return ItemView{i} }

// Contract is the read-only interface a container satisfies structurally --
// what a concrete view is passed as when it crosses a boundary today.
type Contract[K, V any] interface {
	Len() int
	Get(K) (V, bool)
}

// ---- A: the shape ADR 0012 built. Container pointer + viewer field, so three
// words wide and not pointer-shaped. ----------------------------------------

type FieldView[K comparable, V, NK, NV any] struct {
	d      *Dict[K, V]
	viewer Viewer[K, NK, V, NV]
}

func ViewField[K comparable, V, NK, NV any](d *Dict[K, V], vw Viewer[K, NK, V, NV]) FieldView[K, V, NK, NV] {
	return FieldView[K, V, NK, NV]{d, vw}
}

func (v FieldView[K, V, NK, NV]) Len() int { return v.d.Len() }
func (v FieldView[K, V, NK, NV]) Get(nk NK) (NV, bool) {
	k, ok := v.viewer.FromKeyView(nk)
	if !ok {
		var zero NV
		return zero, false
	}
	raw, ok := v.d.Get(k)
	if !ok {
		var zero NV
		return zero, false
	}
	return v.viewer.ToValueView(raw), true
}

// ---- B: the same struct behind a sealed interface. The seal is what keeps the
// container out of it: *Dict has Len and Get, but not sealedView. -----------

type DictView[NK, NV any] interface {
	Len() int
	Get(NK) (NV, bool)
	sealedView()
}

func (v FieldView[K, V, NK, NV]) sealedView() {}

func ViewIface[K comparable, V, NK, NV any](d *Dict[K, V], vw Viewer[K, NK, V, NV]) DictView[NK, NV] {
	return FieldView[K, V, NK, NV]{d, vw}
}

// ---- C: pointer-shaped. One word, so it boxes for free, but the constructor
// always allocates. ---------------------------------------------------------

type ptrViewBody[K comparable, V, NK, NV any] struct {
	d      *Dict[K, V]
	viewer Viewer[K, NK, V, NV]
}

type PtrView[K comparable, V, NK, NV any] struct {
	b *ptrViewBody[K, V, NK, NV]
}

func ViewPtr[K comparable, V, NK, NV any](d *Dict[K, V], vw Viewer[K, NK, V, NV]) PtrView[K, V, NK, NV] {
	return PtrView[K, V, NK, NV]{&ptrViewBody[K, V, NK, NV]{d, vw}}
}

func (v PtrView[K, V, NK, NV]) Len() int { return v.b.d.Len() }
func (v PtrView[K, V, NK, NV]) Get(nk NK) (NV, bool) {
	k, ok := v.b.viewer.FromKeyView(nk)
	if !ok {
		var zero NV
		return zero, false
	}
	raw, ok := v.b.d.Get(k)
	if !ok {
		var zero NV
		return zero, false
	}
	return v.b.viewer.ToValueView(raw), true
}

// ---- D2: methods directly on a pointer to the view, with no wrapper struct.
//
// PtrView above wraps a pointer to a body in a one-word struct. The obvious
// question is what the wrapper buys over handing out the body pointer itself,
// which is already pointer-shaped. This is that form, for comparison.

type DirectView[K comparable, V, NK, NV any] struct {
	d      *Dict[K, V]
	viewer Viewer[K, NK, V, NV]
}

func ViewDirect[K comparable, V, NK, NV any](d *Dict[K, V], vw Viewer[K, NK, V, NV]) *DirectView[K, V, NK, NV] {
	return &DirectView[K, V, NK, NV]{d, vw}
}

func (v *DirectView[K, V, NK, NV]) Len() int { return v.d.Len() }
func (v *DirectView[K, V, NK, NV]) Get(nk NK) (NV, bool) {
	k, ok := v.viewer.FromKeyView(nk)
	if !ok {
		var zero NV
		return zero, false
	}
	raw, ok := v.d.Get(k)
	if !ok {
		var zero NV
		return zero, false
	}
	return v.viewer.ToValueView(raw), true
}

// ---- A view that converts nothing, behind a sealed interface.
//
// SortedSetView in the library carries no viewer (ADR 0012 decision 3), so it
// is one word. The question option B raises is whether putting it behind an
// interface makes it allocate for the first time.

type ShallowView[K comparable, V any] struct{ d *Dict[K, V] }

type SealedShallow[K, V any] interface {
	Len() int
	Get(K) (V, bool)
	sealedView()
}

func (v ShallowView[K, V]) Len() int          { return v.d.Len() }
func (v ShallowView[K, V]) Get(k K) (V, bool) { return v.d.Get(k) }
func (v ShallowView[K, V]) sealedView()       {}

func ViewShallowSealed[K comparable, V any](d *Dict[K, V]) SealedShallow[K, V] {
	return ShallowView[K, V]{d}
}
