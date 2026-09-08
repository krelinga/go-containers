// Package iteration measures the shapes an iteration contract can take.
//
// ADR 0017 collects three problems with the library's iteration API. Two of them
// have measurable inputs.
//
// First, a type has one All, so Elems[T] and Elems2[K, V] are mutually
// exclusive: a Vector can be seen as values or as index/value pairs, never
// both. One way out is to give each shape its own method name, which is what
// the stdlib does -- slices.All yields Seq2 and slices.Values yields Seq. The
// other is to keep one method per container and derive the other shapes with
// free functions, as maps.Keys does. That choice turns on what deriving costs,
// which is what this measures.
//
// Second, ordered containers iterate forward only. Whether reverse deserves its
// own method depends partly on whether reversing is cheap over the backings this
// library uses -- a sorted slice, and a vector's slice.
//
// The third problem, inconsistent bulk construction and insertion, is an API
// audit rather than a measurement, and is recorded in the ADR.
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the findings and
// docs/adr/0017-iteration.md for the problems they inform.
package iteration
