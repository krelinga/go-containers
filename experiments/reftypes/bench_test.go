package reftypes

import (
	"fmt"
	"testing"
)

// Typed sinks. An `any` sink would box and add an allocation to every
// measurement.
var (
	sinkInt      int
	sinkBool     bool
	sinkPtrSet   *PtrSet[int]
	sinkRefSet   RefSet[int]
	sinkNilSafe  NilSafeSet[int]
	sinkIfaceSet IfaceSet[int]
)

const nElems = 64

func fixtures() (*PtrSet[int], RefSet[int], NilSafeSet[int], IfaceSet[int]) {
	p, r, n, i := NewPtrSet[int](), NewRefSet[int](), NewNilSafeSet[int](), NewIfaceSet[int]()
	for k := range nElems {
		p.Add(k)
		r.Add(k)
		n.Add(k)
		i.Add(k)
	}
	return p, r, n, i
}

func BenchmarkNoOp(b *testing.B) {
	for b.Loop() {
		sinkInt++
	}
}

// ---- 1. the read that dominates: does the handle shape show up at all? ----

func BenchmarkHas(b *testing.B) {
	p, r, n, i := fixtures()
	probe := nElems / 2

	b.Run("Ptr", func(b *testing.B) {
		for b.Loop() {
			sinkBool = p.Has(probe)
		}
	})
	b.Run("RefStruct", func(b *testing.B) {
		for b.Loop() {
			sinkBool = r.Has(probe)
		}
	})
	b.Run("RefStructNilChecked", func(b *testing.B) {
		for b.Loop() {
			sinkBool = n.Has(probe)
		}
	})
	b.Run("Interface", func(b *testing.B) {
		for b.Loop() {
			sinkBool = i.Has(probe)
		}
	})
}

func BenchmarkLen(b *testing.B) {
	p, r, n, i := fixtures()

	b.Run("Ptr", func(b *testing.B) {
		for b.Loop() {
			sinkInt = p.Len()
		}
	})
	b.Run("RefStruct", func(b *testing.B) {
		for b.Loop() {
			sinkInt = r.Len()
		}
	})
	b.Run("RefStructNilChecked", func(b *testing.B) {
		for b.Loop() {
			sinkInt = n.Len()
		}
	})
	b.Run("Interface", func(b *testing.B) {
		for b.Loop() {
			sinkInt = i.Len()
		}
	})
}

func BenchmarkAdd(b *testing.B) {
	b.Run("Ptr", func(b *testing.B) {
		s := NewPtrSet[int]()
		k := 0
		for b.Loop() {
			s.Add(k)
			k++
		}
	})
	b.Run("RefStruct", func(b *testing.B) {
		s := NewRefSet[int]()
		k := 0
		for b.Loop() {
			s.Add(k)
			k++
		}
	})
	b.Run("Interface", func(b *testing.B) {
		s := NewIfaceSet[int]()
		k := 0
		for b.Loop() {
			s.Add(k)
			k++
		}
	})
}

// ---- 2. iteration, where the nil check sits outside the loop -------------

func BenchmarkAll(b *testing.B) {
	p, r, n, i := fixtures()

	b.Run("Ptr", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for v := range p.All() {
				t += v
			}
			sinkInt = t
		}
	})
	b.Run("RefStruct", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for v := range r.All() {
				t += v
			}
			sinkInt = t
		}
	})
	// AllInner, not All: see BenchmarkAllNilCheckPlacement for why the check
	// belongs inside the closure.
	b.Run("RefStructNilChecked", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for v := range n.AllInner() {
				t += v
			}
			sinkInt = t
		}
	})
	b.Run("Interface", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for v := range i.All() {
				t += v
			}
			sinkInt = t
		}
	})
}

// ---- 3. construction, and what a handle costs to make ---------------------

func BenchmarkConstruct(b *testing.B) {
	b.Run("Ptr", func(b *testing.B) {
		for b.Loop() {
			sinkPtrSet = NewPtrSet[int]()
		}
	})
	b.Run("RefStruct", func(b *testing.B) {
		for b.Loop() {
			sinkRefSet = NewRefSet[int]()
		}
	})
	b.Run("RefStructNilChecked", func(b *testing.B) {
		for b.Loop() {
			sinkNilSafe = NewNilSafeSet[int]()
		}
	})
	b.Run("Interface", func(b *testing.B) {
		for b.Loop() {
			sinkIfaceSet = NewIfaceSet[int]()
		}
	})
}

// ---- 4. the zero value, which is the whole nil question ------------------

func BenchmarkReportNilSemantics(b *testing.B) {
	var raw map[int]struct{}
	var ref RefSet[int]
	var safe NilSafeSet[int]
	var iface IfaceSet[int]

	rangeCount := func(f func() int) int { return f() }

	b.Logf("builtin nil map:      len=%d  read=%v  write panics=%v",
		len(raw),
		func() bool { _, ok := raw[1]; return !ok }(),
		panics(func() { raw[1] = struct{}{} }))

	b.Logf("RefStruct (no check): nil-comparable=false  Len panics=%v  Has panics=%v  write panics=%v",
		panics(func() { sinkInt = ref.Len() }),
		panics(func() { sinkBool = ref.Has(1) }),
		panics(func() { ref.Add(1) }))

	n := rangeCount(func() int {
		c := 0
		for range safe.All() {
			c++
		}
		return c
	})
	b.Logf("RefStruct (checked):  nil-comparable=false  IsZero=%v  len=%d  read-ok=%v  range=%d  write panics=%v",
		safe.IsZero(), safe.Len(), !safe.Has(1), n, panics(func() { safe.Add(1) }))

	b.Logf("Interface:            nil-comparable=%v  Len panics=%v  Has panics=%v  write panics=%v",
		iface == nil,
		panics(func() { sinkInt = iface.Len() }),
		panics(func() { sinkBool = iface.Has(1) }),
		panics(func() { iface.Add(1) }))

	b.Logf("widths: Ptr=%d RefStruct=%d NilChecked=%d Interface=%d",
		sizeOf(sinkPtrSet), sizeOf(RefSet[int]{}), sizeOf(NilSafeSet[int]{}), sizeOf(sinkIfaceSet))

	for b.Loop() {
		sinkInt++
	}
}

func panics(f func()) (did bool) {
	defer func() { did = recover() != nil }()
	f()
	return false
}

// ---- 5. what reference semantics fixes: copies that share ---------------

func BenchmarkReportCopySemantics(b *testing.B) {
	// Value semantics: the copy's append does not reach the original, and may
	// or may not scribble on it depending on capacity. This is the aliasing
	// surprise the repository was started over.
	orig := &ValueList[int]{es: make([]int, 0, 8)}
	orig.Append(1)
	cp := *orig // vet's copylocks would flag this today; here it compiles
	cp.Append(2)
	b.Logf("value copy: original Len=%d, copy Len=%d  -> diverged=%v",
		orig.Len(), cp.Len(), orig.Len() != cp.Len())

	// Reference semantics: the copy is the same container, like a map.
	ref := NewRefList[int](8)
	ref.Append(1)
	rcp := ref
	rcp.Append(2)
	b.Logf("reference copy: original Len=%d, copy Len=%d -> shared=%v",
		ref.Len(), rcp.Len(), ref.Len() == rcp.Len())

	for b.Loop() {
		sinkInt++
	}
}

// ---- 6. the ceiling ADR 0002 recorded, and whether it survives ----------

func BenchmarkReportSetAlgebra(b *testing.B) {
	a, c := NewIfaceSet[int](), NewIfaceSet[int]()
	a.Add(1)
	c.Add(2)
	u := a.Union(c)
	b.Logf("Union on a sealed interface returns the interface: Len=%d", u.Len())
	b.Logf("under ADR 0002's shape this does not compile: *HashSet.Union cannot")
	b.Logf("satisfy an interface method returning the interface")
	_ = fmt.Sprint(u.Len())
	for b.Loop() {
		sinkInt++
	}
}

// ---- 7. where does the nil check belong in All? --------------------------
func BenchmarkAllNilCheckPlacement(b *testing.B) {
	_, _, n, _ := fixtures()

	b.Run("CheckBeforeClosure", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for v := range n.All() {
				t += v
			}
			sinkInt = t
		}
	})
	b.Run("CheckInsideClosure", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for v := range n.AllInner() {
				t += v
			}
			sinkInt = t
		}
	})
	// And on a zero container, which is the case the check exists for.
	var zero NilSafeSet[int]
	b.Run("ZeroContainer", func(b *testing.B) {
		for b.Loop() {
			t := 0
			for v := range zero.AllInner() {
				t += v
			}
			sinkInt = t
		}
	})
}
