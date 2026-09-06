package containers

import (
	"cmp"
	"iter"
	"slices"
)

type sortedEntry[K cmp.Ordered, V any] struct {
	k K
	v V
}

// A SortedMap is a map whose keys are kept in ascending order.
//
// The zero value is an empty SortedMap ready to use:
//
//	var m containers.SortedMap[int, string]
//	m.Set(10, "0.90")
//
// Like Set, every method has a pointer receiver, a nil *SortedMap panics on any
// method, and copying the struct shares the backing array — use Clone, and
// prefer a SortedMap field over a *SortedMap field. `go vet` reports struct
// copies, though `go test` does not run that check: use `go vet ./...` or
// `go test -vet=all`.
//
// # Performance
//
// A SortedMap is backed by a sorted slice, so lookup is O(log n), ordered
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
// A SortedMap is not safe for concurrent use.
type SortedMap[K cmp.Ordered, V any] struct {
	_       noCopy
	entries []sortedEntry[K, V]
}

// NewSortedMap returns an empty SortedMap. It is a convenience; the zero value
// is equally usable.
func NewSortedMap[K cmp.Ordered, V any]() *SortedMap[K, V] {
	return &SortedMap[K, V]{}
}

// find returns the index of k, or the index where it would be inserted.
func (m *SortedMap[K, V]) find(k K) (int, bool) {
	return slices.BinarySearchFunc(m.entries, k, func(e sortedEntry[K, V], k K) int {
		return cmp.Compare(e.k, k)
	})
}

// Set associates v with k, replacing any existing value.
func (m *SortedMap[K, V]) Set(k K, v V) {
	if i, found := m.find(k); found {
		m.entries[i].v = v
	} else {
		m.entries = slices.Insert(m.entries, i, sortedEntry[K, V]{k, v})
	}
}

// Get returns the value associated with k.
func (m *SortedMap[K, V]) Get(k K) (V, bool) {
	if i, found := m.find(k); found {
		return m.entries[i].v, true
	}
	var zero V
	return zero, false
}

// Delete removes k, reporting whether it was present.
func (m *SortedMap[K, V]) Delete(k K) bool {
	i, found := m.find(k)
	if !found {
		return false
	}
	m.entries = slices.Delete(m.entries, i, i+1)
	return true
}

// Len returns the number of entries.
func (m *SortedMap[K, V]) Len() int { return len(m.entries) }

// All returns an iterator over every entry in ascending key order.
//
// The iterator is bound to the map's contents as of the call to All, not as of
// iteration. Modifying the map during iteration is not supported.
func (m *SortedMap[K, V]) All() iter.Seq2[K, V] {
	es := m.entries // eager, so a nil receiver panics here like every other method
	return seq2Over(es)
}

// Range returns an iterator over the entries with lo <= key < hi, in ascending
// key order. If hi <= lo the range is empty.
//
// Bounds are resolved when Range is called, not when the result is iterated.
// The iterator does not expose the map's backing array; a caller cannot mutate
// the map through it.
func (m *SortedMap[K, V]) Range(lo, hi K) iter.Seq2[K, V] {
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
func (m *SortedMap[K, V]) Floor(k K) (K, V, bool) {
	i, found := m.find(k)
	if !found {
		i-- // find returned the insertion point; the entry before it is the floor
	}
	return m.at(i)
}

// Ceil returns the entry with the smallest key >= k.
func (m *SortedMap[K, V]) Ceil(k K) (K, V, bool) {
	i, _ := m.find(k) // exact index if present, else the first key greater than k
	return m.at(i)
}

// Min returns the entry with the smallest key.
func (m *SortedMap[K, V]) Min() (K, V, bool) { return m.at(0) }

// Max returns the entry with the largest key.
func (m *SortedMap[K, V]) Max() (K, V, bool) { return m.at(len(m.entries) - 1) }

// at returns the entry at i, reporting false if i is out of range. It is the
// single place the boundary conditions of the ordered lookups live.
func (m *SortedMap[K, V]) at(i int) (K, V, bool) {
	if i < 0 || i >= len(m.entries) {
		var zk K
		var zv V
		return zk, zv, false
	}
	e := m.entries[i]
	return e.k, e.v, true
}

// Clone returns an independent copy. Mutating the result does not affect m.
func (m *SortedMap[K, V]) Clone() *SortedMap[K, V] {
	return &SortedMap[K, V]{entries: slices.Clone(m.entries)}
}
