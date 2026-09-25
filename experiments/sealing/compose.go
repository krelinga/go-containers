package sealing

import "iter"

// Composing views: ADR 0023's question. Today a converting constructor takes the
// CONCRETE CONTAINER, so a view cannot be narrowed or re-converted by whoever
// holds it. Two source-parameter shapes could fix that, and they differ in where
// the cost lands.
//
// Names here are prefixed `co` to stay clear of the sealing shapes above.

type coSet struct{ m map[int]struct{} }

func newCoSet(n int) coSet {
	s := coSet{m: make(map[int]struct{}, n)}
	for i := range n {
		s.m[i] = struct{}{}
	}
	return s
}

func (s coSet) Len() int { return len(s.m) }
func (s coSet) Keys() iter.Seq[int] {
	m := s.m
	return func(yield func(int) bool) {
		for t := range m {
			if !yield(t) {
				return
			}
		}
	}
}
func (s coSet) coToken() {}

// coTier is the sealed public tier; coInner its unexported mirror, as shipped.
type coTier interface {
	Len() int
	Keys() iter.Seq[int]
	coToken()
}

type coInner interface {
	Len() int
	Keys() iter.Seq[int]
}

type coViewer interface{ Convert(int) int }

// The view: a concrete struct wrapping the inner tier, exactly as shipped.
type coView struct{ impl coInner }

func (v coView) Len() int            { return v.impl.Len() }
func (v coView) Keys() iter.Seq[int] { return v.impl.Keys() }
func (v coView) coToken()            {}

func coIdentity(s coSet) coView { return coView{impl: s} }

// --- BASELINE: source is the concrete CONTAINER (today). Not composable: a
// coView cannot be passed here at all. The inner walk is a static call. ---

type convFromContainer struct {
	s  coSet
	vw coViewer
}

func (c convFromContainer) Len() int { return c.s.Len() }
func (c convFromContainer) Keys() iter.Seq[int] {
	s, vw := c.s, c.vw
	return func(yield func(int) bool) {
		for t := range s.Keys() {
			if !yield(vw.Convert(t)) {
				return
			}
		}
	}
}

func viewFromContainer(s coSet, vw coViewer) coView {
	return coView{impl: convFromContainer{s, vw}}
}

// --- OPTION A: source is the concrete VIEW type. Composable; no boxing, because
// the parameter is concrete. The impl holds a struct that itself holds an
// interface, so obtaining the iterator crosses one more dynamic call. ---

type convFromView struct {
	v  coView
	vw coViewer
}

func (c convFromView) Len() int { return c.v.Len() }
func (c convFromView) Keys() iter.Seq[int] {
	v, vw := c.v, c.vw
	return func(yield func(int) bool) {
		for t := range v.Keys() {
			if !yield(vw.Convert(t)) {
				return
			}
		}
	}
}

func viewFromView(v coView, vw coViewer) coView {
	return coView{impl: convFromView{v, vw}}
}

// --- OPTION B: source is the SEALED TIER. Maximally composable -- any view that
// exposes keys fits, whatever container it came from -- and one hop rather than
// two. But a two-word view must be BOXED to enter it. ---

type convFromTier struct {
	k  coTier
	vw coViewer
}

func (c convFromTier) Len() int { return c.k.Len() }
func (c convFromTier) Keys() iter.Seq[int] {
	k, vw := c.k, c.vw
	return func(yield func(int) bool) {
		for t := range k.Keys() {
			if !yield(vw.Convert(t)) {
				return
			}
		}
	}
}

func viewFromTier(k coTier, vw coViewer) coView {
	return coView{impl: convFromTier{k, vw}}
}
