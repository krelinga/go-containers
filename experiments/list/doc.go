// Package list measures whether a doubly-linked list earns a place in this
// package, and what it costs to make its cursors safe.
//
// Three questions.
//
// First: does it beat a slice at what it exists for? Removing an element given
// a handle is O(1) for a list and O(n) for a slice, and the crossover matters.
//
// Second: how much does pointer chasing cost on iteration? This is the standard
// objection to linked lists. It is measured twice — once with a raw loop and
// once through an iter.Seq closure — because this package's containers are only
// ever iterated through All(), and the answer differs sharply between the two.
//
// Third: what does a back-pointer from each node to its owning list cost? It
// makes a stale or foreign cursor detectable. A cheaper tombstone — pointing a
// removed node's prev at itself — is measured alongside it, because it catches
// staleness for free and the difference between the two is exactly the foreign-
// cursor case.
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the findings and
// docs/adr/0010-list.md for the decision they informed.
package list
