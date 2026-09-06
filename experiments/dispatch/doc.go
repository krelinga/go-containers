// Package dispatch measures what generic programming over container interfaces
// actually costs, and compares it against a concrete API decision it might have
// been assumed to dwarf.
//
// The occasion was Map[K, V]'s Delete signature. SortedMap.Delete reports
// whether the key was present; the builtin delete does not, so a map-backed
// Delete with the same signature needs an extra lookup. The question was
// whether that lookup matters next to the interface dispatch such code performs
// anyway.
//
// Three call paths are compared for each operation: a direct call on the
// concrete type, a call through an interface value, and a call through a
// generic function whose type parameter is constrained by that interface. The
// raw builtin is included as the floor.
//
// The interface value is held in a package-level slice so the compiler cannot
// see its dynamic type. Without that it devirtualises the call and the
// interface numbers become fiction.
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the findings and
// docs/adr/0007-map.md for the decision they informed.
package dispatch
