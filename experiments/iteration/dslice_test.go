package iteration

import (
	"slices"
	"testing"
)

var (
	sinkDStrs []string
	sinkDSet  map[string]struct{}
	sinkDBig  []Big
	sinkDKV   []KeyValue[string, int]
)

func mkD(n int) (*udictD, *odictD) {
	ks := fixtureKeys(n)
	m := make(map[string]int, n)
	for i, k := range ks {
		m[k] = i
	}
	return &udictD{m}, &odictD{ks}
}

// 1. A map's keys into a vector. D must walk the map to build the slice and
// then copy it; A walks the map once through a yield closure. This is D's
// worst case.
func BenchmarkDMapKeysToVector(b *testing.B) {
	const n = 1024
	ua, _ := mkA(n)
	ud, _ := mkD(n)

	b.Run("A", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkDStrs = newVecA(KeysOfA[string](ua))
		}
	})
	b.Run("D", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkDStrs = newVecD(ud.KeySlice())
		}
	})
	b.Run("D_adopt", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkDStrs = newVecDAdopt(ud.KeySlice())
		}
	})
}

// 2. A slice-backed container into a vector. D is two memmoves; A is a yield
// call per element. This is D's best case.
func BenchmarkDSliceToVector(b *testing.B) {
	const n = 1024
	_, oa := mkA(n)
	_, od := mkD(n)

	b.Run("A", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkDStrs = newVecA(KeysOfA[string](oa))
		}
	})
	b.Run("D", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkDStrs = newVecD(od.KeySlice())
		}
	})
	b.Run("D_adopt", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkDStrs = newVecDAdopt(od.KeySlice())
		}
	})
}

// 3. Into a map-backed container, where the insert dominates both sides.
func BenchmarkDToSet(b *testing.B) {
	const n = 1024
	_, oa := mkA(n)
	_, od := mkD(n)

	b.Run("A", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkDSet = newSetA(KeysOfA[string](oa))
		}
	})
	b.Run("D", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkDSet = newSetD(od.KeySlice())
		}
	})
}

// 4. Wide values. D copies 1 KiB per element twice; A never copies the payload
// beyond what the yield itself costs. This is where D should hurt most.
func BenchmarkDWideValues(b *testing.B) {
	const n = 256 // 256 KiB of payload
	s := make([]Big, n)
	for i := range s {
		s[i][0] = int64(i)
	}
	va := &bigVecA{s}
	vd := &bigVecD{s}

	b.Run("A", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkDBig = newVecA(KeysOfA[Big](va))
		}
	})
	b.Run("D", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkDBig = newVecD(vd.ValueSlice())
		}
	})
	b.Run("D_adopt", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkDBig = newVecDAdopt(vd.ValueSlice())
		}
	})
}

// 5. Pairs. D materialises KeyValue structs; A yields two values per step.
func BenchmarkDPairs(b *testing.B) {
	const n = 1024
	ud, _ := mkD(n)
	b.Run("D_AllSlice", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkDKV = ud.AllSlice()
		}
	})
}

// 6. The common case: just walking a container's keys. A ranges an iter.Seq;
// D has to materialise first, which is the tax D pays on every read.
func BenchmarkDIterate(b *testing.B) {
	const n = 1024
	ua, _ := mkA(n)
	ud, _ := mkD(n)

	b.Run("A_seq", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range ua.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("D_slice", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range ud.KeySlice() {
				c++
			}
			sinkInt = c
		}
	})
}

// 7. Reverse. D owns its copy, so reversal is slices.Reverse in place; A needs
// a container-provided Backward.
func BenchmarkDReverse(b *testing.B) {
	const n = 1024
	_, od := mkD(n)
	b.Run("D_sliceReverse", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			ks := od.KeySlice()
			slices.Reverse(ks)
			sinkDStrs = ks
		}
	})
}
