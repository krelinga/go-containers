// Package witness re-prices the type-parameter witness against the shape the
// library actually ships, on the operation nobody measured at the time.
//
// ADR 0011 found the witness the cheapest view shape and measured Get. ADR 0012
// closed it off because a witness must be STATELESS and FromKeyView generally
// needs a registry. ADR 0013 then found that iterating a view allocates three
// times per call -- an erratum that is still live, and that neither 0011 nor
// 0012 had in view when they weighed the witness.
//
// Two witness variants matter, and they are not the same thing:
//
//	pure     struct{ c C }        + type param VW, zero value materialised per call
//	         one word; stateless viewers only -- this is what 0012 rejected
//
//	carried  struct{ c C; vw VW } with VW a CONCRETE type parameter
//	         static dispatch AND stateful viewers; width is 1 + sizeof(VW)
//
// A later question joined it: if iterating through an interface costs a fixed
// ~34 ns and three-to-six allocations, is there an ESCAPE HATCH a caller can
// reach for where that matters? each_test.go and alt_test.go price
//
//	Each(f func(T) bool)   // container drives; false stops. sync.Map.Range's shape
//
// against the Go iteration paradigm, and against the two workarounds callers
// already have: hoisting the iter.Seq out of the loop, and materialising with
// KeySlice.
//
// The harness is bench_test.go, each_test.go and alt_test.go; the findings are
// RESULTS.md.
package witness
