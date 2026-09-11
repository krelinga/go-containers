package reverse

import "testing"

const n = 1024

var (
	sinkInt int
	sinkSet setView[int]
	sinkSpn span[int]
)

func fixture() sortedSet[int] {
	es := make([]int, n)
	for i := range es {
		es[i] = i
	}
	return newSortedSet(es)
}

// Baseline: what the forward walk costs, so the reverse numbers have a floor.
func BenchmarkBaseline(b *testing.B) {
	s := fixture()
	b.Run("noop", func(b *testing.B) {
		for b.Loop() {
			sinkInt++
		}
	})
	b.Run("A/forward-full", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.A_Keys() {
				c++
			}
			sinkInt = c
		}
	})
}

// 1. Full-container reverse, three shapes plus the one that must not ship.
func BenchmarkReverseFull(b *testing.B) {
	s := fixture()

	b.Run("A/method-pair", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.A_KeysBackward() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("B/view.Backward", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.B_View().Backward().Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("C/span.Backward", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.C_Span().Backward().Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("naive/materialise", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.Naive_Backward() {
				c++
			}
			sinkInt = c
		}
	})
}

// 2. Sub-range, forward. A 64-element window out of 1024 -- the case where
// materialising looks cheapest and is still the wrong shape.
func BenchmarkSubRangeForward(b *testing.B) {
	s := fixture()
	const lo, hi = 500, 564

	b.Run("A/method-pair", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.A_RangeKeys(lo, hi) {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("B/view", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.B_Range(lo, hi).Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("C/span", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.C_Range(lo, hi).Keys() {
				c++
			}
			sinkInt = c
		}
	})
}

// 3. Sub-range, backward -- the composition that is the whole point.
func BenchmarkSubRangeBackward(b *testing.B) {
	s := fixture()
	const lo, hi = 500, 564

	b.Run("A/method-pair", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.A_RangeKeysBackward(lo, hi) {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("B/view.Backward", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.B_Range(lo, hi).Backward().Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("C/span.Backward", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.C_Range(lo, hi).Backward().Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("naive/materialise", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.Naive_RangeBackward(lo, hi) {
				c++
			}
			sinkInt = c
		}
	})
}

// 4. Constructing the intermediate value, with no walk at all. This is where a
// composable shape either is or is not free.
func BenchmarkConstructOnly(b *testing.B) {
	s := fixture()
	const lo, hi = 500, 564

	b.Run("B/view", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSet = s.B_Range(lo, hi)
		}
	})
	b.Run("B/view.Backward", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSet = s.B_Range(lo, hi).Backward()
		}
	})
	b.Run("C/span", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSpn = s.C_Range(lo, hi)
		}
	})
	b.Run("C/span.Backward", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSpn = s.C_Range(lo, hi).Backward()
		}
	})
}

// 5. The case the first cut missed: a span that must hold an interface, because
// a VIEW produced it. Does it still construct for free?
func BenchmarkIfaceSpan(b *testing.B) {
	s := fixture()
	const lo, hi = 500, 564
	var sinkIS ifaceSpan[int]
	var sinkWS wrapSpan[int]

	b.Run("construct/concrete-span", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSpn = s.C_Range(lo, hi)
		}
	})
	b.Run("construct/iface-span", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkIS = s.D_IfaceSpan(lo, hi)
		}
	})
	b.Run("reverse/concrete-span", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkSpn = s.C_Range(lo, hi).Backward()
		}
	})
	b.Run("reverse/iface-span(flag)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkIS = s.D_IfaceSpan(lo, hi).Backward()
		}
	})
	b.Run("reverse/iface-span(wrapper)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkWS = s.D_WrapSpan(lo, hi).Backward()
		}
	})
	b.Run("walk/concrete-span", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.C_Range(lo, hi).Backward().Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("walk/iface-span", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range s.D_IfaceSpan(lo, hi).Backward().Keys() {
				c++
			}
			sinkInt = c
		}
	})
	_, _ = sinkIS, sinkWS
}

// 6. Where the interface-span allocation actually comes from.
func BenchmarkSpanBoxing(b *testing.B) {
	s := fixture()
	v := s.E_View()
	const lo, hi = 500, 564
	var sink ifaceSpan[int]

	b.Run("box-a-wide-impl(3 words)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sink = s.D_IfaceSpan(lo, hi)
		}
	})
	b.Run("box-a-pointer-impl(1 word)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sink = s.E_RangePtr(lo, hi)
		}
	})
	b.Run("copy-an-existing-iface(from a view)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sink = v.E_Range(500, 564)
		}
	})
	b.Run("copy-then-reverse", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sink = v.E_Range(500, 564).Backward()
		}
	})
	_ = sink
}

// 7. What Len costs on a key-bounded span, which is the only correct kind.
func BenchmarkSpanLen(b *testing.B) {
	s := fixture()
	const lo, hi = 500, 564
	kb := s.F_KeyBounded(lo, hi)
	ib := s.F_IndexBounded(lo, hi)

	b.Run("container/Len", func(b *testing.B) {
		for b.Loop() {
			sinkInt = len(s.st.es)
		}
	})
	b.Run("span/Len(key-bounded)", func(b *testing.B) {
		for b.Loop() {
			sinkInt = kb.Len()
		}
	})
	b.Run("span/Len(index-bounded, stale-prone)", func(b *testing.B) {
		for b.Loop() {
			sinkInt = ib.Len()
		}
	})
	b.Run("construct/key-bounded", func(b *testing.B) {
		b.ReportAllocs()
		var sink keyBoundedSpan[int]
		for b.Loop() {
			sink = s.F_KeyBounded(lo, hi)
		}
		_ = sink
	})
	b.Run("construct/index-bounded", func(b *testing.B) {
		b.ReportAllocs()
		var sink indexBoundedSpan[int]
		for b.Loop() {
			sink = s.F_IndexBounded(lo, hi)
		}
		_ = sink
	})
}
