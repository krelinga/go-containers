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
