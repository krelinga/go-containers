package containers

// A viewer supplies the conversions a view applies to what it hands back. The
// interfaces come in two families.
//
// **Capabilities** describe what a viewer can do. Implement these:
//
//   - KeyViewer converts keys out of a container and back in.
//   - ValueViewer converts values out.
//
// **Requirements** describe what one constructor needs, and are named for the
// operation. Do not implement these directly; a viewer satisfies them by
// implementing the capabilities they compose. Their purpose is the error
// message: failing to satisfy CanViewHashDict says which operation you cannot
// perform, rather than which structural property you lack.
//
// Write the halves separately and compose by embedding. A stateless composed
// viewer is zero bytes:
//
//	type releaseKeys struct{}   // ToKeyView, FromKeyView
//	type releaseValues struct{} // ToValueView
//
//	type releaseViewer struct {
//		releaseKeys
//		releaseValues
//	}
//
// The library cannot supply a projection for your types: for a key of *Item,
// somebody has to write the read-only type and both directions of its
// conversion. See ADR 0012.

// KeyViewer converts a container's keys to the type a view exposes, and back.
//
// ToKeyView is total: every key in the container has a view form. FromKeyView is
// not — a view key need not correspond to a real one — so it reports whether the
// conversion succeeded. A failure reads as a miss wherever a view uses it, never
// as a panic or a fabricated key.
//
// The success flag exists so that an implementation never has to invent a key it
// cannot produce.
type KeyViewer[K, NK any] interface {
	ToKeyView(K) NK
	FromKeyView(NK) (K, bool)
}

// ValueViewer converts a container's values to the type a view exposes.
//
// There is no inbound direction: a view is read-only, so no value travels
// inward.
type ValueViewer[V, NV any] interface {
	ToValueView(V) NV
}

// CanViewHashSet is what ViewHashSet requires.
type CanViewHashSet[T, NT any] interface {
	KeyViewer[T, NT]
}

// CanViewHashDict is what ViewHashDict requires.
type CanViewHashDict[K, NK, V, NV any] interface {
	KeyViewer[K, NK]
	ValueViewer[V, NV]
}

// CanViewMap is what ViewMap requires.
type CanViewMap[K, NK, V, NV any] interface {
	KeyViewer[K, NK]
	ValueViewer[V, NV]
}

// CanViewSortedDict is what ViewSortedDict requires.
//
// Values only. A sorted container's keys are cmp.Ordered, which admits only
// integers, floats and strings — every one an immutable value type — so there is
// nothing for key conversion to protect. See ADR 0012, decision 3.
type CanViewSortedDict[V, NV any] interface {
	ValueViewer[V, NV]
}

// IdentityViewer converts nothing, in both directions and on both halves.
//
// The Identity constructors do not use it: ADR 0013 gives them a viewer-less
// representation that is one word and allocates nothing. It is here for
// composing a *partial* identity — a viewer that converts keys but passes values
// through, or the reverse — by embedding the half you do not want to write.
type IdentityViewer[K, V any] struct{}

func (IdentityViewer[K, V]) ToKeyView(k K) K           { return k }
func (IdentityViewer[K, V]) FromKeyView(k K) (K, bool) { return k, true }
func (IdentityViewer[K, V]) ToValueView(v V) V         { return v }

// IdentityValueViewer converts nothing, and offers only the value half.
//
// Embed it in a viewer that converts keys but should pass values through. As
// with IdentityViewer, ViewSortedDictIdentity does not use it.
type IdentityValueViewer[V any] struct{}

func (IdentityValueViewer[V]) ToValueView(v V) V { return v }

// CanViewVector is what ViewVector requires.
//
// Values only. A vector is indexed by int, and an index is not a key the caller
// supplied — it is a position the container assigned — so there is nothing for
// an inbound conversion to translate and no membership question to answer. At
// takes an int on both sides of the view.
type CanViewVector[T, NT any] interface {
	ValueViewer[T, NT]
}

// CanViewSlice is what ViewSlice requires.
//
// Values only, for the reason given on CanViewVector: a slice is indexed by int,
// and an index is a position rather than a key the caller supplied.
//
// It is structurally identical to CanViewVector and separate from it on purpose.
// These interfaces are named for the constructor that requires them, so that
// failing to satisfy one names the operation you cannot perform rather than the
// structural property you lack. CanViewHashDict and CanViewMap are identical to
// each other on the same grounds. See ADR 0012.
type CanViewSlice[T, NT any] interface {
	ValueViewer[T, NT]
}
