// Package vectorcost measures what a container wrapper costs a slice.
//
// Every container in this library so far wraps a map or a sorted slice, and the
// dispatch and hashdict experiments established that a method wrapper over a
// map operation inlines away entirely. A sequence is a harder case for two
// reasons that do not arise for maps.
//
// First, bounds-check elimination. The compiler can prove i < len(s) inside
// `for i := range s` and drop the check. Through a wrapper it sees i < v.Len()
// and an index into a different expression, and may not be able to. A map has
// no bounds check to lose.
//
// Second, proportion. A map lookup is ~3 ns, so a nanosecond of overhead is
// noise. A slice index is well under a nanosecond and raw iteration is around
// one nanosecond per element, so the same absolute overhead is enormous as a
// share. The linkedlist experiment already found that iterator abstraction
// costs ~6x raw ranging; against a slice that is the dominant term.
//
// Third, and the reason a Vector is proposed at all: what the wrapper buys. A
// slice header is a value, so a reallocating append is visible to callers as
// divergence, and an accessor returning []T hands out a mutable interior. This
// measures the cost side; the aliasing report shows the benefit side.
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the findings and
// docs/adr/0015-vector.md for the decision they inform.
package vectorcost
