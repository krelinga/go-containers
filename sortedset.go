package containers

import (
	"cmp"
	"iter"
	"slices"
)

// SortedSet is a set kept in ascending order, backed by a sorted slice.
//
// It is a reference type (ADR 0018): a one-word value whose copies share the
// same contents. The backing slice sits behind a pointer because a slice header
// must be replaced when it grows -- that replacement is what every copy must
// see, and is the aliasing surprise this library was begun over.
//
// The zero value reads as empty and panics on a write, matching a nil map:
//
//	var s SortedSet[int]
//	s.Len()      // 0
//	s.Has(1)     // false
//	s.Add(1)     // panics
type SortedSet[T cmp.Ordered] struct {
	st *sortedSetState[T]
}

type sortedSetState[T cmp.Ordered] struct{ es []T }

// NewSortedSet returns a SortedSet containing vs.
//
// vs is copied before anything is sorted, so the caller's slice is never
// reordered and is not retained (ADR 0017).
func NewSortedSet[T cmp.Ordered](vs ...T) SortedSet[T] {
	s := SortedSet[T]{st: &sortedSetState[T]{}}
	s.AddAll(vs...)
	return s
}

// IsZero reports whether s was ever constructed.
func (s SortedSet[T]) IsZero() bool { return s.st == nil }

// Add inserts one element, O(n). AddAll sorts and merges instead, which is much
// cheaper for more than a couple of elements.
func (s SortedSet[T]) Add(v T) {
	if i, found := slices.BinarySearch(s.st.es, v); !found {
		s.st.es = slices.Insert(s.st.es, i, v)
	}
}

// AddAll inserts every element of vs, sorting once and merging:
// O(k log k + n + k) against O(kn).
func (s SortedSet[T]) AddAll(vs ...T) {
	if len(vs) == 0 {
		return // writes nothing, so it does not panic -- as the builtin does not
	}
	s.st.es = mergeSortedValues(s.st.es, sortDistinct(slices.Clone(vs)))
}

// Delete removes one element, or does nothing if it is absent. Deleting from a
// zero SortedSet is a no-op, as delete on a nil map is.
func (s SortedSet[T]) Delete(v T) {
	if s.st == nil {
		return
	}
	if i, found := slices.BinarySearch(s.st.es, v); found {
		s.st.es = slices.Delete(s.st.es, i, i+1)
	}
}

// DeleteAll removes every element of ks. s.DeleteAll(s.KeySlice()...) is safe,
// because KeySlice already returned a copy (ADR 0017).
func (s SortedSet[T]) DeleteAll(ks ...T) {
	if s.st == nil {
		return
	}
	for _, k := range ks {
		if i, found := slices.BinarySearch(s.st.es, k); found {
			s.st.es = slices.Delete(s.st.es, i, i+1)
		}
	}
}

// Has reports whether v is in the set.
//
// This calls slices.BinarySearch directly rather than searching through a
// comparator closure, which ADR 0005 measured at roughly 3.4x.
func (s SortedSet[T]) Has(v T) bool {
	if s.st == nil {
		return false
	}
	_, found := slices.BinarySearch(s.st.es, v)
	return found
}

// Len returns the number of elements.
func (s SortedSet[T]) Len() int {
	if s.st == nil {
		return 0
	}
	return len(s.st.es)
}

// Keys iterates the elements in ascending order.
//
// # What the iterator is bound to
//
// The slice HEADER is captured here, at the call, not at iteration. That is a
// weaker guarantee than a snapshot, and the difference matters:
//
//   - A later Add, Delete or any change of LENGTH is not seen: the captured
//     header keeps the old length, and a growth reallocates away from it.
//   - A later write to an element already in range IS seen, because the header
//     points at the same backing array. An insert that shifts elements within
//     spare capacity is seen the same way.
//
// So the guarantee is: taking an iterator never panics and never observes a
// reallocation. Modifying a container while iterating it remains unsupported,
// exactly as it is for a builtin map.
//
// Capturing the header before the single return is also what keeps the nil
// check off the heap. Two returns yielding two closures, or a closure that
// ranges the inner sequence and re-yields, each allocate (ADR 0018).
func (s SortedSet[T]) Keys() iter.Seq[T] {
	var es []T
	if s.st != nil {
		es = s.st.es
	}
	return seqOverValues(es)
}

// KeySlice returns the elements as a new slice, in ascending order.
func (s SortedSet[T]) KeySlice() []T {
	if s.st == nil {
		return nil
	}
	return slices.Clone(s.st.es)
}

// RangeKeys iterates the elements in [lo, hi), ascending. Empty if hi <= lo.
//
// Bounds are resolved when RangeKeys is called, not when the result is
// iterated.
func (s SortedSet[T]) RangeKeys(lo, hi T) iter.Seq[T] {
	if s.st == nil {
		return func(func(T) bool) {}
	}
	i, _ := slices.BinarySearch(s.st.es, lo)
	j, _ := slices.BinarySearch(s.st.es, hi)
	if j < i {
		j = i
	}
	return seqOverValues(s.st.es[i:j])
}

func seqOverValues[T cmp.Ordered](es []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, e := range es {
			if !yield(e) {
				return
			}
		}
	}
}

// FloorKey returns the largest element <= v.
func (s SortedSet[T]) FloorKey(v T) (T, bool) {
	if s.st == nil {
		var zero T
		return zero, false
	}
	i, found := slices.BinarySearch(s.st.es, v)
	if found {
		return s.st.es[i], true
	}
	return s.at(i - 1)
}

// CeilKey returns the smallest element >= v.
func (s SortedSet[T]) CeilKey(v T) (T, bool) {
	if s.st == nil {
		var zero T
		return zero, false
	}
	i, _ := slices.BinarySearch(s.st.es, v)
	return s.at(i)
}

// MinKey returns the smallest element.
//
// Named MinKey rather than Min because a set's element is its key (ADR 0017),
// and because it is what lets SortedKeys cover sorted sets and sorted maps
// alike (ADR 0018).
func (s SortedSet[T]) MinKey() (T, bool) { return s.at(0) }

// MaxKey returns the largest element.
func (s SortedSet[T]) MaxKey() (T, bool) {
	if s.st == nil {
		var zero T
		return zero, false
	}
	return s.at(len(s.st.es) - 1)
}

func (s SortedSet[T]) at(i int) (T, bool) {
	if s.st == nil || i < 0 || i >= len(s.st.es) {
		var zero T
		return zero, false
	}
	return s.st.es[i], true
}

// Clone returns an independent copy.
func (s SortedSet[T]) Clone() SortedSet[T] {
	if s.st == nil {
		return SortedSet[T]{}
	}
	return SortedSet[T]{st: &sortedSetState[T]{es: slices.Clone(s.st.es)}}
}

// Union returns a new set holding every element of s and o.
func (s SortedSet[T]) Union(o SortedSet[T]) SortedSet[T] {
	return SortedSet[T]{st: &sortedSetState[T]{es: mergeSortedValues(s.KeySlice(), o.KeySlice())}}
}

// Intersect returns a new set holding the elements present in both.
func (s SortedSet[T]) Intersect(o SortedSet[T]) SortedSet[T] {
	var out []T
	for _, v := range s.KeySlice() {
		if o.Has(v) {
			out = append(out, v)
		}
	}
	return SortedSet[T]{st: &sortedSetState[T]{es: out}}
}

// Difference returns a new set holding the elements of s not in o.
func (s SortedSet[T]) Difference(o SortedSet[T]) SortedSet[T] {
	var out []T
	for _, v := range s.KeySlice() {
		if !o.Has(v) {
			out = append(out, v)
		}
	}
	return SortedSet[T]{st: &sortedSetState[T]{es: out}}
}

// sortDistinct sorts vs and keeps the LAST of each run of equal values.
func sortDistinct[T cmp.Ordered](vs []T) []T {
	slices.Sort(vs)
	return slices.Compact(vs)
}

// mergeSortedValues merges two ascending, distinct runs.
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
