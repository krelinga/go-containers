package iteration

import "testing"

// Typed sinks: an interface sink for A, a struct sink for B. Assigning to each
// is what the two proposals actually ask a call site to do.
var (
	sinkCollA  CollA[string]
	sinkSrcB   SrcB[string]
	sinkABKeys []string
)

func mkA(n int) (*udictA, *odictA) {
	ks := fixtureKeys(n)
	m := make(map[string]int, n)
	for i, k := range ks {
		m[k] = i
	}
	return &udictA{m}, &odictA{ks}
}

func mkB(n int) (*udictB, *odictB) {
	ks := fixtureKeys(n)
	m := make(map[string]int, n)
	for i, k := range ks {
		m[k] = i
	}
	return &udictB{m}, &odictB{ks}
}

// 1. Constructing the thing and nothing else. This is the boxing question in
// isolation: A must box a 3-word struct into an interface; B copies a value.
func BenchmarkABConstruct(b *testing.B) {
	ua, oa := mkA(64)
	ub, ob := mkB(64)

	b.Run("unordered/A_KeysOf", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkCollA = KeysOfA[string](ua)
		}
	})
	b.Run("unordered/B_Keys", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSrcB = ub.Keys()
		}
	})
	b.Run("ordered/A_KeysOf", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkCollA = KeysOfA[string](oa)
		}
	})
	b.Run("ordered/B_Keys", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSrcB = ob.Keys()
		}
	})
}

// 2. The whole operation: construct a vector from one container's keys.
func abBuild(b *testing.B, n int) {
	ua, oa := mkA(n)
	ub, ob := mkB(n)

	b.Run("unordered/A", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkABKeys = newVecA(KeysOfA[string](ua))
		}
	})
	b.Run("unordered/B", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkABKeys = newVecB(ub.Keys())
		}
	})
	b.Run("ordered/A", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkABKeys = newVecA(KeysOfA[string](oa))
		}
	})
	b.Run("ordered/B", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkABKeys = newVecB(ob.Keys())
		}
	})
}

func BenchmarkABBuild8(b *testing.B)    { abBuild(b, 8) }
func BenchmarkABBuild1024(b *testing.B) { abBuild(b, 1024) }

// 3. Three sources unioned. A's variadic slice holds 2-word interfaces; B's
// holds 4-word structs, so B's slice is larger even though it boxes nothing.
func BenchmarkABUnion3(b *testing.B) {
	ua1, _ := mkA(64)
	ua2, _ := mkA(64)
	ua3, _ := mkA(64)
	ub1, _ := mkB(64)
	ub2, _ := mkB(64)
	ub3, _ := mkB(64)

	b.Run("A", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkABKeys = newVecA(KeysOfA[string](ua1), KeysOfA[string](ua2), KeysOfA[string](ua3))
		}
	})
	b.Run("B", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkABKeys = newVecB(ub1.Keys(), ub2.Keys(), ub3.Keys())
		}
	})
}

// 4. A literal list, the cheapest possible source. Also isolates what B's
// second closure costs, by running B with and without a reverse half.
func BenchmarkABItems(b *testing.B) {
	b.Run("A", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkABKeys = newVecA(ItemsA("a", "b", "c"))
		}
	})
	b.Run("B", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkABKeys = newVecB(ItemsB("a", "b", "c"))
		}
	})
}

// 5. What B's second closure costs. newVecB never reads Back, so measuring the
// two through a consumer lets the compiler eliminate the reverse half entirely
// -- an elision artifact, not a result. Assigning the source to an escaping sink
// is what prices it: B populates Back eagerly whether or not anyone reads it,
// where A's RangeAll builds the reverse half only when asked for it.
func BenchmarkABReverseHalf(b *testing.B) {
	b.Run("B_reversible", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSrcB = ItemsB("a", "b", "c")
		}
	})
	b.Run("B_forward_only", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSrcB = ItemsBFwd("a", "b", "c")
		}
	})
	b.Run("A_equivalent", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkCollA = ItemsA("a", "b", "c")
		}
	})
}
