// Package sortedsetbacking measures whether a sorted set should be built on
// SortedMap[T, struct{}] or on its own []T.
//
// The suspicion worth testing is layout. SortedMap stores sortedEntry{k K; v V},
// which puts a zero-sized field last when V is struct{} — and Go pads a struct
// whose final field is zero-sized, so the entry is twice the width of a bare
// key. That is the same rule that governs where noCopy must be declared.
//
// Three backings are compared: a bare []T, an entry slice with the value last
// (SortedMap's layout today), and an entry slice with the value first. The
// third exists because reordering those fields costs nothing for a normal value
// type but removes the padding entirely for struct{}.
//
// Iteration is measured two ways: directly, and through an iter.Seq2 adapter
// that discards the value, since that is what a SortedSet delegating to
// SortedMap would actually pay.
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the findings.
package sortedsetbacking
