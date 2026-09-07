// Package views measures the shapes a read-only view of a container can take.
//
// ADR 0001 established that views are justified where access crosses an API
// boundary and is called many times, and deferred how one is expressed. This
// measures the candidates rather than arguing about them.
//
// Four questions.
//
// First, what a read-only handle actually prevents. Handing out a contract
// interface does not prevent mutation, because the holder can assert back to
// the concrete container. The escape hatches form tiers, and they are not
// equivalent: the cheap one is invisible in review, the expensive one requires
// importing unsafe.
//
// Second, per-container against per-contract views. One holds a concrete
// pointer, the other an interface, and only one of them is pointer-shaped.
//
// Third, how a view can deny access to a mutable element type. A shallow view
// blocks mutation of the container but not of a *T it hands back. A projection
// closes that, and can be carried either as a struct field or as a type
// parameter whose zero value is materialised per call.
//
// Fourth, whether type aliases recover the ergonomics that the type-parameter
// form costs.
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the findings and
// docs/adr/0011-views.md for the alternatives they inform.
package views
