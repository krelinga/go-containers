package iteration

import "iter"

// Native forms: what a container implements directly over a slice backing.

func nativeValues[T any](s []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, e := range s {
			if !yield(e) {
				return
			}
		}
	}
}

func nativeAll[T any](s []T) iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for i, e := range s {
			if !yield(i, e) {
				return
			}
		}
	}
}

func nativeBackward[T any](s []T) iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for i := len(s) - 1; i >= 0; i-- {
			if !yield(i, s[i]) {
				return
			}
		}
	}
}

func nativeValuesBackward[T any](s []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for i := len(s) - 1; i >= 0; i-- {
			if !yield(s[i]) {
				return
			}
		}
	}
}

// Derived forms: what a free function costs when the container offers only the
// other shape. This is the maps.Keys approach.

func derivedValues[K, V any](seq iter.Seq2[K, V]) iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, v := range seq {
			if !yield(v) {
				return
			}
		}
	}
}

func derivedKeys[K, V any](seq iter.Seq2[K, V]) iter.Seq[K] {
	return func(yield func(K) bool) {
		for k := range seq {
			if !yield(k) {
				return
			}
		}
	}
}

// Deriving the pair form from the value form has to invent the index.
func derivedAll[V any](seq iter.Seq[V]) iter.Seq2[int, V] {
	return func(yield func(int, V) bool) {
		i := 0
		for v := range seq {
			if !yield(i, v) {
				return
			}
			i++
		}
	}
}

// Reversing a bare iterator cannot stream: it must buffer, because an iter.Seq
// only runs forwards.
func derivedBackward[V any](seq iter.Seq[V]) iter.Seq[V] {
	return func(yield func(V) bool) {
		var buf []V
		for v := range seq {
			buf = append(buf, v)
		}
		for i := len(buf) - 1; i >= 0; i-- {
			if !yield(buf[i]) {
				return
			}
		}
	}
}

// ---- element size ---------------------------------------------------------
//
// The derived-is-free finding was measured with int values, 8 bytes. A Seq2
// passes its value BY VALUE into yield, so a consumer that drops it has already
// paid for the copy. Whether that matters depends entirely on the value's width.

// Big is 1024 bytes.
type Big [128]int64

// nativeKeysOnly never touches the value: the floor for a key-only walk.
func nativeKeysOnly[T any](s []T) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range s {
			if !yield(i) {
				return
			}
		}
	}
}

// nativeAllBig yields index and value, so every element is copied into yield.
func nativeAllBig(s []Big) iter.Seq2[int, Big] {
	return func(yield func(int, Big) bool) {
		for i := range s {
			if !yield(i, s[i]) {
				return
			}
		}
	}
}

// An All that yields a POINTER to the value instead of the value.
func nativeAllBigPtr(s []Big) iter.Seq2[int, *Big] {
	return func(yield func(int, *Big) bool) {
		for i := range s {
			if !yield(i, &s[i]) {
				return
			}
		}
	}
}

func derivedKeysBig(seq iter.Seq2[int, Big]) iter.Seq[int] {
	return func(yield func(int) bool) {
		for k := range seq {
			if !yield(k) {
				return
			}
		}
	}
}

func derivedKeysBigPtr(seq iter.Seq2[int, *Big]) iter.Seq[int] {
	return func(yield func(int) bool) {
		for k := range seq {
			if !yield(k) {
				return
			}
		}
	}
}

func derivedValuesBig(seq iter.Seq2[int, Big]) iter.Seq[Big] {
	return func(yield func(Big) bool) {
		for _, v := range seq {
			if !yield(v) {
				return
			}
		}
	}
}

func nativeValuesBig(s []Big) iter.Seq[Big] {
	return func(yield func(Big) bool) {
		for i := range s {
			if !yield(s[i]) {
				return
			}
		}
	}
}

// The realistic case for this library: All comes back through an interface, so
// the call is dynamic and the compiler cannot see that the consumer drops the
// value. ADR 0013's views return iterators exactly this way.
type BigSource interface {
	All() iter.Seq2[int, Big]
	Values() iter.Seq[Big]
	Keys() iter.Seq[int]
}

type bigSlice struct{ s []Big }

func (b bigSlice) All() iter.Seq2[int, Big] { return nativeAllBig(b.s) }
func (b bigSlice) Values() iter.Seq[Big]    { return nativeValuesBig(b.s) }
func (b bigSlice) Keys() iter.Seq[int]      { return nativeKeysOnly(b.s) }

func NewBigSource(s []Big) BigSource { return bigSlice{s} }

// Does the cost of a dropped value scale with its width? If it does, the copy
// is happening; if it is flat, the compiler is eliding it.

type Sz64 [8]int64    // 64 B
type Sz1K [128]int64  // 1 KiB
type Sz8K [1024]int64 // 8 KiB

func allOf[T any](s []T) iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for i := range s {
			if !yield(i, s[i]) {
				return
			}
		}
	}
}

func keysOf[T any](s []T) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range s {
			if !yield(i) {
				return
			}
		}
	}
}

func dropValue[K, V any](seq iter.Seq2[K, V]) iter.Seq[K] {
	return func(yield func(K) bool) {
		for k := range seq {
			if !yield(k) {
				return
			}
		}
	}
}

// Returned through an interface so the call is opaque, as a view's is.
type seqSource[T any] interface {
	all() iter.Seq2[int, T]
	keys() iter.Seq[int]
}

type sliceSource[T any] struct{ s []T }

func (x sliceSource[T]) all() iter.Seq2[int, T] { return allOf(x.s) }
func (x sliceSource[T]) keys() iter.Seq[int]    { return keysOf(x.s) }

func newSource[T any](s []T) seqSource[T] { return sliceSource[T]{s} }

// ---- cross-container construction ----------------------------------------
//
// Building a Vector of a dict's keys. Today the only route is an iter.Seq,
// which loses the length; ADR 0006 measured what a length is worth. And if the
// keys are derived from an All, the value is copied and discarded per element.

// A dict-shaped source, generic so the compiler cannot see through it.
type dictSource[K comparable, V any] struct {
	keys []K
	vals []V
}

func (d dictSource[K, V]) Len() int { return len(d.keys) }

func (d dictSource[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for i := range d.keys {
			if !yield(d.keys[i], d.vals[i]) {
				return
			}
		}
	}
}

// The native key walk a container could offer, which never touches a value.
func (d dictSource[K, V]) Keys() iter.Seq[K] {
	return func(yield func(K) bool) {
		for i := range d.keys {
			if !yield(d.keys[i]) {
				return
			}
		}
	}
}

func newDictSource[K comparable, V any](keys []K, vals []V) Elems2[K, V] {
	return dictSource[K, V]{keys, vals}
}

// Elems2 as this library declares it.
type Elems2[K, V any] interface {
	Len() int
	All() iter.Seq2[K, V]
}

type Elems[T any] interface {
	Len() int
	All() iter.Seq[T]
}

// A shape adapter that preserves the length, which is what today's route loses.
type sized[T any] struct {
	n   int
	seq iter.Seq[T]
}

func (s sized[T]) Len() int         { return s.n }
func (s sized[T]) All() iter.Seq[T] { return s.seq }

// KeysOf derives keys, upgrading to a native walk when the source offers one.
func KeysOf[K, V any](e Elems2[K, V]) Elems[K] {
	if native, ok := e.(interface{ Keys() iter.Seq[K] }); ok {
		return sized[K]{e.Len(), native.Keys()}
	}
	return sized[K]{e.Len(), dropValue(e.All())}
}

// KeysOfDerived always derives, for comparison.
func KeysOfDerived[K, V any](e Elems2[K, V]) Elems[K] {
	return sized[K]{e.Len(), dropValue(e.All())}
}

// The two collectors: one with a length, one without.
func collectSized[T any](src Elems[T]) []T {
	out := make([]T, 0, src.Len())
	for v := range src.All() {
		out = append(out, v)
	}
	return out
}

func collectSeq[T any](seq iter.Seq[T]) []T {
	var out []T
	for v := range seq {
		out = append(out, v)
	}
	return out
}
