package iteration

// Proposal D: materialise to a slice, and build everything from slices.
//
// D's bet is that the copy A avoids is cheaper than the yield indirection A
// pays. Two structural consequences make it worth measuring rather than
// assuming:
//
//   - the size stops being plumbed. A slice knows its own length, so SizeHint,
//     CanLen and the Sized constructors have nothing to do.
//   - a source-shaped slice is a full copy the caller owns, so reversal is
//     slices.Reverse and self-referential deletion is safe by construction.
//
// The cost is one allocation and one extra pass per bulk operation, which is
// exactly what these benchmarks price.

import (
	"iter"
	"slices"
)

type KeyValue[K, V any] struct {
	Key   K
	Value V
}

// --- Sources: the same containers as abcost.go, D's methods ---

type udictD struct{ m map[string]int }

func (d *udictD) KeySlice() []string {
	o := make([]string, 0, len(d.m))
	for k := range d.m {
		o = append(o, k)
	}
	return o
}

func (d *udictD) AllSlice() []KeyValue[string, int] {
	o := make([]KeyValue[string, int], 0, len(d.m))
	for k, v := range d.m {
		o = append(o, KeyValue[string, int]{k, v})
	}
	return o
}

type odictD struct{ ks []string }

func (d *odictD) KeySlice() []string { return slices.Clone(d.ks) }

type bigVecD struct{ s []Big }

func (v *bigVecD) ValueSlice() []Big { return slices.Clone(v.s) }

// --- Consumers ---

// newVecD copies, which is the honest default: the constructor cannot know the
// slice it was handed is not retained by the caller.
func newVecD[T any](vs []T) []T {
	o := make([]T, 0, len(vs))
	return append(o, vs...)
}

// newVecDAdopt takes ownership instead. D's contract says a *Slice result is
// already an independent copy, so this is legal for `NewVector(d.KeySlice())`
// and wrong for a slice from anywhere else. Measured to price what the
// distinction is worth.
func newVecDAdopt[T any](vs []T) []T { return vs }

func newSetD[T comparable](vs []T) map[T]struct{} {
	o := make(map[T]struct{}, len(vs))
	for _, v := range vs {
		o[v] = struct{}{}
	}
	return o
}

// --- A-side equivalents that abcost.go does not already provide ---

type HoldsValuesA[T any] interface{ Values() iter.Seq[T] }

type bigVecA struct{ s []Big }

func (v *bigVecA) Len() int            { return len(v.s) }
func (v *bigVecA) Keys() iter.Seq[Big] { return slices.Values(v.s) }

func newSetA[T comparable](cs ...CollA[T]) map[T]struct{} {
	n, known := 0, false
	for _, c := range cs {
		if k, ok := c.SizeHint(); ok {
			n += k
			known = true
		}
	}
	var o map[T]struct{}
	if known {
		o = make(map[T]struct{}, n)
	} else {
		o = map[T]struct{}{}
	}
	for _, c := range cs {
		for v := range c.AsSeq() {
			o[v] = struct{}{}
		}
	}
	return o
}
