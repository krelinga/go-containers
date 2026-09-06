package containers

import (
	"iter"
	"maps"
)

// A HashDict is an unordered map from keys to values.
//
// The zero value is an empty HashDict ready to use:
//
//	var d containers.HashDict[string, int]
//	d.Set("a", 1)
//
// Every method has a pointer receiver, a nil *HashDict panics on any method,
// and copying the struct shares the backing map — use Clone, and prefer a
// HashDict field over a *HashDict field. `go vet` reports struct copies, though
// `go test` does not run that check: use `go vet ./...` or `go test -vet=all`.
//
// # Choosing between HashDict and Map
//
// Both wrap a Go map. HashDict is the default: it behaves like every other
// container in this package. Map is `map[K]V` itself with methods attached, and
// is the right choice only when you need something that follows from that:
//
//   - wrapping an existing map[K]V with no copy, as a free type conversion
//   - passing the result where a map[K]V is expected
//   - builtin syntax — m[k], len(m), delete(m, k), range
//   - encoding/json, which round-trips a Map with string or integer keys and
//     marshals every struct-backed container here to {}
//
// Map is an adapter for those cases. HashDict is what to reach for otherwise.
//
// A HashDict is not safe for concurrent use.
type HashDict[K comparable, V any] struct {
	_ noCopy
	m map[K]V
}

// NewHashDict returns an empty HashDict. It is a convenience; the zero value is
// equally usable.
func NewHashDict[K comparable, V any]() *HashDict[K, V] {
	return &HashDict[K, V]{}
}

// CollectHashDict returns a HashDict holding every entry in src.
//
// src.Len() presizes the underlying map, which is worth considerably more here
// than the equivalent hint is for the sorted containers: experiments/hashdict
// measured presizing at 2.4x to 4.1x in time and about half the allocation
// volume. A sorted container's O(n log n) sort hides that saving; a hash dict
// has no sort, so it is the whole difference.
//
// Len is a hint. A src whose Len disagrees with its All produces a worse
// allocation, never a wrong result.
func CollectHashDict[K comparable, V any](src Elems2[K, V]) *HashDict[K, V] {
	n := src.Len()
	seq := src.All()
	d := &HashDict[K, V]{m: make(map[K]V, max(0, n))}
	for k, v := range seq {
		d.m[k] = v
	}
	return d
}

// CollectHashDictSeq returns a HashDict holding every entry in seq.
//
// Prefer CollectHashDict when the source knows its length — see its
// documentation for how much the hint is worth. Use this for bare iterators,
// which cannot report one.
func CollectHashDictSeq[K comparable, V any](seq iter.Seq2[K, V]) *HashDict[K, V] {
	d := &HashDict[K, V]{}
	d.setAll(seq, 0)
	return d
}

// Get returns the value stored under k.
func (d *HashDict[K, V]) Get(k K) (V, bool) {
	v, ok := d.m[k]
	return v, ok
}

// Set stores v under k, replacing any existing value.
func (d *HashDict[K, V]) Set(k K, v V) {
	if d.m == nil {
		d.m = make(map[K]V)
	}
	d.m[k] = v
}

// Delete removes k. Deleting an absent key is a no-op.
//
// It reports nothing, matching Map.Delete and SortedDict.Delete so that all
// three satisfy MutableDict. Use Get first if you need to know whether the key
// was present.
func (d *HashDict[K, V]) Delete(k K) { delete(d.m, k) }

// Len returns the number of entries.
func (d *HashDict[K, V]) Len() int { return len(d.m) }

// All returns an iterator over the entries, in the unspecified order a map
// range produces.
//
// Modifying the dict during iteration is not supported; see MutableDict.
func (d *HashDict[K, V]) All() iter.Seq2[K, V] {
	m := d.m // read eagerly, so a nil receiver panics here like every other method
	return maps.All(m)
}

// Clone returns an independent copy. Mutating the result does not affect d.
func (d *HashDict[K, V]) Clone() *HashDict[K, V] {
	// maps.Clone rather than make plus a copy: it duplicates the hash table
	// instead of re-hashing every key, which experiments/hashdict measured at
	// 3-4x faster.
	return &HashDict[K, V]{m: maps.Clone(d.m)}
}

// SetAll adds every entry in src, replacing existing values for keys already
// present. Where src yields the same key more than once, the last wins.
//
// Unlike SortedDict.SetAll, this has no algorithmic advantage over calling Set
// in a loop. It inserts one entry at a time, because Go exposes no growth hint
// for a map that already exists; src.Len() presizes only when the receiver is
// still at its zero value. Rebuilding the map to presize it was measured in
// experiments/hashdict and rejected — it only pays when the additions outnumber
// the existing entries several times over. SetAll exists so that every
// MutableDict implementation offers the same operations, not to be faster.
//
// src is fully consumed as it is applied.
func (d *HashDict[K, V]) SetAll(src Elems2[K, V]) {
	n := src.Len() // eager, so a nil src panics
	seq := src.All()
	d.setAll(seq, n)
}

// SetAllSeq is SetAll for a bare iterator, which cannot report its length.
func (d *HashDict[K, V]) SetAllSeq(seq iter.Seq2[K, V]) {
	d.setAll(seq, 0)
}

func (d *HashDict[K, V]) setAll(seq iter.Seq2[K, V], sizeHint int) {
	// Touches the receiver before consuming seq, so a nil receiver panics even
	// when seq is empty.
	if d.m == nil {
		d.m = make(map[K]V, max(0, sizeHint))
	}
	for k, v := range seq {
		d.m[k] = v
	}
}
