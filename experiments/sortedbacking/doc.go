// Package sortedbacking measures what should back a sorted map: a sorted slice
// or a B-tree.
//
// The sorted slice's weakness is insertion — every insert memmoves half the
// backing array on average, so building n elements is O(n^2) in bytes moved. Its
// strengths are lookup, ordered iteration, and range scans, all of which are
// cache-friendly and allocation-free. The question this answers is where the
// crossover is, and whether it sits above or below the sizes a sorted map in
// this library would realistically hold.
//
// Insertion order is measured twice, because it matters more than size: keys
// arriving in ascending order make slice insertion a plain append, while random
// arrival forces the memmove.
//
// The B-tree side is github.com/google/btree rather than a hand-written tree, so
// the comparison is against a mature implementation instead of a strawman. That
// dependency is confined here: experiments are separate modules and never enter
// the library's dependency graph.
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the findings, and
// docs/adr/0003-*.md for the decision they informed.
package sortedbacking
