package refcontainers

import "testing"

var (
	sinkBool   bool
	sinkInt    int
	sinkStrs   []string
	sinkStr    string
	sinkOBBase obSetView[string]
	sinkIdv    identityView[string]
	sinkWide   wideView[string, string]
	sinkNarrow narrowView[string, string]
	sinkBase   structSetView[string]
	sinkIface  ifaceSetView[string]
	sinkWrap   wrapSetView[string]
)

const n = 64

// BenchmarkNoOp is the zero-cost baseline: it is how harness overhead leaking
// into the numbers above would show up.
func BenchmarkNoOp(b *testing.B) {
	for b.Loop() {
		sinkInt++
	}
}

// 1. The container shapes, on today's method set. ADR 0014 measured Has/Len/
// Add/All/construct on a toy set; KeySlice is new since ADR 0017 and is on the
// hot path for every bulk operation.
func BenchmarkContainerShape(b *testing.B) {
	vs := fixture(n)
	ps := newPtrSet(vs...)
	rs := newRefSet(vs...)
	ns := newNilRefSet(vs...)
	probe := vs[n/2]

	b.Run("Has/ptr", func(b *testing.B) {
		for b.Loop() {
			sinkBool = ps.Has(probe)
		}
	})
	b.Run("Has/ref", func(b *testing.B) {
		for b.Loop() {
			sinkBool = rs.Has(probe)
		}
	})
	b.Run("Has/ref+nilcheck", func(b *testing.B) {
		for b.Loop() {
			sinkBool = ns.Has(probe)
		}
	})

	b.Run("Len/ptr", func(b *testing.B) {
		for b.Loop() {
			sinkInt = ps.Len()
		}
	})
	b.Run("Len/ref", func(b *testing.B) {
		for b.Loop() {
			sinkInt = rs.Len()
		}
	})
	b.Run("Len/ref+nilcheck", func(b *testing.B) {
		for b.Loop() {
			sinkInt = ns.Len()
		}
	})

	b.Run("Keys/ptr", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range ps.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("Keys/ref", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range rs.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("Keys/ref+nilcheck", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range ns.Keys() {
				c++
			}
			sinkInt = c
		}
	})

	b.Run("KeySlice/ptr", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkStrs = ps.KeySlice()
		}
	})
	b.Run("KeySlice/ref", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkStrs = rs.KeySlice()
		}
	})
	b.Run("KeySlice/ref+nilcheck", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkStrs = ns.KeySlice()
		}
	})

	b.Run("construct/ptr", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = newPtrSet(vs...).Len()
		}
	})
	b.Run("construct/ref", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = newRefSet(vs...).Len()
		}
	})
}

// 2. The view shapes. ADR 0013's erratum -- iterating a view allocates three
// times per call through a dynamic call -- is live in the shipped library, and
// is the cost a struct view would remove rather than add.
func BenchmarkViewShape(b *testing.B) {
	vs := fixture(n)
	ps := newPtrSet(vs...)
	probe := vs[n/2]

	b.Run("construct/iface", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkIface = viewIface(ps)
		}
	})
	b.Run("construct/struct", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkBase = viewStruct(ps)
		}
	})

	iv, sv := viewIface(ps), viewStruct(ps)

	b.Run("Has/iface", func(b *testing.B) {
		for b.Loop() {
			sinkBool = iv.Has(probe)
		}
	})
	b.Run("Has/struct", func(b *testing.B) {
		for b.Loop() {
			sinkBool = sv.Has(probe)
		}
	})

	b.Run("Len/iface", func(b *testing.B) {
		for b.Loop() {
			sinkInt = iv.Len()
		}
	})
	b.Run("Len/struct", func(b *testing.B) {
		for b.Loop() {
			sinkInt = sv.Len()
		}
	})

	b.Run("Keys/iface", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range iv.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("Keys/struct", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range sv.Keys() {
				c++
			}
			sinkInt = c
		}
	})

	// The ADR 0017 path: bulk operations consume a view by materialising it.
	b.Run("KeySlice/iface", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkStrs = iv.KeySlice()
		}
	})
	b.Run("KeySlice/struct", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkStrs = sv.KeySlice()
		}
	})
}

// 3. Substitution -- ADR 0013 decision 2, an ordered view used where the base is
// wanted. Free with interfaces (embedding); an explicit conversion with structs.
func BenchmarkViewSubstitution(b *testing.B) {
	ps := newPtrSet(fixture(n)...)
	isv := viewIfaceSorted(ps)
	ssv := viewStructSorted(ps)

	b.Run("iface/embedding", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkIface = isv
		}
	})
	b.Run("struct/explicit", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkBase = ssv.Set()
		}
	})
}

// 4. The honest struct-view shape: a struct wrapping the sealed interface,
// which is what polymorphism over backings requires. The monomorphic form in
// BenchmarkViewShape inlines and is not what the library could ship.
func BenchmarkWrapView(b *testing.B) {
	vs := fixture(n)
	ps := newPtrSet(vs...)
	probe := vs[n/2]
	iv, wv := viewIface(ps), viewWrap(ps)

	b.Run("construct/iface", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkIface = viewIface(ps)
		}
	})
	b.Run("construct/wrap", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkWrap = viewWrap(ps)
		}
	})
	b.Run("Has/iface", func(b *testing.B) {
		for b.Loop() {
			sinkBool = iv.Has(probe)
		}
	})
	b.Run("Has/wrap", func(b *testing.B) {
		for b.Loop() {
			sinkBool = wv.Has(probe)
		}
	})
	b.Run("Keys/iface", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range iv.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("Keys/wrap", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range wv.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("KeySlice/iface", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkStrs = iv.KeySlice()
		}
	})
	b.Run("KeySlice/wrap", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkStrs = wv.KeySlice()
		}
	})

	// Substitution, in the shape that could actually ship.
	isv := viewIfaceSorted(ps)
	wsv := viewWrapSorted(ps)
	b.Run("substitute/iface", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkIface = isv
		}
	})
	b.Run("substitute/wrap", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkWrap = wsv.Set()
		}
	})
}

// 5. The all-interface shape: the container itself is a sealed interface.
// ADR 0014 rejected this on cost against a toy method set; ADR 0017 changed the
// method set, so it is re-priced here.
func BenchmarkIfaceContainer(b *testing.B) {
	vs := fixture(n)
	ps := newPtrSet(vs...)
	is := newIfaceSet(vs...)
	probe := vs[n/2]

	b.Run("Has/ptr", func(b *testing.B) {
		for b.Loop() {
			sinkBool = ps.Has(probe)
		}
	})
	b.Run("Has/iface", func(b *testing.B) {
		for b.Loop() {
			sinkBool = is.Has(probe)
		}
	})
	b.Run("Len/ptr", func(b *testing.B) {
		for b.Loop() {
			sinkInt = ps.Len()
		}
	})
	b.Run("Len/iface", func(b *testing.B) {
		for b.Loop() {
			sinkInt = is.Len()
		}
	})

	// Iteration: the shape that carries the per-call allocations.
	b.Run("Keys/ptr", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range ps.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("Keys/iface", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range is.Keys() {
				c++
			}
			sinkInt = c
		}
	})

	// The ADR 0017 bulk path: a slice through a dynamic call carries no
	// per-call allocation the way an iter.Seq does.
	b.Run("KeySlice/ptr", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkStrs = ps.KeySlice()
		}
	})
	b.Run("KeySlice/iface", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkStrs = is.KeySlice()
		}
	})

	b.Run("construct/ptr", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = newPtrSet(vs...).Len()
		}
	})
	b.Run("construct/iface", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = newIfaceSet(vs...).Len()
		}
	})

	// Set algebra: impossible on a contract for concrete containers (ADR 0002),
	// and the one thing this shape unlocks.
	po := newPtrSet(fixture(n / 2)...)
	io := newIfaceSet(fixture(n / 2)...)
	b.Run("Union/ptr", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = ps.Union(po).Len()
		}
	})
	b.Run("Union/iface", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = is.Union(io).Len()
		}
	})
}

// 6. Indexed access, the worst case for dispatch: At is close to free, so a
// fixed indirect call is a large share of it.
func BenchmarkIfaceIndexed(b *testing.B) {
	vs := fixture(n)
	pv := newPtrVec(vs...)
	iv := newIfaceVec(vs...)

	b.Run("At/ptr", func(b *testing.B) {
		for b.Loop() {
			sinkStr = pv.At(n / 2)
		}
	})
	b.Run("At/iface", func(b *testing.B) {
		for b.Loop() {
			sinkStr = iv.At(n / 2)
		}
	})
	b.Run("indexed-loop/ptr", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for i := range pv.Len() {
				t += len(pv.At(i))
			}
			sinkInt = t
		}
	})
	b.Run("indexed-loop/iface", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for i := range iv.Len() {
				t += len(iv.At(i))
			}
			sinkInt = t
		}
	})
}

// 7. Option (b) end to end: a reference container with nil-checked reads, and a
// struct view with the nil checks that make its zero value match. This is the
// complete shape, not a half of it.
func BenchmarkOptionB(b *testing.B) {
	vs := fixture(n)
	ps := newPtrSet(vs...)
	ns := newNilRefSet(vs...)
	probe := vs[n/2]
	iv := viewIface(ps)
	nv := viewNilWrap(ps)

	b.Run("container/Has/status-quo", func(b *testing.B) {
		for b.Loop() {
			sinkBool = ps.Has(probe)
		}
	})
	b.Run("container/Has/optionB", func(b *testing.B) {
		for b.Loop() {
			sinkBool = ns.Has(probe)
		}
	})
	b.Run("container/KeySlice/status-quo", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkStrs = ps.KeySlice()
		}
	})
	b.Run("container/KeySlice/optionB", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkStrs = ns.KeySlice()
		}
	})

	b.Run("view/Has/status-quo", func(b *testing.B) {
		for b.Loop() {
			sinkBool = iv.Has(probe)
		}
	})
	b.Run("view/Has/optionB", func(b *testing.B) {
		for b.Loop() {
			sinkBool = nv.Has(probe)
		}
	})
	b.Run("view/Keys/status-quo", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range iv.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("view/Keys/optionB", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range nv.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("view/KeySlice/status-quo", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkStrs = iv.KeySlice()
		}
	})
	b.Run("view/KeySlice/optionB", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkStrs = nv.KeySlice()
		}
	})

	// The emptiness check itself, which is the thing option (b) is buying.
	b.Run("isempty/status-quo(==nil)", func(b *testing.B) {
		for b.Loop() {
			sinkBool = ps == nil
		}
	})
	b.Run("isempty/optionB(IsZero)", func(b *testing.B) {
		for b.Loop() {
			sinkBool = nv.IsZero()
		}
	})
}

// 8. The nil-checked view iterator, two ways. A re-yield loop costs a second
// closure; handing yield straight through does not.
func BenchmarkNilCheckedIterator(b *testing.B) {
	ps := newPtrSet(fixture(n)...)
	iv := viewIface(ps)
	nv := viewNilWrap(ps)

	b.Run("no-check/iface", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range iv.Keys() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("checked/re-yield", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range nv.KeysReYield() {
				c++
			}
			sinkInt = c
		}
	})
	b.Run("checked/pass-through", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c := 0
			for range nv.Keys() {
				c++
			}
			sinkInt = c
		}
	})
}

// 9. Option (b) with the hierarchy recovered by embedding rather than a
// conversion method: is a promoted call, or a field-selector conversion, free?
func BenchmarkOptionBEmbedding(b *testing.B) {
	ps := newPtrSet(fixture(n)...)
	probe := fixture(n)[n/2]
	sv := viewOBSorted(ps)
	base := sv.SetView
	iv := viewIfaceSorted(ps)

	b.Run("Has/direct-on-base", func(b *testing.B) {
		for b.Loop() {
			sinkBool = base.Has(probe)
		}
	})
	b.Run("Has/promoted-through-ordered", func(b *testing.B) {
		for b.Loop() {
			sinkBool = sv.Has(probe)
		}
	})
	b.Run("Has/through-iface-embedding", func(b *testing.B) {
		for b.Loop() {
			sinkBool = iv.Has(probe)
		}
	})
	b.Run("substitute/iface-embedding", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkIface = iv
		}
	})
	b.Run("substitute/struct-field-selector", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkOBBase = sv.SetView
		}
	})

	// The zero-value read path, which every method now carries.
	var zero obSet[string]
	full := newOBSet(fixture(n)...)
	b.Run("zero/Len", func(b *testing.B) {
		for b.Loop() {
			sinkInt = zero.Len()
		}
	})
	b.Run("full/Len", func(b *testing.B) {
		for b.Loop() {
			sinkInt = full.Len()
		}
	})
	b.Run("full/Has", func(b *testing.B) {
		for b.Loop() {
			sinkBool = full.Has(probe)
		}
	})
	b.Run("full/KeySlice", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkStrs = full.KeySlice()
		}
	})
}

// 10. The two-tier design's decisive cost: passing a concrete view into a
// capability-interface parameter. A one-word struct is pointer-shaped and boxes
// free; anything wider allocates on every crossing.
func BenchmarkTwoTier(b *testing.B) {
	c := newPtrSet(fixture(n)...)
	idv := viewIdentity(c)
	wide := viewWide[string, string](c, upper{})
	narrow := viewNarrow[string, string](c, upper{})

	b.Run("construct/identity(1 word)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkIdv = viewIdentity(c)
		}
	})
	b.Run("construct/converting-wide(3 words)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkWide = viewWide[string, string](c, upper{})
		}
	})
	b.Run("construct/converting-narrow(1 word)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkNarrow = viewNarrow[string, string](c, upper{})
		}
	})

	// The boundary the design exists to make cheap.
	b.Run("pass-to-Set/container", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = countThrough[string](c)
		}
	})
	b.Run("pass-to-Set/identity(1 word)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = countThrough[string](idv)
		}
	})
	b.Run("pass-to-Set/converting-wide(3 words)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = countThrough[string](wide)
		}
	})
	b.Run("pass-to-Set/converting-narrow(1 word)", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkInt = countThrough[string](narrow)
		}
	})

	// Direct calls on the concrete type, for reference.
	b.Run("direct/identity.Len", func(b *testing.B) {
		for b.Loop() {
			sinkInt = idv.Len()
		}
	})
	b.Run("direct/narrow.Len", func(b *testing.B) {
		for b.Loop() {
			sinkInt = narrow.Len()
		}
	})
}
