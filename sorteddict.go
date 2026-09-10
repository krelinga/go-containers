package containers

import (
	"cmp"
	"iter"
	"slices"
)

// sortedEntry declares its value field FIRST. A zero-sized field in trailing
// position forces Go to pad the struct, which would make SortedDict[K, struct{}]
// twice the width of a bare key in both memory and insert time. The ordering
// costs nothing for a non-empty V. Measured in experiments/sortedsetbacking;
// same rule that decides where noCopy goes (ADR 0002, decision 5).
type sortedEntry[K cmp.Ordered, V any] struct {
	v V
	k K
}

// A SortedDict is a map whose keys are kept in ascending order.
//
// The zero value is an empty SortedDict ready to use:
//
//	var m containers.SortedDict[int, string]
//	m.Set(10, "0.90")
//
// Like Set, every method has a pointer receiver, a nil *SortedDict panics on any
// method, and copying the struct shares the backing array — use Clone, and
// prefer a SortedDict field over a *SortedDict field. `go vet` reports struct
// copies, though `go test` does not run that check: use `go vet ./...` or
// `go test -vet=all`.
//
// # Performance
//
// A SortedDict is backed by a sorted slice, so lookup is O(log n), ordered
// iteration and range scans are sequential reads, and insertion is O(n) — each
// out-of-order insert memmoves half the backing array.
//
// That last point is the one thing to know before choosing this type.
// experiments/sortedbacking measured it against a B-tree: the slice wins lookup
// (~1.4x), ordered iteration (~8x) and range scans (~13x at scale), and loses
// only on random-order insertion, where the crossover falls between 64 and 512
// entries and the gap reaches 59x at 65 536. **Inserting in ascending key order
// avoids this entirely** — an in-order insert is an append, and beats the tree
// at every size measured. Bulk-load sorted data where you control the order.
//
// A SortedDict is not safe for concurrent use.
type SortedDict[K cmp.Ordered, V any] struct {
	_       noCopy
	entries []sortedEntry[K, V]
}

// NewSortedDict returns a SortedDict holding kvs. The zero value is equally
// usable.
//
// Spread a slice to build from another container:
//
//	NewSortedDict(other.AllSlice()...)
//
// Where kvs repeats a key, the last occurrence wins (ADR 0004). kvs is copied
// before anything is sorted, so the caller's slice is never reordered and is
// not retained.
func NewSortedDict[K cmp.Ordered, V any](kvs ...KeyValue[K, V]) *SortedDict[K, V] {
	d := &SortedDict[K, V]{}
	d.SetAll(kvs...)
	return d
}

// find returns the index of k, or the index where it would be inserted.
func (m *SortedDict[K, V]) find(k K) (int, bool) {
	return slices.BinarySearchFunc(m.entries, k, func(e sortedEntry[K, V], k K) int {
		return cmp.Compare(e.k, k)
	})
}

// Set associates v with k, replacing any existing value.
func (m *SortedDict[K, V]) Set(k K, v V) {
	if i, found := m.find(k); found {
		m.entries[i].v = v
	} else {
		m.entries = slices.Insert(m.entries, i, sortedEntry[K, V]{k: k, v: v})
	}
}

// Get returns the value associated with k.
func (m *SortedDict[K, V]) Get(k K) (V, bool) {
	if i, found := m.find(k); found {
		return m.entries[i].v, true
	}
	var zero V
	return zero, false
}

// Delete removes k. Deleting an absent key is a no-op.
//
// It reports nothing so that the signature matches Map.Delete and both types
// satisfy MutableDict; the builtin delete cannot report presence without an
// extra lookup. Use Get first if you need to know. See ADR 0007.
func (m *SortedDict[K, V]) Delete(k K) {
	if i, found := m.find(k); found {
		m.entries = slices.Delete(m.entries, i, i+1)
	}
}

// Len returns the number of entries.
func (m *SortedDict[K, V]) Len() int { return len(m.entries) }

// All returns an iterator over every entry in ascending key order.
//
// The iterator is bound to the map's contents as of the call to All, not as of
// iteration. Modifying the map during iteration is not supported.
func (m *SortedDict[K, V]) All() iter.Seq2[K, V] {
	es := m.entries // eager, so a nil receiver panics here like every other method
	return seq2Over(es)
}

// Keys returns an iterator over the keys in ascending order.
func (m *SortedDict[K, V]) Keys() iter.Seq[K] {
	es := m.entries // eager, so a nil receiver panics here
	return func(yield func(K) bool) {
		for _, e := range es {
			if !yield(e.k) {
				return
			}
		}
	}
}

// Values returns an iterator over the values, in ascending order of their keys.
func (m *SortedDict[K, V]) Values() iter.Seq[V] {
	es := m.entries
	return func(yield func(V) bool) {
		for _, e := range es {
			if !yield(e.v) {
				return
			}
		}
	}
}

// KeySlice returns the keys as a new slice, in ascending order.
//
// The result is a full, independent copy (ADR 0017), which is what makes
// m.DeleteAll(m.KeySlice()...) safe — deleting from a container while walking it
// is not supported, and this contract removes the hazard rather than
// documenting it.
func (m *SortedDict[K, V]) KeySlice() []K {
	out := make([]K, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, e.k)
	}
	return out
}

// ValueSlice returns the values as a new slice, in ascending order of their
// keys.
func (m *SortedDict[K, V]) ValueSlice() []V {
	out := make([]V, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, e.v)
	}
	return out
}

// AllSlice returns the entries as a new slice of pairs, in ascending key order.
// It is the bulk-transfer shape: NewSortedDict(m.AllSlice()...).
func (m *SortedDict[K, V]) AllSlice() []KeyValue[K, V] {
	out := make([]KeyValue[K, V], 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, KeyValue[K, V]{e.k, e.v})
	}
	return out
}

// Range returns an iterator over the entries with lo <= key < hi, in ascending
// key order. If hi <= lo the range is empty.
//
// Bounds are resolved when Range is called, not when the result is iterated.
// The iterator does not expose the map's backing array; a caller cannot mutate
// the map through it.
func (m *SortedDict[K, V]) Range(lo, hi K) iter.Seq2[K, V] {
	i, _ := m.find(lo)
	j, _ := m.find(hi)
	if j < i {
		j = i
	}
	return seq2Over(m.entries[i:j])
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
func (m *SortedDict[K, V]) Floor(k K) (K, V, bool) {
	i, found := m.find(k)
	if !found {
		i-- // find returned the insertion point; the entry before it is the floor
	}
	return m.at(i)
}

// Ceil returns the entry with the smallest key >= k.
func (m *SortedDict[K, V]) Ceil(k K) (K, V, bool) {
	i, _ := m.find(k) // exact index if present, else the first key greater than k
	return m.at(i)
}

// Min returns the entry with the smallest key.
func (m *SortedDict[K, V]) Min() (K, V, bool) { return m.at(0) }

// Max returns the entry with the largest key.
func (m *SortedDict[K, V]) Max() (K, V, bool) { return m.at(len(m.entries) - 1) }

// at returns the entry at i, reporting false if i is out of range. It is the
// single place the boundary conditions of the ordered lookups live.
func (m *SortedDict[K, V]) at(i int) (K, V, bool) {
	if i < 0 || i >= len(m.entries) {
		var zk K
		var zv V
		return zk, zv, false
	}
	e := m.entries[i]
	return e.k, e.v, true
}

// Clone returns an independent copy. Mutating the result does not affect m.
func (m *SortedDict[K, V]) Clone() *SortedDict[K, V] {
	return &SortedDict[K, V]{entries: slices.Clone(m.entries)}
}

// SetAll writes every entry of kvs, sorting once and merging rather than
// inserting one at a time. Where kvs repeats a key, the last occurrence wins
// (ADR 0004).
//
// Spread a slice to bulk-write:
//
//	m.SetAll(other.AllSlice()...)
//
// kvs is copied before anything is sorted, so the caller's slice is never
// reordered and is not retained.
func (m *SortedDict[K, V]) SetAll(kvs ...KeyValue[K, V]) {
	base := m.entries // eager, so a nil receiver panics even when kvs is empty
	if len(kvs) == 0 {
		return
	}
	add := make([]sortedEntry[K, V], len(kvs))
	for i, kv := range kvs {
		add[i] = sortedEntry[K, V]{k: kv.Key, v: kv.Value}
	}
	m.setAll(base, sortDistinctEntries(add))
}

func (m *SortedDict[K, V]) setAll(base, add []sortedEntry[K, V]) {
	if len(add) == 0 {
		return
	}
	m.entries = mergeSortedEntries(base, add)
}

// DeleteAll removes the entries under every key in ks. Keys not present are
// ignored. Removal takes keys, never pairs (ADR 0017).
func (m *SortedDict[K, V]) DeleteAll(ks ...K) {
	if m.entries == nil {
		// Forces the nil-receiver panic when ks is empty.
		return
	}
	for _, k := range ks {
		if i, found := m.find(k); found {
			m.entries = slices.Delete(m.entries, i, i+1)
		}
	}
}

func collectSortedEntries[K cmp.Ordered, V any](seq iter.Seq2[K, V], sizeHint int) []sortedEntry[K, V] {
	var es []sortedEntry[K, V]
	if sizeHint > 0 {
		es = make([]sortedEntry[K, V], 0, sizeHint)
	}
	for k, v := range seq {
		es = append(es, sortedEntry[K, V]{k: k, v: v})
	}
	return sortDistinctEntries(es)
}

// sortDistinctEntries sorts es by key and keeps the LAST entry of each run of
// equal keys.
//
// slices.CompactFunc keeps the FIRST, which would silently diverge from
// repeated Set and from ADR 0004's last-write-wins rule. The sort is stable, so
// a run preserves the order the entries arrived in. Tests covering this must
// use distinguishable values, or they pass either way.
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
			out = append(out, add[j]) // add replaces base
			i++
			j++
		}
	}
	out = append(out, base[i:]...)
	return append(out, add[j:]...)
}
