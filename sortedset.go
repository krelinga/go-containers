package containers

import (
	"cmp"
	"iter"
	"slices"
)

// A SortedSet is a set whose values are kept in ascending order.
//
// The zero value is an empty SortedSet ready to use:
//
//	var s containers.SortedSet[int]
//	s.Add(10)
//
// As with Set and SortedDict, every method has a pointer receiver, a nil
// *SortedSet panics on any method, and copying the struct shares the backing
// array — use Clone, and prefer a SortedSet field over a *SortedSet field.
// `go vet` reports struct copies, though `go test` does not run that check: use
// `go vet ./...` or `go test -vet=all`.
//
// # Choosing between Set and SortedSet
//
// Set is map-backed: O(1) membership and insertion, but no order. SortedSet is
// backed by a sorted slice: O(log n) membership, O(n) insertion, and ordered
// iteration, range scans and Floor/Ceil for free.
//
// Prefer Set unless ordering is *queried*. Sorting a Set's contents once for
// output is cheaper than maintaining order on every insert; SortedSet earns its
// place when Range, Floor, Ceil, Min or Max are called repeatedly.
//
// Random-order insertion is O(n) per value, since each insert memmoves half the
// backing array. AddAll sorts once and merges instead — see its documentation.
//
// A SortedSet is not safe for concurrent use.
type SortedSet[T cmp.Ordered] struct {
	_  noCopy
	es []T
}

// NewSortedSet returns a SortedSet containing vs. The zero value is equally
// usable.
func NewSortedSet[T cmp.Ordered](vs ...T) *SortedSet[T] {
	s := &SortedSet[T]{}
	s.AddAll(vs...)
	return s
}

// Add inserts one element. Values already present are ignored.
//
// This is an insert, O(n). AddAll sorts and merges instead, which is much
// cheaper for more than a couple of elements — so prefer it in a loop.
func (s *SortedSet[T]) Add(v T) {
	base := s.es // eager, so a nil receiver panics here
	if i, found := slices.BinarySearch(base, v); !found {
		s.es = slices.Insert(base, i, v)
	}
}

// AddAll inserts every element of vs, sorting once and merging rather than
// inserting one at a time: O(k log k + n + k) against O(kn). Spread a slice to
// bulk-insert:
//
//	s.AddAll(other.KeySlice()...)
//
// vs is copied before anything is sorted, so the caller's slice is never
// reordered and is not retained.
func (s *SortedSet[T]) AddAll(vs ...T) {
	base := s.es // eager, so a nil receiver panics even when vs is empty
	if len(vs) == 0 {
		return
	}
	s.es = mergeSortedValues(base, sortDistinct(slices.Clone(vs)))
}

// Delete removes one element. Values not present are ignored.
func (s *SortedSet[T]) Delete(v T) {
	if s.es == nil {
		return
	}
	if i, found := slices.BinarySearch(s.es, v); found {
		s.es = slices.Delete(s.es, i, i+1)
	}
}

// DeleteAll removes every element of ks. Spread a slice to bulk-delete:
//
//	s.DeleteAll(s.KeySlice()...)
//
// That call is safe because KeySlice already returned a copy; deleting from a
// container while walking it is not supported, and this contract is what makes
// the obvious spelling correct rather than merely lucky.
func (s *SortedSet[T]) DeleteAll(ks ...T) {
	if s.es == nil {
		// Forces the nil-receiver panic when ks is empty, which a bare range
		// over ks would skip.
		return
	}
	for _, k := range ks {
		if i, found := slices.BinarySearch(s.es, k); found {
			s.es = slices.Delete(s.es, i, i+1)
		}
	}
}

// Has reports whether v is in the set.
//
// This calls slices.BinarySearch directly rather than searching through a
// comparator closure, which ADR 0005 measured at roughly 3.4x — the reason
// SortedSet owns its backing rather than wrapping SortedDict[T, struct{}].
func (s *SortedSet[T]) Has(v T) bool {
	_, found := slices.BinarySearch(s.es, v)
	return found
}

// Len returns the number of values in the set.
func (s *SortedSet[T]) Len() int { return len(s.es) }

// All returns an iterator over the values in ascending order.
//
// The iterator is bound to the set's contents as of the call to All, not as of
// iteration. Modifying the set during iteration is not supported.
func (s *SortedSet[T]) Keys() iter.Seq[T] {
	es := s.es // eager, so a nil receiver panics here like every other method
	return seqOverValues(es)
}

// KeySlice returns the elements of the set as a new slice, in ascending order.
//
// The result is a full, independent copy (ADR 0017): nothing the set does
// afterwards is visible through it, and nothing done to it is visible in the
// set. That is what makes s.DeleteAll(s.KeySlice()...) safe.
func (s *SortedSet[T]) KeySlice() []T { return slices.Clone(s.es) }

// Range returns an iterator over the values with lo <= v < hi, in ascending
// order. If hi <= lo the range is empty.
//
// Bounds are resolved when Range is called, not when the result is iterated.
// The iterator does not expose the backing array.
func (s *SortedSet[T]) Range(lo, hi T) iter.Seq[T] {
	i, _ := slices.BinarySearch(s.es, lo)
	j, _ := slices.BinarySearch(s.es, hi)
	if j < i {
		j = i
	}
	return seqOverValues(s.es[i:j])
}

func seqOverValues[T cmp.Ordered](es []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, v := range es {
			if !yield(v) {
				return
			}
		}
	}
}

// Floor returns the largest value <= v.
func (s *SortedSet[T]) Floor(v T) (T, bool) {
	i, found := slices.BinarySearch(s.es, v)
	if !found {
		i-- // BinarySearch returned the insertion point; the value before it is the floor
	}
	return s.at(i)
}

// Ceil returns the smallest value >= v.
func (s *SortedSet[T]) Ceil(v T) (T, bool) {
	i, _ := slices.BinarySearch(s.es, v) // exact index if present, else the first greater
	return s.at(i)
}

// Min returns the smallest value.
func (s *SortedSet[T]) Min() (T, bool) { return s.at(0) }

// Max returns the largest value.
func (s *SortedSet[T]) Max() (T, bool) { return s.at(len(s.es) - 1) }

// at returns the value at i, reporting false if i is out of range. It is the
// single place the ordered lookups' boundary conditions live.
func (s *SortedSet[T]) at(i int) (T, bool) {
	if i < 0 || i >= len(s.es) {
		var zero T
		return zero, false
	}
	return s.es[i], true
}

// Clone returns an independent copy. Mutating the result does not affect s.
func (s *SortedSet[T]) Clone() *SortedSet[T] {
	return &SortedSet[T]{es: slices.Clone(s.es)}
}

// Union returns a new set containing every value in s or o.
//
// Both operands are already sorted, so this is a linear merge, O(n+m), rather
// than n probes of O(log m).
func (s *SortedSet[T]) Union(o *SortedSet[T]) *SortedSet[T] {
	return &SortedSet[T]{es: mergeSortedValues(s.es, o.es)}
}

// Intersect returns a new set containing the values in both s and o.
func (s *SortedSet[T]) Intersect(o *SortedSet[T]) *SortedSet[T] {
	a, b := s.es, o.es
	out := make([]T, 0, min(len(a), len(b)))
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch cmp.Compare(a[i], b[j]) {
		case -1:
			i++
		case +1:
			j++
		default:
			out = append(out, a[i])
			i++
			j++
		}
	}
	return &SortedSet[T]{es: out}
}

// Difference returns a new set containing the values in s that are not in o.
func (s *SortedSet[T]) Difference(o *SortedSet[T]) *SortedSet[T] {
	a, b := s.es, o.es
	out := make([]T, 0, len(a))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch cmp.Compare(a[i], b[j]) {
		case -1:
			out = append(out, a[i])
			i++
		case +1:
			j++
		default:
			i++
			j++
		}
	}
	return &SortedSet[T]{es: append(out, a[i:]...)}
}

// collectSortedValues drains seq into a sorted, distinct slice. Unlike
// SortedDict's equivalent there is no last-write-wins question: the values are
// the keys, so duplicates simply collapse.
// collectSortedValues drains seq into a sorted, distinct slice, preallocating
// to sizeHint when it is positive. The hint only affects allocation: every
// value seq yields is appended regardless of how many that turns out to be.
func collectSortedValues[T cmp.Ordered](seq iter.Seq[T], sizeHint int) []T {
	var vs []T
	if sizeHint > 0 {
		vs = make([]T, 0, sizeHint)
	}
	for v := range seq {
		vs = append(vs, v)
	}
	return sortDistinct(vs)
}

func sortDistinct[T cmp.Ordered](vs []T) []T {
	slices.Sort(vs)
	return slices.Compact(vs)
}

// mergeSortedValues merges two ascending, distinct runs into a new slice.
func mergeSortedValues[T cmp.Ordered](a, b []T) []T {
	out := make([]T, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch cmp.Compare(a[i], b[j]) {
		case -1:
			out = append(out, a[i])
			i++
		case +1:
			out = append(out, b[j])
			j++
		default:
			out = append(out, a[i])
			i++
			j++
		}
	}
	out = append(out, a[i:]...)
	return append(out, b[j:]...)
}
