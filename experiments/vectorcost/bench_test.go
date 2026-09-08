package vectorcost

import (
	"slices"
	"testing"
)

var (
	sinkInt   int
	sinkSlice []int
	sinkVec   *Vector[int]
)

const n = 1024

func fixture() ([]int, *Vector[int]) {
	s := make([]int, n)
	for i := range s {
		s[i] = i
	}
	return s, NewVector(s...)
}

func BenchmarkNoOp(b *testing.B) {
	for b.Loop() {
		sinkInt++
	}
}

// ---- 1. iteration: the operation a sequence exists for -------------------

func BenchmarkSum(b *testing.B) {
	s, v := fixture()

	// The floor: ranging a slice directly.
	b.Run("Slice/Range", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for _, x := range s {
				t += x
			}
			sinkInt = t
		}
	})
	b.Run("Slice/Index", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for i := 0; i < len(s); i++ {
				t += s[i]
			}
			sinkInt = t
		}
	})
	// Through the wrapper's index accessors: this is where bounds-check
	// elimination is at risk.
	b.Run("Vector/Index", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for i := 0; i < v.Len(); i++ {
				t += v.At(i)
			}
			sinkInt = t
		}
	})
	// Through iterators, which is what the contracts speak.
	b.Run("Slice/ValuesSeq", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for x := range slices.Values(s) {
				t += x
			}
			sinkInt = t
		}
	})
	b.Run("Vector/AllSeq", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for x := range v.All() {
				t += x
			}
			sinkInt = t
		}
	})
	b.Run("Vector/AllIndexedSeq", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for _, x := range v.AllIndexed() {
				t += x
			}
			sinkInt = t
		}
	})
}

// ---- 2. a single element read, isolated ----------------------------------

func BenchmarkAt(b *testing.B) {
	s, v := fixture()
	probe := n / 2

	b.Run("Slice", func(b *testing.B) {
		for b.Loop() {
			sinkInt = s[probe]
		}
	})
	b.Run("Vector", func(b *testing.B) {
		for b.Loop() {
			sinkInt = v.At(probe)
		}
	})
}

// ---- 3. append, with and without a reallocation --------------------------

func BenchmarkAppend(b *testing.B) {
	b.Run("Slice/Growing", func(b *testing.B) {
		s := []int(nil)
		for b.Loop() {
			s = append(s, 1)
			if len(s) > n {
				s = s[:0]
			}
		}
		sinkSlice = s
	})
	b.Run("Vector/Growing", func(b *testing.B) {
		v := &Vector[int]{}
		for b.Loop() {
			v.Append(1)
			if v.Len() > n {
				v.es = v.es[:0]
			}
		}
		sinkVec = v
	})
	// Presized, so no reallocation happens in the measured region.
	b.Run("Slice/Presized", func(b *testing.B) {
		s := make([]int, 0, n+1)
		for b.Loop() {
			s = append(s, 1)
			if len(s) > n {
				s = s[:0]
			}
		}
		sinkSlice = s
	})
	b.Run("Vector/Presized", func(b *testing.B) {
		v := &Vector[int]{es: make([]int, 0, n+1)}
		for b.Loop() {
			v.Append(1)
			if v.Len() > n {
				v.es = v.es[:0]
			}
		}
		sinkVec = v
	})
}

// ---- 4. what the wrapper buys: reallocation stops being observable -------

func BenchmarkReportAliasing(b *testing.B) {
	// A slice header is a value. Two appends off the same header with spare
	// capacity write the same backing cell.
	base := make([]int, 1, 10)
	base[0] = 0
	x := append(base, 1)
	y := append(base, 2)
	b.Logf("slice: two appends off one header -> x[1]=%d y[1]=%d  aliased=%v",
		x[1], y[1], x[1] == y[1])

	// And an append inside a callee is invisible to the caller unless returned.
	s := make([]int, 0, 10)
	appendSlice(s, 42)
	b.Logf("slice: callee appended, caller sees Len=%d  (the append is lost)", len(s))

	// A Vector owns its slice behind a pointer, so neither happens.
	v := NewVector(0)
	appendVector(v, 42)
	b.Logf("vector: callee appended, caller sees Len=%d", v.Len())

	// Growth across a reallocation stays invisible: every holder sees it.
	v2 := &Vector[int]{es: make([]int, 0, 1)}
	alias := v2
	for i := range 8 {
		v2.Append(i) // reallocates several times
	}
	b.Logf("vector: after %d reallocating appends, an aliasing holder sees Len=%d",
		8, alias.Len())

	for b.Loop() {
		sinkInt++
	}
}

//go:noinline
func appendSlice(s []int, v int) { s = append(s, v); _ = s }

//go:noinline
func appendVector(v *Vector[int], e int) { v.Append(e) }

// ---- 5. is the append cost the container, or the variadic signature? -----

func BenchmarkAppendOne(b *testing.B) {
	b.Run("Slice", func(b *testing.B) {
		s := make([]int, 0, n+1)
		for b.Loop() {
			s = append(s, 1)
			if len(s) > n {
				s = s[:0]
			}
		}
		sinkSlice = s
	})
	b.Run("VectorVariadic", func(b *testing.B) {
		v := &Vector[int]{es: make([]int, 0, n+1)}
		for b.Loop() {
			v.Append(1)
			if v.Len() > n {
				v.es = v.es[:0]
			}
		}
		sinkVec = v
	})
	b.Run("VectorNonVariadic", func(b *testing.B) {
		v := &Vector[int]{es: make([]int, 0, n+1)}
		for b.Loop() {
			v.AppendOne(1)
			if v.Len() > n {
				v.es = v.es[:0]
			}
		}
		sinkVec = v
	})
}
