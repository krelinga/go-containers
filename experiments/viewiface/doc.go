// Package viewiface measures what it costs to hide a view behind an interface.
//
// ADR 0012 left every view a concrete struct carrying a viewer in a field. That
// makes a view type spell out the container's own key and value types alongside
// the converted ones -- HashDictView[K, V, NK, NV] -- even though a consumer
// cares about only the converted pair. An interface would spell as
// DictView[NK, NV].
//
// The prior experiments already settled the pieces this builds on: interface
// dispatch costs about 0.7 ns (dispatch), and a struct holding an interface
// field is not pointer-shaped, so it costs an allocation every time it is
// passed as a contract (views). The open question is what those two facts add
// up to across a realistic call shape.
//
// Three questions.
//
// First, where the crossover lies. A concrete view re-boxes at every boundary
// it crosses; an interface-typed view boxes once, at construction, and is free
// to pass thereafter. Counting boundaries decides which is cheaper, and the
// answer is not the one the framing "interfaces are slower" suggests.
//
// Second, whether escape analysis rescues the concrete form. A box that does
// not outlive its frame can live on the stack, which would make the concrete
// view's boundary crossing free in leaf code. This measures inlinable and
// non-inlinable consumers separately, because the answer differs.
//
// Third, whether a pointer-shaped view is a third option: it boxes for free
// like the type-parameter witness in the views experiment, but unlike that
// witness it can carry a stateful viewer, which ADR 0012's FromKeyView needs.
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the findings and
// docs/adr/0013-view-interfaces.md for the alternatives they inform.
package viewiface
