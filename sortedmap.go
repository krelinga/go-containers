package containers

import (
	"cmp"
	"iter"
	"slices"
)

// SortedMap is a key/value container kept in ascending key order, backed by a
// sorted slice of entries.
//
// It is a reference type (ADR 0018): a one-word value whose copies share the
// same contents, with the backing slice behind a pointer so that a reallocation
// is visible to every holder.
//
// The zero value reads as empty and panics on a write, matching a nil map.
type SortedMap[K cmp.Ordered, V any] struct {
	st *sortedMapState[K, V]
}

type sortedMapState[K cmp.Ordered, V any] struct{ entries []sortedEntry[K, V] }

type sortedEntry[K cmp.Ordered, V any] struct {
	k K
	v V
}

// NewSortedMap returns a SortedMap holding kvs, last occurrence winning for a
// repeated key (ADR 0004).
//
// kvs is copied before anything is sorted, so the caller's slice is never
// reordered and is not retained.
func NewSortedMap[K cmp.Ordered, V any](kvs ...Entry[K, V]) SortedMap[K, V] {
	m := SortedMap[K, V]{st: &sortedMapState[K, V]{}}
	m.SetAll(kvs...)
	return m
}

// IsZero reports whether m was ever constructed.
func (m SortedMap[K, V]) IsZero() bool { return m.st == nil }

func (m SortedMap[K, V]) find(k K) (int, bool) {
	return slices.BinarySearchFunc(m.st.entries, k,
		func(e sortedEntry[K, V], k K) int { return cmp.Compare(e.k, k) })
}

// Set writes one entry. Writing to a zero SortedMap panics.
func (m SortedMap[K, V]) Set(k K, v V) {
	i, found := m.find(k)
	if found {
		m.st.entries[i].v = v
		return
	}
	m.st.entries = slices.Insert(m.st.entries, i, sortedEntry[K, V]{k, v})
}

// SetAll writes every entry of kvs, sorting once and merging. kvs is copied
// before anything is sorted, so the caller's slice is never reordered.
func (m SortedMap[K, V]) SetAll(kvs ...Entry[K, V]) {
	if len(kvs) == 0 {
		return // writes nothing, so it does not panic
	}
	add := make([]sortedEntry[K, V], len(kvs))
	for i, kv := range kvs {
		add[i] = sortedEntry[K, V]{k: kv.Slot, v: kv.Value}
	}
	m.st.entries = mergeSortedEntries(m.st.entries, sortDistinctEntries(add))
}

// Get returns the value stored under k.
func (m SortedMap[K, V]) Get(k K) (V, bool) {
	if m.st == nil {
		var zero V
		return zero, false
	}
	if i, found := m.find(k); found {
		return m.st.entries[i].v, true
	}
	var zero V
	return zero, false
}

// Has reports whether k is present.
func (m SortedMap[K, V]) Has(k K) bool {
	if m.st == nil {
		return false
	}
	_, found := m.find(k)
	return found
}

// Delete removes the entry under k, or does nothing if it is absent.
func (m SortedMap[K, V]) Delete(k K) {
	if m.st == nil {
		return
	}
	if i, found := m.find(k); found {
		m.st.entries = slices.Delete(m.st.entries, i, i+1)
	}
}

// DeleteAll removes the entries under every key in ks.
func (m SortedMap[K, V]) DeleteAll(ks ...K) {
	if m.st == nil {
		return
	}
	for _, k := range ks {
		if i, found := m.find(k); found {
			m.st.entries = slices.Delete(m.st.entries, i, i+1)
		}
	}
}

// Len returns the number of entries.
func (m SortedMap[K, V]) Len() int {
	if m.st == nil {
		return 0
	}
	return len(m.st.entries)
}

// All iterates the entries in ascending key order.
//
// The slice header is captured at the call, not at iteration. See SortedSet.Keys
// for exactly what that does and does not guarantee -- it is weaker than a
// snapshot, and the same in every slice-backed container here.
func (m SortedMap[K, V]) All() iter.Seq2[K, V] {
	var es []sortedEntry[K, V]
	if m.st != nil {
		es = m.st.entries
	}
	return seq2Over(es)
}

// Keys iterates the keys in ascending order.
func (m SortedMap[K, V]) Keys() iter.Seq[K] {
	var es []sortedEntry[K, V]
	if m.st != nil {
		es = m.st.entries
	}
	return func(yield func(K) bool) {
		for _, e := range es {
			if !yield(e.k) {
				return
			}
		}
	}
}

// Values iterates the values, in ascending order of their keys.
func (m SortedMap[K, V]) Values() iter.Seq[V] {
	var es []sortedEntry[K, V]
	if m.st != nil {
		es = m.st.entries
	}
	return func(yield func(V) bool) {
		for _, e := range es {
			if !yield(e.v) {
				return
			}
		}
	}
}

// KeySlice returns the keys as a new slice, ascending.
func (m SortedMap[K, V]) KeySlice() []K {
	if m.st == nil {
		return nil
	}
	out := make([]K, 0, len(m.st.entries))
	for _, e := range m.st.entries {
		out = append(out, e.k)
	}
	return out
}

// ValueSlice returns the values as a new slice, in ascending key order.
func (m SortedMap[K, V]) ValueSlice() []V {
	if m.st == nil {
		return nil
	}
	out := make([]V, 0, len(m.st.entries))
	for _, e := range m.st.entries {
		out = append(out, e.v)
	}
	return out
}

// AllSlice returns the entries as a new slice of pairs, ascending.
func (m SortedMap[K, V]) AllSlice() []Entry[K, V] {
	if m.st == nil {
		return nil
	}
	out := make([]Entry[K, V], 0, len(m.st.entries))
	for _, e := range m.st.entries {
		out = append(out, Entry[K, V]{e.k, e.v})
	}
	return out
}

// Range iterates the entries with lo <= key < hi, ascending. Empty if hi <= lo.
func (m SortedMap[K, V]) Range(lo, hi K) iter.Seq2[K, V] {
	if m.st == nil {
		return func(func(K, V) bool) {}
	}
	i, j := m.bounds(lo, hi)
	return seq2Over(m.st.entries[i:j])
}

// RangeKeys iterates the keys with lo <= key < hi, ascending.
//
// This is the native key-only walk. Deriving it from Range would copy every
// value only to discard it, which ADR 0017 measured at 14.9x for wide values.
func (m SortedMap[K, V]) RangeKeys(lo, hi K) iter.Seq[K] {
	if m.st == nil {
		return func(func(K) bool) {}
	}
	i, j := m.bounds(lo, hi)
	es := m.st.entries[i:j]
	return func(yield func(K) bool) {
		for _, e := range es {
			if !yield(e.k) {
				return
			}
		}
	}
}

func (m SortedMap[K, V]) bounds(lo, hi K) (int, int) {
	i, _ := m.find(lo)
	j, _ := m.find(hi)
	if j < i {
		j = i
	}
	return i, j
}

func seq2Over[K cmp.Ordered, V any](es []sortedEntry[K, V]) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, e := range es {
			if !yield(e.k, e.v) {
				return
			}
		}
	}
}

// Floor returns the entry with the largest key <= k.
func (m SortedMap[K, V]) Floor(k K) (K, V, bool) {
	if m.st == nil {
		var zk K
		var zv V
		return zk, zv, false
	}
	i, found := m.find(k)
	if found {
		return m.at(i)
	}
	return m.at(i - 1)
}

// Ceil returns the entry with the smallest key >= k.
func (m SortedMap[K, V]) Ceil(k K) (K, V, bool) {
	if m.st == nil {
		var zk K
		var zv V
		return zk, zv, false
	}
	i, _ := m.find(k)
	return m.at(i)
}

// Min returns the entry with the smallest key.
func (m SortedMap[K, V]) Min() (K, V, bool) { return m.at(0) }

// Max returns the entry with the largest key.
func (m SortedMap[K, V]) Max() (K, V, bool) {
	if m.st == nil {
		var zk K
		var zv V
		return zk, zv, false
	}
	return m.at(len(m.st.entries) - 1)
}

// FloorKey, CeilKey, MinKey and MaxKey are the key-only forms. They are what
// lets SortedKeys cover sorted sets and sorted maps alike (ADR 0018); a caller
// who wants the value calls Get, or uses the pair-returning form above.

func (m SortedMap[K, V]) FloorKey(k K) (K, bool) { kk, _, ok := m.Floor(k); return kk, ok }
func (m SortedMap[K, V]) CeilKey(k K) (K, bool)  { kk, _, ok := m.Ceil(k); return kk, ok }
func (m SortedMap[K, V]) MinKey() (K, bool)      { kk, _, ok := m.Min(); return kk, ok }
func (m SortedMap[K, V]) MaxKey() (K, bool)      { kk, _, ok := m.Max(); return kk, ok }

func (m SortedMap[K, V]) at(i int) (K, V, bool) {
	if m.st == nil || i < 0 || i >= len(m.st.entries) {
		var zk K
		var zv V
		return zk, zv, false
	}
	e := m.st.entries[i]
	return e.k, e.v, true
}

// Clone returns an independent copy.
func (m SortedMap[K, V]) Clone() SortedMap[K, V] {
	if m.st == nil {
		return SortedMap[K, V]{}
	}
	return SortedMap[K, V]{st: &sortedMapState[K, V]{entries: slices.Clone(m.st.entries)}}
}

// sortDistinctEntries sorts es by key and keeps the LAST entry of each run of
// equal keys.
//
// slices.CompactFunc keeps the FIRST, which would silently diverge from
// repeated Set and from ADR 0004's last-write-wins rule. Tests covering this
// must use distinguishable values, or they pass either way.
func sortDistinctEntries[K cmp.Ordered, V any](es []sortedEntry[K, V]) []sortedEntry[K, V] {
	slices.SortStableFunc(es, func(a, b sortedEntry[K, V]) int { return cmp.Compare(a.k, b.k) })
	out := es[:0]
	for i, e := range es {
		if i+1 < len(es) && es[i+1].k == e.k {
			continue // a later entry carries this key
		}
		out = append(out, e)
	}
	return out
}

// mergeSortedEntries merges two key-sorted, key-distinct runs. Entries from add
// replace equal keys in base.
func mergeSortedEntries[K cmp.Ordered, V any](base, add []sortedEntry[K, V]) []sortedEntry[K, V] {
	out := make([]sortedEntry[K, V], 0, len(base)+len(add))
	i, j := 0, 0
	for i < len(base) && j < len(add) {
		switch cmp.Compare(base[i].k, add[j].k) {
		case -1:
			out = append(out, base[i])
			i++
		case +1:
			out = append(out, add[j])
			j++
		default:
			out = append(out, add[j])
			i++
			j++
		}
	}
	out = append(out, base[i:]...)
	return append(out, add[j:]...)
}
