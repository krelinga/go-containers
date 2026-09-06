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

// NewSortedDict returns an empty SortedDict. It is a convenience; the zero value
// is equally usable.
func NewSortedDict[K cmp.Ordered, V any]() *SortedDict[K, V] {
	return &SortedDict[K, V]{}
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

// SetAll adds every entry in src, replacing existing values for keys already
// present. Where src yields the same key more than once, the last wins — the
// same result as calling Set for each entry in order.
//
// src.Len() sizes the working slice; it is a hint, and a wrong one costs only a
// worse allocation.
//
// SetAll sorts the input once and merges, which is O(k log k + n + k) for k entries
// against a map of n. Calling Set in a loop is O(kn), because each out-of-order
// insert memmoves half the backing array. experiments/bulkinsert measured the
// crossover at roughly four to sixteen entries, nearly independent of n, with
// SetAll 42x faster at n=100 000 and k=1 024. **Below that crossover Set is
// marginally cheaper**, so prefer Set when adding one or two entries; SetAll is
// the better choice for anything more.
//
// src is fully consumed before the map is modified, so m.SetAll(m) is well
// defined, if pointless.
func (m *SortedDict[K, V]) SetAll(src Elems2[K, V]) {
	base := m.entries              // eager, so a nil receiver panics even for an empty src
	seq, n := src.All(), src.Len() // eager, so a nil src panics too
	m.setAll(base, collectSortedEntries(seq, n))
}

// SetAllSeq is SetAll for a bare iterator, which cannot report its length.
// Prefer SetAll when the source knows it.
func (m *SortedDict[K, V]) SetAllSeq(seq iter.Seq2[K, V]) {
	base := m.entries // eager, so a nil receiver panics even when seq is empty
	m.setAll(base, collectSortedEntries(seq, 0))
}

func (m *SortedDict[K, V]) setAll(base, add []sortedEntry[K, V]) {
	if len(add) == 0 {
		return
	}
	m.entries = mergeSortedEntries(base, add)
}

// CollectSortedDict returns a SortedDict holding every entry in src. Where src
// yields the same key more than once, the last wins.
//
// src.Len() is used to size the working slice, so this allocates once where
// CollectSortedDictSeq must grow as it goes. The saving is allocation volume
// rather than wall-clock time: the sort that follows dominates. See ADR 0006.
//
// It is also cheaper than NewSortedDict followed by SetAll, since with nothing
// to merge against the merge degenerates to the sort alone.
func CollectSortedDict[K cmp.Ordered, V any](src Elems2[K, V]) *SortedDict[K, V] {
	return &SortedDict[K, V]{entries: collectSortedEntries(src.All(), src.Len())}
}

// CollectSortedDictSeq returns a SortedDict holding every entry in seq.
//
// Prefer CollectSortedDict when the source knows its length. Use this for bare
// iterators — maps.All, or another container's Range — which cannot report one.
func CollectSortedDictSeq[K cmp.Ordered, V any](seq iter.Seq2[K, V]) *SortedDict[K, V] {
	return &SortedDict[K, V]{entries: collectSortedEntries(seq, 0)}
}

// collectSortedEntries drains seq into a slice sorted by key, keeping the last
// entry for any repeated key. It preallocates to sizeHint when that is
// positive; the hint only affects allocation, never which entries are kept.
func collectSortedEntries[K cmp.Ordered, V any](seq iter.Seq2[K, V], sizeHint int) []sortedEntry[K, V] {
	var es []sortedEntry[K, V]
	if sizeHint > 0 {
		es = make([]sortedEntry[K, V], 0, sizeHint)
	}
	for k, v := range seq {
		es = append(es, sortedEntry[K, V]{k: k, v: v})
	}
	slices.SortStableFunc(es, func(a, b sortedEntry[K, V]) int { return cmp.Compare(a.k, b.k) })

	// Keep the LAST of each run of equal keys. slices.CompactFunc keeps the
	// first, which would silently diverge from repeated Set. The sort is stable,
	// so a run preserves the order seq yielded.
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
