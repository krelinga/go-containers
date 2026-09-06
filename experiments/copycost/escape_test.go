package copycost

import (
	"slices"
	"testing"
)

// The three functions below are identical in observable behaviour and differ
// only in how the copy's length is known. `go build -gcflags=-m` reports that
// none of the copies escape — but "does not escape" only permits stack
// allocation, it does not guarantee it. A copy whose length is not a
// compile-time constant must still go to the heap, because the compiler cannot
// reserve a frame of unknown size.

//go:noinline
func sumConstArr(src []int) int {
	var c [32]int // constant size: stack
	copy(c[:], src)
	t := 0
	for _, v := range c {
		t += v
	}
	return t
}

//go:noinline
func sumVarLen(src []int) int {
	c := make([]int, len(src)) // dynamic size: heap, despite not escaping
	copy(c, src)
	t := 0
	for _, v := range c {
		t += v
	}
	return t
}

//go:noinline
func sumClone(src []int) int {
	c := slices.Clone(src) // same, via the stdlib helper
	t := 0
	for _, v := range c {
		t += v
	}
	return t
}

// BenchmarkEscape compares the three over identical 32-element input. The
// const-size variant should report 0 allocs; the other two should not.
func BenchmarkEscape(b *testing.B) {
	src := make([]int, 32)

	b.Run("const-size-array", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkScalar = sumConstArr(src)
		}
	})
	b.Run("var-len-make", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkScalar = sumVarLen(src)
		}
	})
	b.Run("slices.Clone", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkScalar = sumClone(src)
		}
	})
}
