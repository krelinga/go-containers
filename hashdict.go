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

// NewHashDict returns a HashDict holding kvs. The zero value is equally usable.
//
// Spread a slice to build from another container:
//
//	NewHashDict(other.AllSlice()...)
//
// len(kvs) presizes the underlying map, which is worth considerably more here
// than the equivalent hint is for the sorted containers: experiments/hashdict
// measured presizing at 2.4x to 4.1x in time and about half the allocation
// volume. A sorted container's O(n log n) sort hides that saving; a hash dict
// has no sort, so it is the whole difference.
//
// kvs is copied and not retained.
func NewHashDict[K comparable, V any](kvs ...KeyValue[K, V]) *HashDict[K, V] {
	d := &HashDict[K, V]{m: make(map[K]V, len(kvs))}
	for _, kv := range kvs {
		d.m[kv.Key] = kv.Value
	}
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

// Keys returns an iterator over the keys, in the unspecified order a map range
// produces.
func (d *HashDict[K, V]) Keys() iter.Seq[K] {
	m := d.m // read eagerly, so a nil receiver panics here like every other method
	return maps.Keys(m)
}

// Values returns an iterator over the values, in the unspecified order a map
// range produces.
func (d *HashDict[K, V]) Values() iter.Seq[V] {
	m := d.m
	return maps.Values(m)
}

// All returns an iterator over the entries, in the unspecified order a map
// range produces.
//
// Modifying the dict during iteration is not supported; see MutableDict.
func (d *HashDict[K, V]) All() iter.Seq2[K, V] {
	m := d.m
	return maps.All(m)
}

// KeySlice returns the keys as a new slice, in no particular order.
//
// The result is a full, independent copy (ADR 0017): nothing the dict does
// afterwards is visible through it, and nothing done to it is visible in the
// dict. That is what makes d.DeleteAll(d.KeySlice()...) safe.
func (d *HashDict[K, V]) KeySlice() []K {
	out := make([]K, 0, len(d.m))
	for k := range d.m {
		out = append(out, k)
	}
	return out
}

// ValueSlice returns the values as a new slice, in no particular order.
// Duplicate values are preserved; the result has one entry per key.
func (d *HashDict[K, V]) ValueSlice() []V {
	out := make([]V, 0, len(d.m))
	for _, v := range d.m {
		out = append(out, v)
	}
	return out
}

// AllSlice returns the entries as a new slice of pairs, in no particular order.
// It is the bulk-transfer shape: NewHashDict(d.AllSlice()...).
func (d *HashDict[K, V]) AllSlice() []KeyValue[K, V] {
	out := make([]KeyValue[K, V], 0, len(d.m))
	for k, v := range d.m {
		out = append(out, KeyValue[K, V]{k, v})
	}
	return out
}

// Clone returns an independent copy. Mutating the result does not affect d.
func (d *HashDict[K, V]) Clone() *HashDict[K, V] {
	// maps.Clone rather than make plus a copy: it duplicates the hash table
	// instead of re-hashing every key, which experiments/hashdict measured at
	// 3-4x faster.
	return &HashDict[K, V]{m: maps.Clone(d.m)}
}

// SetAll writes every entry of kvs, replacing existing values for keys already
// present. Where kvs repeats a key, the last occurrence wins (ADR 0004).
//
// Spread a slice to bulk-write:
//
//	d.SetAll(other.AllSlice()...)
//
// Unlike SortedDict.SetAll, this has no algorithmic advantage over calling Set
// in a loop. It inserts one entry at a time, because Go exposes no growth hint
// for a map that already exists; len(kvs) presizes only when the receiver is
// still at its zero value. Rebuilding the map to presize it was measured in
// experiments/hashdict and rejected — it only pays when the additions outnumber
// the existing entries several times over.
//
// kvs is copied and not retained.
func (d *HashDict[K, V]) SetAll(kvs ...KeyValue[K, V]) {
	// Touches the receiver before consuming kvs, so a nil receiver panics even
	// when kvs is empty.
	if d.m == nil {
		d.m = make(map[K]V, len(kvs))
	}
	for _, kv := range kvs {
		d.m[kv.Key] = kv.Value
	}
}

// DeleteAll removes the entries under every key in ks. Keys not present are
// ignored. Removal takes keys, never pairs, because a set's element is its key
// and this makes the signature identical across sets and dicts (ADR 0017).
func (d *HashDict[K, V]) DeleteAll(ks ...K) {
	if d.m == nil {
		// Forces the nil-receiver panic when ks is empty.
		return
	}
	for _, k := range ks {
		delete(d.m, k)
	}
}
