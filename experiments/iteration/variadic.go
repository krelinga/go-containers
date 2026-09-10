package iteration

// Does D still need separate single-element and bulk mutators?
//
// ADR 0015 made Vector.Append non-variadic on measurement, and proposal A kept
// the single-element mutators because a Collector for one element cost 49x the
// direct call. Neither argument transfers to proposal D: D has no Collector, so
// the only question left is what a `...T` parameter costs against a `T` one.
//
// If it is free, D collapses Append/AppendAll, Add/AddAll and Delete/DeleteAll
// into one variadic method each, halving the mutator surface.

// Slice-backed, generic -- as the real Vector is.
type vec[T any] struct{ es []T }

func (v *vec[T]) AppendOne(e T)     { v.es = append(v.es, e) }
func (v *vec[T]) AppendVar(es ...T) { v.es = append(v.es, es...) }

// Map-backed, generic -- as the real HashSet is.
type set[T comparable] struct{ m map[T]struct{} }

func (s *set[T]) AddOne(v T) { s.m[v] = struct{}{} }
func (s *set[T]) AddVar(vs ...T) {
	for _, v := range vs {
		s.m[v] = struct{}{}
	}
}

func (s *set[T]) DeleteOne(k T) { delete(s.m, k) }
func (s *set[T]) DeleteVar(ks ...T) {
	for _, k := range ks {
		delete(s.m, k)
	}
}

// Non-inlinable twins. A real container method is larger than these models --
// it dereferences a noCopy receiver, may grow, may check invariants -- so the
// inlined figure is a lower bound on the variadic cost and these are the upper
// bound. The truth for any given method is between them.

//go:noinline
func (v *vec[T]) AppendOneNI(e T) { v.es = append(v.es, e) }

//go:noinline
func (v *vec[T]) AppendVarNI(es ...T) { v.es = append(v.es, es...) }

//go:noinline
func (s *set[T]) AddOneNI(v T) { s.m[v] = struct{}{} }

//go:noinline
func (s *set[T]) AddVarNI(vs ...T) {
	for _, v := range vs {
		s.m[v] = struct{}{}
	}
}
