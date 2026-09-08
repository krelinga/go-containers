package sliceadapter

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
)

var (
	sinkInt   int
	sinkView  VectorView[int]
	sinkBytes []byte
)

const n = 1024

func source() []int {
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

// ---- 1. what the adapter buys the sized constructors ---------------------
//
// ADR 0015 made Vector.Append non-variadic, so bulk-appending a plain slice
// goes through either a length-carrying Elems or a bare iterator with none.

func BenchmarkAppendAll(b *testing.B) {
	src := source()

	// Through Elems: Len is available, so slices.Grow allocates once.
	b.Run("FromSliceAdapter", func(b *testing.B) {
		for b.Loop() {
			v := &Vector[int]{}
			v.AppendAll(Slice[int](src))
			sinkInt = v.Len()
		}
	})
	// Through a bare iterator: no length, so append regrows as it goes.
	b.Run("FromValuesSeq", func(b *testing.B) {
		for b.Loop() {
			v := &Vector[int]{}
			v.AppendAllSeq(slices.Values(src))
			sinkInt = v.Len()
		}
	})
	// The floor: what a plain append into a presized slice costs.
	b.Run("RawPresizedAppend", func(b *testing.B) {
		for b.Loop() {
			es := make([]int, 0, len(src))
			es = append(es, src...)
			sinkInt = len(es)
		}
	})
}

// Appending onto a vector that already holds elements, which is the case
// slices.Grow exists for.
func BenchmarkAppendAllOntoNonEmpty(b *testing.B) {
	src := source()

	b.Run("FromSliceAdapter", func(b *testing.B) {
		for b.Loop() {
			v := &Vector[int]{es: slices.Clone(src)}
			v.AppendAll(Slice[int](src))
			sinkInt = v.Len()
		}
	})
	b.Run("FromValuesSeq", func(b *testing.B) {
		for b.Loop() {
			v := &Vector[int]{es: slices.Clone(src)}
			v.AppendAllSeq(slices.Values(src))
			sinkInt = v.Len()
		}
	})
}

// ---- 2. does a defined slice type keep bounds-check elimination? ---------

func BenchmarkIndexedSum(b *testing.B) {
	raw := source()
	ad := Slice[int](raw)
	vec := &Vector[int]{es: raw}

	b.Run("RawSlice", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for i := 0; i < len(raw); i++ {
				t += raw[i]
			}
			sinkInt = t
		}
	})
	// A defined slice type can still be indexed with builtin syntax.
	b.Run("SliceAdapterBuiltinIndex", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for i := 0; i < len(ad); i++ {
				t += ad[i]
			}
			sinkInt = t
		}
	})
	// Or through its accessor, like any container.
	b.Run("SliceAdapterAtMethod", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for i := 0; i < ad.Len(); i++ {
				t += ad.At(i)
			}
			sinkInt = t
		}
	})
	// The struct wrapper, which vectorcost measured at +24%.
	b.Run("VectorAtMethod", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for i := 0; i < vec.Len(); i++ {
				t += vec.At(i)
			}
			sinkInt = t
		}
	})
}

// ---- 3. what a view over each backing costs ------------------------------

func BenchmarkViewConstruct(b *testing.B) {
	raw := source()
	ad := Slice[int](raw)
	vec := &Vector[int]{es: raw}

	b.Run("OverVector", func(b *testing.B) {
		for b.Loop() {
			sinkView = ViewVectorIdentity(vec)
		}
	})
	b.Run("OverSliceByValue", func(b *testing.B) {
		for b.Loop() {
			sinkView = ViewSliceValue(ad)
		}
	})
	b.Run("OverSliceByPointer", func(b *testing.B) {
		for b.Loop() {
			sinkView = ViewSlicePtr(&ad)
		}
	})
}

// ---- 4. the behavioural differences, reported ----------------------------

func BenchmarkReportSemantics(b *testing.B) {
	// A value receiver can write through, but cannot grow.
	s := Slice[int]{1, 2, 3}
	s.Set(0, 99)
	before := len(s)
	growByValue(s, 4)
	b.Logf("Set through a value receiver sticks=%v; Append through one sticks=%v",
		s[0] == 99, len(s) != before)

	// Conversion is free and aliases in both directions.
	raw := []int{7, 8}
	conv := Slice[int](raw)
	conv.Set(0, 70)
	b.Logf("conversion from []T aliases=%v, and converts back=%v",
		raw[0] == 70, len([]int(conv)) == 2)

	// A slice header is still a value, so an adapter does NOT fix the hazards
	// Vector exists for.
	base := make(Slice[int], 1, 10)
	x := append(base, 1)
	y := append(base, 2)
	b.Logf("appends off a shared adapter header still alias=%v", x[1] == y[1])

	// encoding/json: the reason ADR 0009 keeps Map.
	adJSON, _ := json.Marshal(Slice[int]{1, 2, 3})
	vecJSON, _ := json.Marshal(&Vector[int]{es: []int{1, 2, 3}})
	b.Logf("json.Marshal: Slice -> %s   Vector -> %s", adJSON, vecJSON)

	var round Slice[int]
	err := json.Unmarshal([]byte(`[4,5,6]`), &round)
	b.Logf("json.Unmarshal into Slice: %v err=%v", round, err)

	b.Logf("widths: Slice header=%d  *Vector=%d", sizeOf(Slice[int](nil)), sizeOf(&Vector[int]{}))

	sinkBytes = adJSON
	for b.Loop() {
		sinkInt++
	}
}

//go:noinline
func growByValue(s Slice[int], e int) { s = append(s, e); _ = s }

var _ = fmt.Sprint

// ---- 5. what about just expanding the slice variadically? ----------------

func BenchmarkAppendBulkForms(b *testing.B) {
	src := source()

	b.Run("Empty/VariadicExpansion", func(b *testing.B) {
		for b.Loop() {
			v := &Vector[int]{}
			v.AppendMany(src...)
			sinkInt = v.Len()
		}
	})
	b.Run("Empty/SliceAdapter", func(b *testing.B) {
		for b.Loop() {
			v := &Vector[int]{}
			v.AppendAll(Slice[int](src))
			sinkInt = v.Len()
		}
	})
	b.Run("Empty/ValuesSeq", func(b *testing.B) {
		for b.Loop() {
			v := &Vector[int]{}
			v.AppendAllSeq(slices.Values(src))
			sinkInt = v.Len()
		}
	})
	b.Run("Empty/RawFloor", func(b *testing.B) {
		for b.Loop() {
			es := make([]int, 0, len(src))
			es = append(es, src...)
			sinkInt = len(es)
		}
	})

	b.Run("NonEmpty/VariadicExpansion", func(b *testing.B) {
		for b.Loop() {
			v := &Vector[int]{es: slices.Clone(src)}
			v.AppendMany(src...)
			sinkInt = v.Len()
		}
	})
	b.Run("NonEmpty/SliceAdapter", func(b *testing.B) {
		for b.Loop() {
			v := &Vector[int]{es: slices.Clone(src)}
			v.AppendAll(Slice[int](src))
			sinkInt = v.Len()
		}
	})
}

// Does the variadic bulk form help when the source is NOT already a slice --
// the case the Elems contract exists for?
func BenchmarkAppendFromContainer(b *testing.B) {
	other := &Vector[int]{es: source()}

	b.Run("ViaElems", func(b *testing.B) {
		for b.Loop() {
			v := &Vector[int]{}
			v.AppendAll(other)
			sinkInt = v.Len()
		}
	})
	// A caller with a container and only a variadic method must materialise a
	// slice first.
	b.Run("ViaVariadicNeedsCollect", func(b *testing.B) {
		for b.Loop() {
			v := &Vector[int]{}
			v.AppendMany(slices.Collect(other.All())...)
			sinkInt = v.Len()
		}
	})
}
