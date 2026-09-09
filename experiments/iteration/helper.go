package iteration

import "iter"

// Modelling proposal A's consumer-side branch, and whether a shared helper can
// hide it without giving back what it saves.

type Collector[T any] interface {
	SizeHint() int
	AsSlice() []T
	AsSeq() iter.Seq[T]
}

// Backed by a caller-owned slice: AsSlice is non-nil.
type sliceCollector[T any] struct{ s []T }

func (c sliceCollector[T]) SizeHint() int { return len(c.s) }
func (c sliceCollector[T]) AsSlice() []T  { return c.s }
func (c sliceCollector[T]) AsSeq() iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, v := range c.s {
			if !yield(v) {
				return
			}
		}
	}
}

// Backed by a container's iterator: AsSlice is nil.
type seqCollector[T any] struct {
	n   int
	seq iter.Seq[T]
}

func (c seqCollector[T]) SizeHint() int      { return c.n }
func (c seqCollector[T]) AsSlice() []T       { return nil }
func (c seqCollector[T]) AsSeq() iter.Seq[T] { return c.seq }

func FromSlice[T any](s []T) Collector[T]              { return sliceCollector[T]{s} }
func FromSeq[T any](n int, s iter.Seq[T]) Collector[T] { return seqCollector[T]{n, s} }

// ---- the three ways a consumer can be written ---------------------------

// (a) hand-written branch: the best a consumer can do.
func buildHandBranch[T comparable](c Collector[T]) map[T]struct{} {
	m := make(map[T]struct{}, c.SizeHint())
	if s := c.AsSlice(); s != nil {
		for _, v := range s {
			m[v] = struct{}{}
		}
		return m
	}
	for v := range c.AsSeq() {
		m[v] = struct{}{}
	}
	return m
}

// (b) a shared helper taking a callback.
func Each[T any](c Collector[T], f func(T)) {
	if s := c.AsSlice(); s != nil {
		for _, v := range s {
			f(v)
		}
		return
	}
	for v := range c.AsSeq() {
		f(v)
	}
}

func buildEach[T comparable](c Collector[T]) map[T]struct{} {
	m := make(map[T]struct{}, c.SizeHint())
	Each(c, func(v T) { m[v] = struct{}{} })
	return m
}

// (c) ignore AsSlice entirely and always take the iterator.
func buildSeqOnly[T comparable](c Collector[T]) map[T]struct{} {
	m := make(map[T]struct{}, c.SizeHint())
	for v := range c.AsSeq() {
		m[v] = struct{}{}
	}
	return m
}

// (d) a helper that hands back a rangeable slice, materialising when it must.
func Slice[T any](c Collector[T]) []T {
	if s := c.AsSlice(); s != nil {
		return s
	}
	out := make([]T, 0, c.SizeHint())
	for v := range c.AsSeq() {
		out = append(out, v)
	}
	return out
}

func buildViaSlice[T comparable](c Collector[T]) map[T]struct{} {
	m := make(map[T]struct{}, c.SizeHint())
	for _, v := range Slice(c) {
		m[v] = struct{}{}
	}
	return m
}
