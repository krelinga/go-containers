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

// ---------------------------------------------------------------------------
// A defined slice type as an adapter -- proposed inside ADR 0023 and WITHDRAWN.
// Kept because the question is deferred to a later ADR and these are its inputs,
// and because the boxing rule generalises: a value wider than one word cannot
// enter an interface without allocating.
//
// Two questions. Can a defined []T satisfy the full position-keyed inner tier
// on VALUE receivers (no Append -- a value receiver cannot grow a slice)? And
// what does its View() cost, given that a slice header is three words where
// every other container is one?
// ---------------------------------------------------------------------------

type coEntry[P, V any] struct {
	Slot  P
	Value V
}

// the full inner tier, as contracts.go declares it
type coPositionValues[P, V any] interface {
	Len() int
	Positions() iter.Seq[P]
	PositionSlice() []P
	Values() iter.Seq[V]
	ValueSlice() []V
	At(P) V
	All() iter.Seq2[P, V]
	AllSlice() []coEntry[P, V]
}

type coSlice[T any] []T

func (s coSlice[T]) Len() int   { return len(s) }
func (s coSlice[T]) At(i int) T { return s[i] }

// Set works on a value receiver: an element write goes through the shared
// backing array. Append cannot -- the length is in the header, which IS the
// value (ADR 0016).
func (s coSlice[T]) Set(i int, v T) { s[i] = v }

func (s coSlice[T]) Positions() iter.Seq[int] {
	return func(y func(int) bool) {
		for i := range s {
			if !y(i) {
				return
			}
		}
	}
}
func (s coSlice[T]) Values() iter.Seq[T] {
	return func(y func(T) bool) {
		for _, v := range s {
			if !y(v) {
				return
			}
		}
	}
}
func (s coSlice[T]) All() iter.Seq2[int, T] {
	return func(y func(int, T) bool) {
		for i, v := range s {
			if !y(i, v) {
				return
			}
		}
	}
}
func (s coSlice[T]) PositionSlice() []int {
	out := make([]int, len(s))
	for i := range s {
		out[i] = i
	}
	return out
}
func (s coSlice[T]) ValueSlice() []T { return append([]T(nil), s...) }
func (s coSlice[T]) AllSlice() []coEntry[int, T] {
	out := make([]coEntry[int, T], len(s))
	for i, v := range s {
		out[i] = coEntry[int, T]{i, v}
	}
	return out
}

// A defined slice type satisfies the whole tier on value receivers.
var _ coPositionValues[int, string] = coSlice[string](nil)

// CastSlice: a conversion cannot infer its type argument; a function call can.
func CastCoSlice[T any](s []T) coSlice[T] { return coSlice[T](s) }

// Two DISTINCT view types over ONE shared inner tier -- ADR 0023 keeps
// SliceView and VectorView separate because they are expected to diverge.
type coSliceView[NT any] struct{ impl coPositionValues[int, NT] }

func (v coSliceView[NT]) Len() int {
	if v.impl == nil {
		return 0
	}
	return v.impl.Len()
}
func (v coSliceView[NT]) At(i int) NT { return v.impl.At(i) }

type coVectorView[NT any] struct{ impl coPositionValues[int, NT] }

func (v coVectorView[NT]) Len() int {
	if v.impl == nil {
		return 0
	}
	return v.impl.Len()
}
func (v coVectorView[NT]) At(i int) NT { return v.impl.At(i) }

func (s coSlice[T]) View() coSliceView[T] { return coSliceView[T]{impl: s} }
