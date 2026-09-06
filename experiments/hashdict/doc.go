// Package hashdict measures the two costs that decide whether a struct-backed
// hash dict is worth adding beside the existing defined map type.
//
// First: what wrapping a map in a struct costs. Map[K, V] is `map[K]V` with
// methods, so it cannot carry noCopy, cannot take pointer receivers, and panics
// on writes to its zero value. A struct wrapper fixes all three, and the
// question is whether the indirection and the lazy-init check show up.
//
// Second: whether a size hint pays. ADR 0006 found that preallocating a slice
// bought only 0-10% end to end, because the sort that followed dominated. A
// hash dict has no sort, so the same question needs asking again rather than
// assuming the answer carries over.
//
// The types are replicated here rather than imported: experiments are separate
// modules.
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the findings and
// docs/adr/0009-hashdict.md for the decision they informed.
package hashdict
