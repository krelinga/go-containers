// Package viewkeys measures what it costs to convert a container's keys through
// a view, and what that buys.
//
// ADR 0011 shipped views that project values only. Keys pass through untouched,
// which is unsound when the key type is mutable — `comparable` admits pointers,
// so HashDict[*Item, V] is legal and its view hands back the raw *Item.
//
// The question is not only safety. A value-only view over a *Item key satisfies
// Dict[*Item, R]: the raw mutable type appears in the contract's own signature,
// so it is part of the API surface of anything accepting that view. Converting
// keys in both directions removes it entirely.
//
// Converting inbound can fail — a projected key need not correspond to a real
// one — so the inbound direction returns (K, bool) and a failure is a miss.
//
// The same fix restores something ADR 0011 recorded as lost: a projecting set
// view whose Has takes the raw element type satisfies only Elems[NT], while one
// converting both directions satisfies Set[NT].
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the findings and
// docs/adr/0012-view-keys.md for the decision they inform.
package viewkeys
