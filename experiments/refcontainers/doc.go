// Package refcontainers re-measures reference-type containers against the
// library as it stands after ADR 0017, not as it stood when ADR 0014 rejected
// them.
//
// ADR 0014's rejection turned on a chain of five links. One of them has since
// been deleted by unrelated work: link 4 charged struct-shaped views with "an
// allocation whenever a view is passed to Elems2, which is what every sized
// constructor takes". ADR 0017 removed Elems and Elems2 and made every bulk
// operation variadic in its element type, so nothing in the library boxes a
// view into a foreign interface any more.
//
// Two things therefore need pricing again:
//
//   - the container side, against today's method set rather than a toy set
//   - the view side, where the boxing cost is gone and where ADR 0013's erratum
//     (iterating a view allocates three times per call, through a dynamic call)
//     is a cost that struct views would remove rather than add
//
// The harness is bench_test.go; the findings are RESULTS.md.
package refcontainers
