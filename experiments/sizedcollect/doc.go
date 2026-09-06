// Package sizedcollect measures what it costs to build a container from an
// iterator that cannot report its length.
//
// iter.Seq is a function; it carries no size. So CollectSortedSet and AddAll
// drain it with a bare append, which reallocates roughly log2(n) times and
// copies about 2n elements in total, where a known length would allocate once.
//
// Three collection strategies are compared: appending with no capacity hint,
// appending into a slice preallocated to the exact length, and cloning a slice
// directly, which is the ceiling since it involves no iterator at all. Each is
// measured alone and then as part of a full sorted-set construction, because
// the sort that follows may well dwarf the difference.
//
// The logic is replicated here rather than imported: experiments are separate
// modules, as in sortedbacking.
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the findings.
package sizedcollect
