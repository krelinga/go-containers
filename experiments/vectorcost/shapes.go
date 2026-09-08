package vectorcost

import "iter"

// noCopy is vet's copylocks marker, per ADR 0002.
type noCopy struct{}

func (noCopy) Lock()   {}
func (noCopy) Unlock() {}

// Vector is the proposed shape: ADR 0002's container form over an owned slice.
type Vector[T any] struct {
	noCopy
	es []T
}

func NewVector[T any](vs ...T) *Vector[T] {
	return &Vector[T]{es: append([]T(nil), vs...)}
}

func (v *Vector[T]) Len() int       { return len(v.es) }
func (v *Vector[T]) At(i int) T     { return v.es[i] }
func (v *Vector[T]) Set(i int, e T) { v.es[i] = e }

func (v *Vector[T]) Append(es ...T) {
	v.es = append(v.es, es...) // touches the receiver outside any loop
}

func (v *Vector[T]) All() iter.Seq[T] {
	es := v.es // eager, per ADR 0002
	return func(yield func(T) bool) {
		for _, e := range es {
			if !yield(e) {
				return
			}
		}
	}
}

// AllIndexed is the index/value form. It cannot be called All, because Elems[T]
// already claims that name for the value-only sequence.
func (v *Vector[T]) AllIndexed() iter.Seq2[int, T] {
	es := v.es
	return func(yield func(int, T) bool) {
		for i, e := range es {
			if !yield(i, e) {
				return
			}
		}
	}
}

// AppendOne is the non-variadic form, to separate the cost of the container
// from the cost of the ...T signature ADR 0004 established for mutators.
func (v *Vector[T]) AppendOne(e T) { v.es = append(v.es, e) }

// SumInternal sums from inside the package, where the compiler can see the
// slice directly. If the indexed-loop penalty is the bounds check, this
// recovers it; if it is the method call, it does not.
func (v *Vector[T]) sumIsNotGeneric() {}
