// Package reverse prices the ways a container could offer reverse iteration and
// sub-ranges.
//
// The constraint set by ADR 0019 is that neither may be surfaced if the
// implementation has to materialise anything: a Backward() that copies the
// range into a temporary and walks it in reverse is exactly what must not
// ship. So every shape here is O(1) to construct and walks in place, and the
// measurement is about what the SHAPE costs -- bare iterators against a
// composable value -- not about the walk itself.
//
// Three shapes:
//
//	A  method pairs:   Keys(), KeysBackward(), RangeKeys(lo,hi), RangeKeysBackward(lo,hi)
//	B  view-returning: Range(lo,hi) returns a view; views gain Backward()
//	C  span value:     Range(lo,hi) returns a concrete Span; Span has Backward()
//
// The harness is bench_test.go; the findings are RESULTS.md.
package reverse
