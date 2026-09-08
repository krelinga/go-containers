package viewiface

import (
	"fmt"
	"testing"
)

// Spelling the concrete forms once here is itself the ergonomic complaint that
// prompted this experiment: four type arguments, two of which name types the
// consumer never mentions.
type (
	fieldForm = FieldView[*Item, *Item, string, ItemView]
	ptrForm   = PtrView[*Item, *Item, string, ItemView]
	ifaceForm = DictView[string, ItemView]
	contract  = Contract[string, ItemView]
)

// Typed sinks. An `any` sink would box and add an allocation to every
// measurement; the interface sinks below are interfaces because the value under
// measurement is one, not for convenience.
var (
	sinkItemView ItemView
	sinkBool     bool
	sinkInt      int
	sinkField    fieldForm
	sinkPtr      ptrForm
	sinkIface    ifaceForm
	sinkContract contract
)

const nEntries = 64

func fixture() (*Dict[*Item, *Item], itemViewer, string) {
	d := NewDict[*Item, *Item]()
	known := map[string]*Item{}
	var probe string
	for i := range nEntries {
		name := fmt.Sprintf("key-%d", i)
		k := &Item{Name: name}
		d.Set(k, &Item{Name: "payload"})
		known[name] = k
		if i == nEntries/2 {
			probe = name
		}
	}
	return d, itemViewer{known: known}, probe
}

// The boundary. Not inlinable, so the box genuinely escapes the caller's frame
// -- the common case for a view handed to another package.
//
//go:noinline
func crossAsContract(d contract, k string) ItemView {
	v, _ := d.Get(k)
	return v
}

//go:noinline
func crossAsView(d ifaceForm, k string) ItemView {
	v, _ := d.Get(k)
	return v
}

// Inlinable twin, to see whether escape analysis rescues the concrete form.
func crossInlinable(d contract, k string) ItemView {
	v, _ := d.Get(k)
	return v
}

func BenchmarkNoOp(b *testing.B) {
	for b.Loop() {
		sinkInt++
	}
}

// ---- 1. construction, with no boundary crossed -----------------------------

func BenchmarkConstruct(b *testing.B) {
	d, vw, _ := fixture()

	b.Run("Field", func(b *testing.B) {
		for b.Loop() {
			sinkField = ViewField[*Item, *Item, string, ItemView](d, vw)
		}
	})
	b.Run("Iface", func(b *testing.B) {
		for b.Loop() {
			sinkIface = ViewIface[*Item, *Item, string, ItemView](d, vw)
		}
	})
	b.Run("Ptr", func(b *testing.B) {
		for b.Loop() {
			sinkPtr = ViewPtr[*Item, *Item, string, ItemView](d, vw)
		}
	})
}

// ---- 2. local use: construct and call, never crossing a boundary -----------

func BenchmarkLocal(b *testing.B) {
	d, vw, probe := fixture()

	b.Run("Field", func(b *testing.B) {
		for b.Loop() {
			v := ViewField[*Item, *Item, string, ItemView](d, vw)
			sinkItemView, sinkBool = v.Get(probe)
		}
	})
	b.Run("Iface", func(b *testing.B) {
		for b.Loop() {
			v := ViewIface[*Item, *Item, string, ItemView](d, vw)
			sinkItemView, sinkBool = v.Get(probe)
		}
	})
	b.Run("Ptr", func(b *testing.B) {
		for b.Loop() {
			v := ViewPtr[*Item, *Item, string, ItemView](d, vw)
			sinkItemView, sinkBool = v.Get(probe)
		}
	})
}

// ---- 3. the crossover: construct once, then cross N boundaries -------------

var crossings = []int{1, 2, 4, 8}

func BenchmarkCross(b *testing.B) {
	d, vw, probe := fixture()

	for _, n := range crossings {
		b.Run(fmt.Sprintf("Field/%d", n), func(b *testing.B) {
			for b.Loop() {
				v := ViewField[*Item, *Item, string, ItemView](d, vw)
				for range n {
					sinkItemView = crossAsContract(v, probe) // boxes every time
				}
			}
		})
		b.Run(fmt.Sprintf("Iface/%d", n), func(b *testing.B) {
			for b.Loop() {
				v := ViewIface[*Item, *Item, string, ItemView](d, vw)
				for range n {
					sinkItemView = crossAsView(v, probe) // already boxed
				}
			}
		})
		b.Run(fmt.Sprintf("Ptr/%d", n), func(b *testing.B) {
			for b.Loop() {
				v := ViewPtr[*Item, *Item, string, ItemView](d, vw)
				for range n {
					sinkItemView = crossAsContract(v, probe) // pointer-shaped: free
				}
			}
		})
	}
}

// ---- 4. the box in isolation, on an already-constructed view ---------------

func BenchmarkBoxOnly(b *testing.B) {
	d, vw, _ := fixture()
	fv := ViewField[*Item, *Item, string, ItemView](d, vw)
	pv := ViewPtr[*Item, *Item, string, ItemView](d, vw)
	iv := ViewIface[*Item, *Item, string, ItemView](d, vw)

	b.Run("Field", func(b *testing.B) {
		for b.Loop() {
			sinkContract = fv
		}
	})
	b.Run("Ptr", func(b *testing.B) {
		for b.Loop() {
			sinkContract = pv
		}
	})
	b.Run("Iface", func(b *testing.B) {
		for b.Loop() {
			sinkIface = iv
		}
	})
}

// ---- 5. does escape analysis rescue the concrete form? ---------------------

func BenchmarkEscape(b *testing.B) {
	d, vw, probe := fixture()
	fv := ViewField[*Item, *Item, string, ItemView](d, vw)

	b.Run("NoInline", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = crossAsContract(fv, probe)
		}
	})
	b.Run("Inlinable", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = crossInlinable(fv, probe)
		}
	})
	// A direct call on the concrete type: no interface anywhere.
	b.Run("Direct", func(b *testing.B) {
		for b.Loop() {
			sinkItemView, sinkBool = fv.Get(probe)
		}
	})
}

// ---- 6. widths, which drive all of the above -------------------------------

func BenchmarkReportSizes(b *testing.B) {
	d, vw, _ := fixture()
	b.Logf("FieldView=%dB PtrView=%dB interface=%dB Dict*=%dB",
		sizeOf(ViewField[*Item, *Item, string, ItemView](d, vw)),
		sizeOf(ViewPtr[*Item, *Item, string, ItemView](d, vw)),
		sizeOf(sinkContract),
		sizeOf(d))
	for b.Loop() {
		sinkInt++
	}
}

// ---- 7. does a sealed interface keep the guarantee ADR 0011 bought? --------
//
// The concrete-struct decision rested on one fact: a container satisfies the
// read contracts structurally, so a bare contract can be asserted back to the
// container and mutated. An interface is only a candidate here if sealing
// closes that at least as well.
func BenchmarkReportSealing(b *testing.B) {
	d, vw, _ := fixture()
	before := d.Len()

	// A bare contract over the RAW KEY TYPE: the container satisfies it
	// structurally, so a provider can pass the container itself, and the holder
	// asserts straight back to it.
	var bare Contract[*Item, *Item] = d
	leaked, bareLeaks := bare.(*Dict[*Item, *Item])
	if bareLeaks {
		leaked.Set(&Item{Name: "injected"}, &Item{}) // mutation through a "read-only" contract
	}
	containerIsContract := true

	// The seal: *Dict has Len and Get but not sealedView, so it cannot be
	// stored in a DictView. This is a COMPILE-time failure --
	//     var v ifaceForm = d   // *Dict does not implement DictView
	// -- checked here at runtime for the record.
	_, containerIsView := any(d).(interface{ sealedView() })

	// And a DictView cannot be asserted back to the container.
	iv := ViewIface[*Item, *Item, string, ItemView](d, vw)
	_, viewLeaks := any(iv).(*Dict[*Item, *Item])

	// It CAN be asserted back to the view struct, by a caller who can spell it.
	// That yields no container: the fields are unexported.
	back, viewAssertsToStruct := any(iv).(fieldForm)
	_ = back

	b.Logf("container satisfies a bare contract:      %v", containerIsContract)
	b.Logf("bare contract asserts back to container:  %v", bareLeaks)
	b.Logf("container satisfies the SEALED interface: %v  (compile error too)", containerIsView)
	b.Logf("sealed view asserts back to container:    %v", viewLeaks)
	b.Logf("sealed view asserts back to view struct:  %v  (fields still unexported)", viewAssertsToStruct)
	b.Logf("container unchanged: %v", d.Len() == before)

	for b.Loop() {
		sinkInt++
	}
}

// ---- 8. passing, isolated from construction and from boxing ---------------
//
// The Cross benchmarks above bundle construction into the measurement, so they
// answer "what does building and handing off a view cost" rather than "what
// does an argument of this shape cost". These separate the two: every value is
// built once, outside the loop.
//
// Two tiers. The first callee ignores its argument, so the measurement is the
// argument itself plus a call. The second calls one method, which is what a
// boundary actually does.

//go:noinline
func passNothing() int { return 1 }

//go:noinline
func passField(v fieldForm) int { return 1 }

//go:noinline
func passPtr(v ptrForm) int { return 1 }

//go:noinline
func passIface(v ifaceForm) int { return 1 }

//go:noinline
func passContract(v contract) int { return 1 }

//go:noinline
func useField(v fieldForm, k string) ItemView { r, _ := v.Get(k); return r }

//go:noinline
func usePtr(v ptrForm, k string) ItemView { r, _ := v.Get(k); return r }

//go:noinline
func useIface(v ifaceForm, k string) ItemView { r, _ := v.Get(k); return r }

//go:noinline
func useContract(v contract, k string) ItemView { r, _ := v.Get(k); return r }

// Tier 1: the argument and the call, with no method dispatched.
func BenchmarkPass(b *testing.B) {
	d, vw, _ := fixture()
	fv := ViewField[*Item, *Item, string, ItemView](d, vw)
	pv := ViewPtr[*Item, *Item, string, ItemView](d, vw)
	iv := ViewIface[*Item, *Item, string, ItemView](d, vw)

	b.Run("NoArg", func(b *testing.B) {
		for b.Loop() {
			sinkInt = passNothing()
		}
	})
	// Concrete parameter types: no interface anywhere, so this is pure
	// argument width -- 3 words against 1.
	b.Run("Struct3Word", func(b *testing.B) {
		for b.Loop() {
			sinkInt = passField(fv)
		}
	})
	b.Run("Struct1Word", func(b *testing.B) {
		for b.Loop() {
			sinkInt = passPtr(pv)
		}
	})
	// Interface parameter, already holding an interface: 2 words copied, and
	// no conversion, because the types are identical.
	b.Run("IfaceAsIs", func(b *testing.B) {
		for b.Loop() {
			sinkInt = passIface(iv)
		}
	})
	// Interface parameter, holding a struct: this is where boxing happens.
	b.Run("Struct3WordBoxed", func(b *testing.B) {
		for b.Loop() {
			sinkInt = passContract(fv)
		}
	})
	b.Run("Struct1WordBoxed", func(b *testing.B) {
		for b.Loop() {
			sinkInt = passContract(pv)
		}
	})
	// Interface to a DIFFERENT interface: no allocation, but an itab lookup.
	b.Run("IfaceConverted", func(b *testing.B) {
		for b.Loop() {
			sinkInt = passContract(iv)
		}
	})
}

// Tier 2: the same, with one method called through the parameter.
func BenchmarkPassAndCall(b *testing.B) {
	d, vw, probe := fixture()
	fv := ViewField[*Item, *Item, string, ItemView](d, vw)
	pv := ViewPtr[*Item, *Item, string, ItemView](d, vw)
	iv := ViewIface[*Item, *Item, string, ItemView](d, vw)

	b.Run("Struct3Word", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = useField(fv, probe)
		}
	})
	b.Run("Struct1Word", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = usePtr(pv, probe)
		}
	})
	b.Run("IfaceAsIs", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = useIface(iv, probe)
		}
	})
	b.Run("Struct3WordBoxed", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = useContract(fv, probe)
		}
	})
	b.Run("Struct1WordBoxed", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = useContract(pv, probe)
		}
	})
	b.Run("IfaceConverted", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = useContract(iv, probe)
		}
	})
}

// ---- 9. does an interface-returning constructor box on every call? --------
//
// Only if the constructor is called every time. A view held once and reused --
// a server holding a view of its own state, say -- boxes once, ever.
func BenchmarkReuse(b *testing.B) {
	d, vw, probe := fixture()

	b.Run("Iface/BuildEachTime", func(b *testing.B) {
		for b.Loop() {
			v := ViewIface[*Item, *Item, string, ItemView](d, vw)
			sinkItemView = useIface(v, probe)
		}
	})
	iv := ViewIface[*Item, *Item, string, ItemView](d, vw)
	b.Run("Iface/BuiltOnce", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = useIface(iv, probe)
		}
	})
	b.Run("Field/BuildEachTime", func(b *testing.B) {
		for b.Loop() {
			v := ViewField[*Item, *Item, string, ItemView](d, vw)
			sinkItemView = useContract(v, probe)
		}
	})
	fv := ViewField[*Item, *Item, string, ItemView](d, vw)
	b.Run("Field/BuiltOnce", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = useContract(fv, probe)
		}
	})
}

// ---- 10. what does the one-word WRAPPER buy over the bare pointer? --------
//
// PtrView is a struct holding a pointer to a body. DirectView is the body, with
// methods on *DirectView. Both are one word and both are pointer-shaped, so the
// question is whether the wrapper earns its extra named type and its extra
// indirection -- v.b.d.Get versus v.d.Get.

type directForm = *DirectView[*Item, *Item, string, ItemView]

var sinkDirect directForm

//go:noinline
func passDirect(v directForm) int { return 1 }

//go:noinline
func useDirect(v directForm, k string) ItemView { r, _ := v.Get(k); return r }

func BenchmarkWrapperVsBarePointer(b *testing.B) {
	d, vw, probe := fixture()
	pv := ViewPtr[*Item, *Item, string, ItemView](d, vw)
	dv := ViewDirect[*Item, *Item, string, ItemView](d, vw)

	b.Run("Construct/Wrapper", func(b *testing.B) {
		for b.Loop() {
			sinkPtr = ViewPtr[*Item, *Item, string, ItemView](d, vw)
		}
	})
	b.Run("Construct/Bare", func(b *testing.B) {
		for b.Loop() {
			sinkDirect = ViewDirect[*Item, *Item, string, ItemView](d, vw)
		}
	})
	// Built and used in one frame: escape analysis should elide both.
	b.Run("Local/Wrapper", func(b *testing.B) {
		for b.Loop() {
			v := ViewPtr[*Item, *Item, string, ItemView](d, vw)
			sinkItemView, sinkBool = v.Get(probe)
		}
	})
	b.Run("Local/Bare", func(b *testing.B) {
		for b.Loop() {
			v := ViewDirect[*Item, *Item, string, ItemView](d, vw)
			sinkItemView, sinkBool = v.Get(probe)
		}
	})
	// Passing to a concrete parameter.
	b.Run("Pass/Wrapper", func(b *testing.B) {
		for b.Loop() {
			sinkInt = passPtr(pv)
		}
	})
	b.Run("Pass/Bare", func(b *testing.B) {
		for b.Loop() {
			sinkInt = passDirect(dv)
		}
	})
	// Calling through a concrete parameter -- where the extra indirection lives.
	b.Run("Call/Wrapper", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = usePtr(pv, probe)
		}
	})
	b.Run("Call/Bare", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = useDirect(dv, probe)
		}
	})
	// Boxing into a contract, then dispatching.
	b.Run("Boxed/Wrapper", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = useContract(pv, probe)
		}
	})
	b.Run("Boxed/Bare", func(b *testing.B) {
		for b.Loop() {
			sinkItemView = useContract(dv, probe)
		}
	})
}

// ---- 11. does a viewer-less view allocate behind a sealed interface? ------
func BenchmarkSealedShallow(b *testing.B) {
	d, _, _ := fixture()
	var sink SealedShallow[*Item, *Item]

	b.Run("Construct", func(b *testing.B) {
		for b.Loop() {
			sink = ViewShallowSealed(d)
		}
	})
	sv := ViewShallowSealed(d)
	b.Run("Call", func(b *testing.B) {
		for b.Loop() {
			_, sinkBool = sv.Get(nil)
		}
	})
	b.Logf("ShallowView width = %d B", sizeOf(ShallowView[*Item, *Item]{d}))
	_ = sink
}
