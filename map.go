package containers

import (
	"iter"
	"maps"
)

// A Map is a map[K]V with methods, so that it can satisfy the interfaces in
// this package and be used from generic code.
//
// # Prefer HashDict unless you need what only Map can do
//
// Map is an adapter, not the default hash dict. It cannot carry a noCopy guard
// or a usable zero value, because it is a map rather than a struct holding one,
// and it satisfies interfaces as a value where every other container does so
// through a pointer. HashDict is the type that behaves like the rest of this
// package. Reach for Map when you specifically need a free conversion from an
// existing map, builtin syntax, passing the result where a map[K]V is expected,
// or encoding/json — see below.
//
//	m := containers.Map[string, int](existing)
//
// That conversion is not a copy: a Map aliases the map it wraps, and mutations
// flow both ways. Wrapping costs nothing, and the methods are inlined away —
// experiments/dispatch measured Get at 3.28ns against 3.20ns for the builtin.
//
// # Semantics are the builtin's, not this package's
//
// Map is a defined map type, so it behaves like a map rather than like the
// struct-backed containers here. ADR 0002's shape rules do not apply:
//
//   - Methods take value receivers, so a Map value satisfies the interfaces
//     directly, where the other containers require a pointer.
//   - There is no noCopy guard, and none is needed. Copying a Map shares the
//     underlying map, exactly as copying a map[K]V does — no illusion of value
//     semantics to protect against.
//   - The zero value reads correctly (Len is 0, Get misses, All yields nothing)
//     and **panics on write**, with "assignment to entry in nil map". Use
//     make or a conversion before writing, as with any map.
//
// Builtin syntax still works: m[k], len(m), delete(m, k) and range all behave
// as they do on the underlying map.
//
// Mutating a Map during iteration over All is permitted by the Go spec for
// deletion, but MutableDict does not promise it — see that interface.
//
// A Map is not safe for concurrent use.
type Map[K comparable, V any] map[K]V

// Get returns the value stored under k.
func (m Map[K, V]) Get(k K) (V, bool) {
	v, ok := m[k]
	return v, ok
}

// Set stores v under k. It panics if m is nil, as a map assignment does.
func (m Map[K, V]) Set(k K, v V) { m[k] = v }

// Delete removes k. Deleting an absent key, or from a nil Map, is a no-op.
//
// It reports nothing, matching SortedDict.Delete. The builtin delete does not
// say whether the key was present, so reporting it would cost an extra lookup —
// measured at +62% per delete in experiments/dispatch, which is several times
// what generic dispatch itself costs. Use Get first if you need to know.
func (m Map[K, V]) Delete(k K) { delete(m, k) }

// Len returns the number of entries.
func (m Map[K, V]) Len() int { return len(m) }

// All returns an iterator over the entries, in the unspecified order the
// builtin range produces.
func (m Map[K, V]) All() iter.Seq2[K, V] { return maps.All(m) }

// Keys returns an iterator over the keys, in the unspecified order a map range
// produces.
func (m Map[K, V]) Keys() iter.Seq[K] { return maps.Keys(m) }

// Values returns an iterator over the values, in the unspecified order a map
// range produces.
func (m Map[K, V]) Values() iter.Seq[V] { return maps.Values(m) }

// KeySlice returns the keys as a new slice, in no particular order. The result
// is a full, independent copy (ADR 0017).
func (m Map[K, V]) KeySlice() []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ValueSlice returns the values as a new slice, in no particular order.
func (m Map[K, V]) ValueSlice() []V {
	out := make([]V, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// AllSlice returns the entries as a new slice of pairs, in no particular order.
func (m Map[K, V]) AllSlice() []KeyValue[K, V] {
	out := make([]KeyValue[K, V], 0, len(m))
	for k, v := range m {
		out = append(out, KeyValue[K, V]{k, v})
	}
	return out
}

// SetAll writes every entry of kvs, replacing existing values for keys already
// present. Where kvs repeats a key, the last occurrence wins (ADR 0004).
//
// Map has no constructor — a composite literal and make already build one,
// which is the point of the type (ADR 0009) — but it is a bulk target like
// every other dict:
//
//	m.SetAll(other.AllSlice()...)
//
// Map is a defined map type with value receivers, so it follows the builtin's
// nil semantics rather than ADR 0002's: writing to a nil Map panics only once
// there is something to write, so SetAll with no arguments is a no-op on one.
func (m Map[K, V]) SetAll(kvs ...KeyValue[K, V]) {
	for _, kv := range kvs {
		m[kv.Key] = kv.Value
	}
}

// DeleteAll removes the entries under every key in ks. Keys not present are
// ignored. As with the builtin delete, doing this to a nil Map is a no-op.
func (m Map[K, V]) DeleteAll(ks ...K) {
	for _, k := range ks {
		delete(m, k)
	}
}

// Clone returns a shallow copy. The result does not alias m.
func (m Map[K, V]) Clone() Map[K, V] { return maps.Clone(m) }
