package sliceadapter

import (
	"iter"
	"slices"
	"unsafe"
)

func sizeOf[T any](v T) int { return int(unsafe.Sizeof(v)) }

// Elems is the library's sized read-only contract (ADR 0006).
type Elems[T any] interface {
	Len() int
	All() iter.Seq[T]
}

// ---- the adapter under consideration -------------------------------------

type Slice[T any] []T

func (s Slice[T]) Len() int       { return len(s) }
func (s Slice[T]) At(i int) T     { return s[i] }
func (s Slice[T]) Set(i int, e T) { s[i] = e }
func (s Slice[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, e := range s {
			if !yield(e) {
				return
			}
		}
	}
}

// ---- Vector, modelled faithfully from the shipped implementation ---------

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

type Vector[T any] struct {
	_  noCopy
	es []T
}

func (v *Vector[T]) Len() int   { return len(v.es) }
func (v *Vector[T]) At(i int) T { return v.es[i] }

func (v *Vector[T]) AppendAll(src Elems[T]) {
	n := src.Len()
	es := slices.Grow(v.es, max(0, n))
	for e := range src.All() {
		es = append(es, e)
	}
	v.es = es
}

func (v *Vector[T]) AppendAllSeq(seq iter.Seq[T]) {
	es := v.es
	for e := range seq {
		es = append(es, e)
	}
	v.es = es
}

func (v *Vector[T]) All() iter.Seq[T] {
	es := v.es
	return func(yield func(T) bool) {
		for _, e := range es {
			if !yield(e) {
				return
			}
		}
	}
}

// ---- views: what a sequence view costs over each backing ------------------

type VectorView[NT any] interface {
	Len() int
	At(int) NT
	All() iter.Seq[NT]
	sealedView()
}

// Over a Vector: one word, the container pointer.
type vectorIdentityView[T any] struct{ vec *Vector[T] }

func (v vectorIdentityView[T]) sealedView()      {}
func (v vectorIdentityView[T]) Len() int         { return v.vec.Len() }
func (v vectorIdentityView[T]) At(i int) T       { return v.vec.At(i) }
func (v vectorIdentityView[T]) All() iter.Seq[T] { return v.vec.All() }

func ViewVectorIdentity[T any](vec *Vector[T]) VectorView[T] {
	return vectorIdentityView[T]{vec}
}

// Over a Slice held BY VALUE: three words, so not pointer-shaped.
type sliceValueView[T any] struct{ s Slice[T] }

func (v sliceValueView[T]) sealedView()      {}
func (v sliceValueView[T]) Len() int         { return v.s.Len() }
func (v sliceValueView[T]) At(i int) T       { return v.s.At(i) }
func (v sliceValueView[T]) All() iter.Seq[T] { return v.s.All() }

func ViewSliceValue[T any](s Slice[T]) VectorView[T] { return sliceValueView[T]{s} }

// Over a Slice held BY POINTER: one word.
type slicePtrView[T any] struct{ s *Slice[T] }

func (v slicePtrView[T]) sealedView()      {}
func (v slicePtrView[T]) Len() int         { return v.s.Len() }
func (v slicePtrView[T]) At(i int) T       { return v.s.At(i) }
func (v slicePtrView[T]) All() iter.Seq[T] { return v.s.All() }

func ViewSlicePtr[T any](s *Slice[T]) VectorView[T] { return slicePtrView[T]{s} }
