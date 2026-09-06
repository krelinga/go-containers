package dispatch

import (
	"iter"
	"maps"
	"testing"
)

type Map[K comparable, V any] map[K]V

func (m Map[K, V]) Get(k K) (V, bool)    { v, ok := m[k]; return v, ok }
func (m Map[K, V]) Set(k K, v V)         { m[k] = v }
func (m Map[K, V]) DelQuiet(k K)         { delete(m, k) }
func (m Map[K, V]) DelReport(k K) bool   { _, ok := m[k]; delete(m, k); return ok }
func (m Map[K, V]) Len() int             { return len(m) }
func (m Map[K, V]) All() iter.Seq2[K, V] { return maps.All(m) }

type MapIface[K comparable, V any] interface {
	Get(K) (V, bool)
	Set(K, V)
	DelQuiet(K)
	DelReport(K) bool
	Len() int
}

// Held in a package-level slice so the compiler cannot see the concrete type
// and devirtualise the call. Without this the "iface" numbers are fiction.
var ifaces []MapIface[int, int]

var (
	sinkB bool
	sinkI int
)

const n = 1024

func fresh() Map[int, int] {
	m := make(Map[int, int], n)
	for i := range n {
		m[i] = i
	}
	return m
}

// generic over a type parameter constrained by the interface.
func genDelQuiet[M MapIface[K, V], K comparable, V any](m M, k K) { m.DelQuiet(k) }
func genDelReport[M MapIface[K, V], K comparable, V any](m M, k K) bool {
	return m.DelReport(k)
}
func genGet[M MapIface[K, V], K comparable, V any](m M, k K) (V, bool) { return m.Get(k) }

func BenchmarkDelete(b *testing.B) {
	m := fresh()
	ifaces = []MapIface[int, int]{m}
	iface := ifaces[0]
	const miss = 999999

	b.Run("raw-builtin", func(b *testing.B) {
		for b.Loop() {
			delete(m, miss)
		}
	})
	b.Run("direct/quiet", func(b *testing.B) {
		for b.Loop() {
			m.DelQuiet(miss)
		}
	})
	b.Run("direct/report", func(b *testing.B) {
		for b.Loop() {
			sinkB = m.DelReport(miss)
		}
	})
	b.Run("iface/quiet", func(b *testing.B) {
		for b.Loop() {
			iface.DelQuiet(miss)
		}
	})
	b.Run("iface/report", func(b *testing.B) {
		for b.Loop() {
			sinkB = iface.DelReport(miss)
		}
	})
	b.Run("generic/quiet", func(b *testing.B) {
		for b.Loop() {
			genDelQuiet(m, miss)
		}
	})
	b.Run("generic/report", func(b *testing.B) {
		for b.Loop() {
			sinkB = genDelReport(m, miss)
		}
	})
}

func BenchmarkGet(b *testing.B) {
	m := fresh()
	ifaces = []MapIface[int, int]{m}
	iface := ifaces[0]

	b.Run("raw-builtin", func(b *testing.B) {
		for b.Loop() {
			sinkI = m[512]
		}
	})
	b.Run("direct", func(b *testing.B) {
		for b.Loop() {
			sinkI, sinkB = m.Get(512)
		}
	})
	b.Run("iface", func(b *testing.B) {
		for b.Loop() {
			sinkI, sinkB = iface.Get(512)
		}
	})
	b.Run("generic", func(b *testing.B) {
		for b.Loop() {
			sinkI, sinkB = genGet(m, 512)
		}
	})
}

// BenchmarkNoOp is the zero-cost baseline: it detects harness overhead leaking
// into the numbers above. It must stay well under the smallest measurement.
func BenchmarkNoOp(b *testing.B) {
	m := fresh()
	for b.Loop() {
		sinkI = len(m)
	}
}

// BenchmarkSetPath repeats the comparison for a write, where the map operation
// is heavier and dispatch should matter proportionally less.
func BenchmarkSet(b *testing.B) {
	m := fresh()
	ifaces = []MapIface[int, int]{m}
	iface := ifaces[0]

	b.Run("raw-builtin", func(b *testing.B) {
		for b.Loop() {
			m[512] = 1
		}
	})
	b.Run("direct", func(b *testing.B) {
		for b.Loop() {
			m.Set(512, 1)
		}
	})
	b.Run("iface", func(b *testing.B) {
		for b.Loop() {
			iface.Set(512, 1)
		}
	})
}
