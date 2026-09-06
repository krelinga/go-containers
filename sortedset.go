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
	s.Add(vs...)
	return s
}

// CollectSortedSet returns a SortedSet holding every value in src.
//
// src.Len() is used to size the working slice, so this allocates once where
// CollectSortedSetSeq must grow as it goes. The saving is allocation volume
// rather than wall-clock time: the sort that follows dominates. See ADR 0006.
//
// It is also cheaper than NewSortedSet followed by AddAll, since with nothing
// to merge against the merge degenerates to the sort.
func CollectSortedSet[T cmp.Ordered](src Elems[T]) *SortedSet[T] {
	return &SortedSet[T]{es: collectSortedValues(src.All(), src.Len())}
}

// CollectSortedSetSeq returns a SortedSet holding every value in seq.
//
// Prefer CollectSortedSet when the source knows its length. Use this for bare
// iterators — maps.Keys, slices.Values, or another container's Range — which
// cannot report one.
func CollectSortedSetSeq[T cmp.Ordered](seq iter.Seq[T]) *SortedSet[T] {
	return &SortedSet[T]{es: collectSortedValues(seq, 0)}
}

// Add adds vs to the set. Values already present are ignored.
//
// Adding a single value is an insert, O(n). Adding several sorts them once and
// merges, O(k log k + n + k) — so passing many values in one call is much
// cheaper than calling Add repeatedly. AddAll does the same for an iterator.
func (s *SortedSet[T]) Add(vs ...T) {
	base := s.es // eager, so a nil receiver panics even when vs is empty
	switch len(vs) {
	case 0:
		return
	case 1:
		if i, found := slices.BinarySearch(base, vs[0]); !found {
			s.es = slices.Insert(base, i, vs[0])
		}
		return
	}
	s.es = mergeSortedValues(base, sortDistinct(slices.Clone(vs)))
}

// AddAll adds every value in src, sorting once and merging rather than
// inserting one at a time. See Add for the cost comparison.
//
// src.Len() sizes the working slice; it is a hint, and a wrong one costs only a
// worse allocation. src is fully consumed before the set is modified, so
// s.AddAll(s) is well defined, if pointless.
func (s *SortedSet[T]) AddAll(src Elems[T]) {
	base := s.es                   // eager, so a nil receiver panics even for an empty src
	seq, n := src.All(), src.Len() // eager, so a nil src panics too
	s.addAll(base, collectSortedValues(seq, n))
}

// AddAllSeq is AddAll for a bare iterator, which cannot report its length.
// Prefer AddAll when the source knows it.
func (s *SortedSet[T]) AddAllSeq(seq iter.Seq[T]) {
	base := s.es // eager, so a nil receiver panics even when seq is empty
	s.addAll(base, collectSortedValues(seq, 0))
}

func (s *SortedSet[T]) addAll(base, add []T) {
	if len(add) == 0 {
		return
	}
	s.es = mergeSortedValues(base, add)
}

// Remove removes vs from the set. Values not present are ignored.
func (s *SortedSet[T]) Remove(vs ...T) {
	if s.es == nil {
		// Also forces the nil-receiver panic when vs is empty, which a bare
		// range over vs would skip.
		return
	}
	for _, v := range vs {
		if i, found := slices.BinarySearch(s.es, v); found {
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
func (s *SortedSet[T]) All() iter.Seq[T] {
	es := s.es // eager, so a nil receiver panics here like every other method
	return seqOverValues(es)
}

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
