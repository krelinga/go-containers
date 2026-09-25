// Package sealing prices the three shapes a read-only view can take, and asks
// what it costs to make "read-only" a COMPILE-TIME guarantee rather than a
// documented convention.
//
// ADR 0018 ships views as a concrete struct wrapping a shape interface. ADR
// 0020 proposed replacing them with a carried witness or with bare interfaces.
// The question nobody had priced is the simplest one: is the struct wrapper
// actually slower than the interface it wraps?
//
//	StructView   struct{ impl inner[NT] }   two words -- what 0018 ships
//	IfaceView    interface{ …; viewOnly() } sealed interface -- 0020's shape
//	PtrView      struct{ st *state }        one word, state behind a pointer
//
// The second question is the guarantee. The shape interfaces in contracts.go are
// deliberately unsealed, so a CONTAINER placed in one can be asserted back out
// and written through. seal_test.go enumerates every route out of a sealed
// interface, including the two that look like they work and do not.
//
// WARNING for anyone extending this harness: an interface with ONE visible
// implementation assigned to a local is devirtualized, and every
// dynamic-dispatch measurement through it reads as free. Every shape here keeps
// a second implementation behind a runtime-selected branch for that reason.
//
// A third question arrived with ADR 0023: a converting constructor takes the
// concrete CONTAINER, so a view cannot be narrowed or re-converted by whoever
// holds it. compose.go prices the two source-parameter shapes that would fix
// that -- the concrete view type, and the sealed shape interface -- at one, two
// and three levels deep.
//
// The harness is bench_test.go, seal_test.go and compose_test.go; the findings
// are RESULTS.md.
package sealing
