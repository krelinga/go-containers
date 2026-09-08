// Package reftypes measures what it costs to make a container a reference type.
//
// ADR 0002 fixed the shape of a container: a struct carrying noCopy, uniform
// pointer receivers, held and passed as *HashSet[T]. Copying is forbidden and
// go vet's copylocks enforces it. Map is the exception, being a defined
// map[K]V and therefore already a reference type.
//
// The proposal is to make every container a reference type -- a small struct
// wrapping a pointer to shared state, copied freely, copies sharing -- so that
// containers behave like builtin maps and like the view interfaces ADR 0013
// introduced. That raises three questions this measures.
//
// First, what the indirection costs. A reference container reads s.st.m where
// today's reads s.m, which looks like an extra hop and may not be one.
//
// Second, what the alternative representation costs. A sealed interface is also
// a reference type, is comparable to nil where a struct is not, and removes ADR
// 0002's set-algebra ceiling -- but makes every container operation dynamic,
// where ADR 0013 accepted that only at boundaries.
//
// Third, what nil means. Builtin maps read clean when nil and panic on write.
// Reproducing that needs a nil check in every read method, which ADR 0002 bans.
// This measures whether the check is free, and reports which representations can
// reproduce the semantics at all -- because one of them cannot.
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the findings and
// docs/adr/0014-reference-containers.md for the alternatives they inform.
package reftypes
