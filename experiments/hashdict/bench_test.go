package hashdict

import (
	"fmt"
	"iter"
	"maps"
	"testing"
)

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// The proposed type: a struct following ADR 0002's shape rules.
type HashDict[K comparable, V any] struct {
	_ noCopy
	m map[K]V
}

func (h *HashDict[K, V]) Set(k K, v V) {
	if h.m == nil {
		h.m = make(map[K]V)
	}
	h.m[k] = v
}
func (h *HashDict[K, V]) Get(k K) (V, bool) { v, ok := h.m[k]; return v, ok }
func (h *HashDict[K, V]) Delete(k K)        { delete(h.m, k) }
func (h *HashDict[K, V]) Len() int          { return len(h.m) }
func (h *HashDict[K, V]) All() iter.Seq2[K, V] {
	m := h.m
	return maps.All(m)
}

// Today's type: a defined map type.
type Map[K comparable, V any] map[K]V

func (m Map[K, V]) Set(k K, v V)      { m[k] = v }
func (m Map[K, V]) Get(k K) (V, bool) { v, ok := m[k]; return v, ok }
func (m Map[K, V]) Delete(k K)        { delete(m, k) }
func (m Map[K, V]) Len() int          { return len(m) }

var (
	sinkI int
	sinkB bool
)

const n = 1024

func BenchmarkOps(b *testing.B) {
	raw := make(map[int]int, n)
	for i := range n {
		raw[i] = i
	}
	dm := Map[int, int](raw)
	hm := &HashDict[int, int]{m: raw}

	b.Run("Get/raw", func(b *testing.B) {
		for b.Loop() {
			sinkI = raw[512]
		}
	})
	b.Run("Get/Map", func(b *testing.B) {
		for b.Loop() {
			sinkI, sinkB = dm.Get(512)
		}
	})
	b.Run("Get/HashDict", func(b *testing.B) {
		for b.Loop() {
			sinkI, sinkB = hm.Get(512)
		}
	})

	b.Run("Set/raw", func(b *testing.B) {
		for b.Loop() {
			raw[512] = 1
		}
	})
	b.Run("Set/Map", func(b *testing.B) {
		for b.Loop() {
			dm.Set(512, 1)
		}
	})
	b.Run("Set/HashDict", func(b *testing.B) {
		for b.Loop() {
			hm.Set(512, 1)
		}
	})

	b.Run("Delete/raw", func(b *testing.B) {
		for b.Loop() {
			delete(raw, 999999)
		}
	})
	b.Run("Delete/Map", func(b *testing.B) {
		for b.Loop() {
			dm.Delete(999999)
		}
	})
	b.Run("Delete/HashDict", func(b *testing.B) {
		for b.Loop() {
			hm.Delete(999999)
		}
	})
}

var sinkM map[int]int

// Does presizing a map from a known length pay, the way preallocating a slice
// did in experiments/sizedcollect?
func BenchmarkPresize(b *testing.B) {
	for _, n := range []int{16, 256, 4096, 65536} {
		b.Run(fmt.Sprintf("unsized/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				m := make(map[int]int)
				for i := range n {
					m[i] = i
				}
				sinkM = m
			}
		})
		b.Run(fmt.Sprintf("sized/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				m := make(map[int]int, n)
				for i := range n {
					m[i] = i
				}
				sinkM = m
			}
		})
	}
}

// BenchmarkNoOp is the zero-cost baseline: it detects harness overhead leaking
// into the numbers above.
func BenchmarkNoOp(b *testing.B) {
	raw := map[int]int{1: 1}
	for b.Loop() {
		sinkI = len(raw)
	}
}
