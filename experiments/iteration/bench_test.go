package iteration

import "testing"

var sinkInt int

const n = 1024

func fixture() []int {
	s := make([]int, n)
	for i := range s {
		s[i] = i
	}
	return s
}

func BenchmarkNoOp(b *testing.B) {
	for b.Loop() {
		sinkInt++
	}
}

// ---- 1. the floor, and what an iterator costs over it --------------------

func BenchmarkForward(b *testing.B) {
	s := fixture()

	b.Run("RawRange", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for _, v := range s {
				t += v
			}
			sinkInt = t
		}
	})
	b.Run("NativeValues", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for v := range nativeValues(s) {
				t += v
			}
			sinkInt = t
		}
	})
	b.Run("NativeAll", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for _, v := range nativeAll(s) {
				t += v
			}
			sinkInt = t
		}
	})
}

// ---- 2. what deriving one shape from another costs -----------------------
//
// The choice between giving every shape its own method and deriving the extras
// with free functions, as maps.Keys does.

func BenchmarkDerived(b *testing.B) {
	s := fixture()

	b.Run("Values/Native", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for v := range nativeValues(s) {
				t += v
			}
			sinkInt = t
		}
	})
	b.Run("Values/DerivedFromAll", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for v := range derivedValues(nativeAll(s)) {
				t += v
			}
			sinkInt = t
		}
	})
	b.Run("Keys/DerivedFromAll", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for k := range derivedKeys(nativeAll(s)) {
				t += k
			}
			sinkInt = t
		}
	})
	b.Run("All/Native", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for i, v := range nativeAll(s) {
				t += i + v
			}
			sinkInt = t
		}
	})
	b.Run("All/DerivedFromValues", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for i, v := range derivedAll(nativeValues(s)) {
				t += i + v
			}
			sinkInt = t
		}
	})
}

// ---- 3. what reverse costs -----------------------------------------------

func BenchmarkBackward(b *testing.B) {
	s := fixture()

	b.Run("Forward/Native", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for _, v := range nativeAll(s) {
				t += v
			}
			sinkInt = t
		}
	})
	b.Run("Backward/Native", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for _, v := range nativeBackward(s) {
				t += v
			}
			sinkInt = t
		}
	})
	b.Run("BackwardValues/Native", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for v := range nativeValuesBackward(s) {
				t += v
			}
			sinkInt = t
		}
	})
	// A caller who only has a forward iterator must buffer to reverse it.
	b.Run("Backward/DerivedMustBuffer", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for v := range derivedBackward(nativeValues(s)) {
				t += v
			}
			sinkInt = t
		}
	})
}
