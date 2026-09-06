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

// ---------------------------------------------------------------------------
// Can SetAll presize after all, by rebuilding?
//
// Go exposes no growth hint for an existing map, so a bulk insert into a
// non-empty dict cannot presize -- it inserts into a map that grows and rehashes
// as it goes. A struct-backed dict has an option a defined map type does not:
// allocate a new map at len(existing)+len(added), copy the existing entries in,
// then insert. It pays an O(n) copy to buy k inserts that never trigger growth.
//
// Measuring this fairly is awkward because the naive path mutates in place while
// the rebuild path does not, and an in-place benchmark would have to restore its
// fixture inside the timed loop. So all three variants below PRODUCE a map,
// leaving the fixture pristine, and cloneOnly isolates the copy that naive and
// rebuild both perform. In-place naive is then (naive - cloneOnly), and the
// comparison that matters is rebuild against that.
// ---------------------------------------------------------------------------

var mergeN = []int{64, 1024, 16384}
var mergeK = []int{1, 16, 256, 4096}

func baseOf(n int) map[int]int {
	m := make(map[int]int, n)
	for i := range n {
		m[i] = i
	}
	return m
}

// addsOf returns k keys that do not collide with a base of size n.
func addsOf(n, k int) []int {
	ks := make([]int, k)
	for i := range k {
		ks[i] = n + i
	}
	return ks
}

func BenchmarkMerge(b *testing.B) {
	for _, n := range mergeN {
		base := baseOf(n)
		for _, k := range mergeK {
			adds := addsOf(n, k)

			// The copy both other variants perform; subtract to get in-place naive.
			b.Run(fmt.Sprintf("cloneOnly/n=%d/k=%d", n, k), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					sinkM = maps.Clone(base)
				}
			})

			// What SetAll does today: insert into a map that grows as it goes.
			b.Run(fmt.Sprintf("naive/n=%d/k=%d", n, k), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					m := maps.Clone(base)
					for _, key := range adds {
						m[key] = key
					}
					sinkM = m
				}
			})

			// The proposal: presize to n+k, copy, then insert without growth.
			b.Run(fmt.Sprintf("rebuild/n=%d/k=%d", n, k), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					m := make(map[int]int, len(base)+len(adds))
					maps.Copy(m, base)
					for _, key := range adds {
						m[key] = key
					}
					sinkM = m
				}
			})
		}
	}
}

// BenchmarkCopyMechanism isolates a confound found while measuring the rebuild
// strategy: maps.Clone and make+maps.Copy are not equivalent. Clone can
// duplicate the hash table structure wholesale, while Copy must re-insert and
// re-hash every key. A rebuild cannot use Clone, because Clone allocates its own
// map and there is no way to tell it the target size -- so choosing to presize
// forces the more expensive copy.
func BenchmarkCopyMechanism(b *testing.B) {
	for _, n := range []int{64, 1024, 16384} {
		base := baseOf(n)
		b.Run(fmt.Sprintf("mapsClone/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkM = maps.Clone(base)
			}
		})
		b.Run(fmt.Sprintf("makeAndCopy/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				m := make(map[int]int, n)
				maps.Copy(m, base)
				sinkM = m
			}
		})
		b.Run(fmt.Sprintf("makeAndLoop/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				m := make(map[int]int, n)
				for k, v := range base {
					m[k] = v
				}
				sinkM = m
			}
		})
	}
}
