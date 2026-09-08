// Package sliceadapter measures what a defined slice type can and cannot do.
//
// ADR 0009 makes Map[K, V] a defined map[K]V: an adapter, reached for when a
// caller wants free conversion from the builtin, builtin syntax, or working
// encoding/json. ADR 0015 records a follow-up proposing the same for slices.
//
// The analogy has a wall in it, and this locates it. A map is a reference type,
// so Map's Set and Delete work on a value receiver — the header points at
// shared state. A slice's length lives in the header, which IS the value, so a
// value receiver cannot grow one. Whatever a slice adapter is, it is not the
// mutable equivalent of Map.
//
// Three questions beyond that.
//
// First, what an adapter buys the sized constructors. ADR 0015 made
// Vector.Append non-variadic, so the only ways to bulk-append a plain slice are
// through a length-carrying Elems or through a bare iterator that has none.
// ADR 0006 says the first should win; this measures by how much.
//
// Second, whether a defined slice type keeps bounds-check elimination.
// experiments/vectorcost found a struct wrapper loses it, costing ~24% on an
// indexed loop. A defined slice type can still be indexed directly, so it may
// not.
//
// Third, what a view over one costs. A slice header is three words, so a view
// holding one by value is not pointer-shaped and boxes with an allocation,
// where Map's view holds a one-word map header and does not.
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the findings and
// docs/adr/0016-slice.md for the decision they inform.
package sliceadapter
