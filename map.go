package containers

import (
	"iter"
	"maps"
)

// Map is the map-backed key/value container.
//
// It is a defined map[K]V, so it is already a reference type with exactly the
// semantics ADR 0018 gives every container: copies share, the zero value reads
// as empty, and writing to it panics. Under that ADR it stops being an adapter
// and becomes the default (HashDict is gone), because the value/pointer
// asymmetry ADR 0007 recorded -- the only thing keeping the two apart -- no
// longer exists.
//
// Being a map, it keeps three things nothing else has: builtin syntax, a free
// conversion to and from map[K]V, and working encoding/json.
//
//	m := Map[string, int]{"a": 1}
//	m2 := Map[string, int](existingMap)
type Map[K comparable, V any] map[K]V

// IsZero reports whether m was ever constructed. Map additionally supports
// m == nil, being a map; IsZero is what every container spells.
func (m Map[K, V]) IsZero() bool { return m == nil }

// Get returns the value stored under k.
func (m Map[K, V]) Get(k K) (V, bool) {
	v, ok := m[k]
	return v, ok
}

// Has reports whether k is present.
func (m Map[K, V]) Has(k K) bool {
	_, ok := m[k]
	return ok
}

// Set writes one entry. Writing to a zero Map panics, as the builtin does.
func (m Map[K, V]) Set(k K, v V) { m[k] = v }

// SetAll writes every entry of kvs, last occurrence winning for a repeated key
// (ADR 0004). kvs is copied and not retained.
func (m Map[K, V]) SetAll(kvs ...Entry[K, V]) {
	for _, kv := range kvs {
		m[kv.Slot] = kv.Value
	}
}

// Delete removes the entry under k, or does nothing if it is absent.
func (m Map[K, V]) Delete(k K) { delete(m, k) }

// DeleteAll removes the entries under every key in ks. Removal takes keys,
// never pairs (ADR 0017).
func (m Map[K, V]) DeleteAll(ks ...K) {
	for _, k := range ks {
		delete(m, k)
	}
}

// Len returns the number of entries.
func (m Map[K, V]) Len() int { return len(m) }

// Keys iterates the keys in the unspecified order a map range produces.
//
// This binds the live map, so a later write IS seen -- exactly as ranging a
// builtin map behaves. Do not modify a container while iterating it.
func (m Map[K, V]) Keys() iter.Seq[K] { return maps.Keys(m) }

// Values iterates the values in the unspecified order a map range produces.
func (m Map[K, V]) Values() iter.Seq[V] { return maps.Values(m) }

// All iterates the entries in the unspecified order a map range produces.
func (m Map[K, V]) All() iter.Seq2[K, V] { return maps.All(m) }

// KeySlice returns the keys as a new slice. The result is a full, independent
// copy (ADR 0017), which is what makes m.DeleteAll(m.KeySlice()...) safe.
func (m Map[K, V]) KeySlice() []K {
	if m == nil {
		return nil
	}
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ValueSlice returns the values as a new slice, one per key.
func (m Map[K, V]) ValueSlice() []V {
	if m == nil {
		return nil
	}
	out := make([]V, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// AllSlice returns the entries as a new slice of pairs. It is the
// bulk-transfer shape: NewSortedMap(m.AllSlice()...).
func (m Map[K, V]) AllSlice() []Entry[K, V] {
	if m == nil {
		return nil
	}
	out := make([]Entry[K, V], 0, len(m))
	for k, v := range m {
		out = append(out, Entry[K, V]{k, v})
	}
	return out
}

// Clone returns an independent copy.
func (m Map[K, V]) Clone() Map[K, V] { return maps.Clone(m) }
