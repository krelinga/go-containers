package sliceadapter

import "iter"

// Can a view be had WITHOUT introducing a Slice type at all -- straight over a
// plain []T?

type bareSliceView[T any] struct{ s *[]T } // one word, so it boxes free

func (v bareSliceView[T]) sealedView() {}
func (v bareSliceView[T]) Len() int    { return len(*v.s) }
func (v bareSliceView[T]) At(i int) T  { return (*v.s)[i] }
func (v bareSliceView[T]) All() iter.Seq[T] {
	es := *v.s // eager, so a nil receiver panics here
	return func(yield func(T) bool) {
		for _, e := range es {
			if !yield(e) {
				return
			}
		}
	}
}

// ViewBareSlice takes the address of an ordinary slice variable or field.
func ViewBareSlice[T any](s *[]T) VectorView[T] { return bareSliceView[T]{s} }

// Elems2 is the library's key/value sized contract.
type Elems2[K, V any] interface {
	Len() int
	All() iter.Seq2[K, V]
}
