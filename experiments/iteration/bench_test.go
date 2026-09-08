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

// ---- 4. does element size change the derived-is-free finding? ------------

const nBig = 256

func bigFixture() []Big {
	s := make([]Big, nBig)
	for i := range s {
		s[i][0] = int64(i)
	}
	return s
}

var sinkI64 int64

func BenchmarkElementSize(b *testing.B) {
	small := fixture()  // []int, 8 B elements
	big := bigFixture() // []Big, 1024 B elements

	// Keys only, small values: the original finding.
	b.Run("Keys/Small/Native", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for k := range nativeKeysOnly(small) {
				t += k
			}
			sinkInt = t
		}
	})
	b.Run("Keys/Small/DerivedFromAll", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for k := range derivedKeys(nativeAll(small)) {
				t += k
			}
			sinkInt = t
		}
	})

	// Keys only, 1 KB values: does dropping the value still cost nothing?
	b.Run("Keys/Big/Native", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for k := range nativeKeysOnly(big) {
				t += k
			}
			sinkInt = t
		}
	})
	b.Run("Keys/Big/DerivedFromAll", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for k := range derivedKeysBig(nativeAllBig(big)) {
				t += k
			}
			sinkInt = t
		}
	})
	// And if All yielded a pointer to the value instead.
	b.Run("Keys/Big/DerivedFromPointerAll", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for k := range derivedKeysBigPtr(nativeAllBigPtr(big)) {
				t += k
			}
			sinkInt = t
		}
	})

	// Values, 1 KB: here the consumer wants the value, so the copy is not waste.
	b.Run("Values/Big/Native", func(b *testing.B) {
		for b.Loop() {
			var t int64
			for v := range nativeValuesBig(big) {
				t += v[0]
			}
			sinkI64 = t
		}
	})
	b.Run("Values/Big/DerivedFromAll", func(b *testing.B) {
		for b.Loop() {
			var t int64
			for v := range derivedValuesBig(nativeAllBig(big)) {
				t += v[0]
			}
			sinkI64 = t
		}
	})
}

// ---- 5. the same question, but through an interface ---------------------
//
// A view returns its iterator through a sealed interface, so the call is
// dynamic. Whatever the compiler was eliding above, it cannot see through this.

func BenchmarkElementSizeThroughInterface(b *testing.B) {
	src := NewBigSource(bigFixture())

	b.Run("Keys/Native", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for k := range src.Keys() {
				t += k
			}
			sinkInt = t
		}
	})
	b.Run("Keys/DerivedFromAll", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for k := range derivedKeysBig(src.All()) {
				t += k
			}
			sinkInt = t
		}
	})
	// For scale: what consuming the value actually costs.
	b.Run("Values/Native", func(b *testing.B) {
		for b.Loop() {
			var t int64
			for v := range src.Values() {
				t += v[0]
			}
			sinkI64 = t
		}
	})
}

// ---- 6. does a dropped value's cost scale with its width? ---------------

type probeable interface{ probe() int64 }

func benchDrop[T probeable](b *testing.B, n int) {
	src := newSource(make([]T, n))

	b.Run("KeysNative", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for k := range src.keys() {
				t += k
			}
			sinkInt = t
		}
	})
	b.Run("KeysDropped", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for k := range dropValue(src.all()) {
				t += k
			}
			sinkInt = t
		}
	})
	// The value genuinely consumed: the copy is not waste here, so this is what
	// the same bytes cost when they are wanted.
	b.Run("ValuesConsumed", func(b *testing.B) {
		for b.Loop() {
			var t int64
			for _, v := range src.all() {
				t += v.probe()
			}
			sinkI64 = t
		}
	})
}

func (s Sz64) probe() int64 { return s[0] }
func (s Sz1K) probe() int64 { return s[0] }
func (s Sz8K) probe() int64 { return s[0] }

func BenchmarkDropScaling(b *testing.B) {
	const n = 256
	b.Run("64B", func(b *testing.B) { benchDrop[Sz64](b, n) })
	b.Run("1KiB", func(b *testing.B) { benchDrop[Sz1K](b, n) })
	b.Run("8KiB", func(b *testing.B) { benchDrop[Sz8K](b, n) })
}
