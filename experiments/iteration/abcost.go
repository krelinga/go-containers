package iteration

// Proposal A's boxed Collector against proposal B's Source value.
//
// ADR 0017 proposal A hands bulk operations a sealed interface; proposal B
// hands them a concrete struct the container returns directly. A's case is that
// capability lives in the type system; B's case is that a value is cheaper than
// a box. This file models both faithfully enough to price the second claim.
//
// The nuance worth isolating: A's collector is one closure plus one interface
// box. B's Source is no box but carries TWO closures when the container is
// ordered, because reversibility is a field rather than a type. So B should win
// on unordered containers and may lose on ordered ones.

import (
	"iter"
	"slices"
)

// --- Proposal A ---

type CollA[T any] interface {
	SizeHint() (int, bool)
	AsSeq() iter.Seq[T]
	sealedA()
}

type collA[T any] struct {
	n     int
	known bool
	seq   iter.Seq[T]
}

func (c collA[T]) SizeHint() (int, bool) { return c.n, c.known }
func (c collA[T]) AsSeq() iter.Seq[T]    { return c.seq }
func (c collA[T]) sealedA()              {}

type HoldsKeysA[T any] interface{ Keys() iter.Seq[T] }

type CanLenA interface{ Len() int }

func KeysOfA[T any](h HoldsKeysA[T]) CollA[T] {
	n, ok := 0, false
	if l, is := h.(CanLenA); is {
		n, ok = l.Len(), true
	}
	return collA[T]{n, ok, h.Keys()}
}

func ItemsA[T any](vs ...T) CollA[T] { return collA[T]{len(vs), true, slices.Values(vs)} }

func newVecA[T any](cs ...CollA[T]) []T {
	n, known := 0, false
	for _, c := range cs {
		if k, ok := c.SizeHint(); ok {
			n += k
			known = true
		}
	}
	var o []T
	if known {
		o = make([]T, 0, n)
	}
	for _, c := range cs {
		for v := range c.AsSeq() {
			o = append(o, v)
		}
	}
	return o
}

// --- Proposal B ---

type SrcB[T any] struct {
	Seq   iter.Seq[T]
	Back  iter.Seq[T]
	N     int
	Known bool
}

func ItemsB[T any](vs ...T) SrcB[T] {
	return SrcB[T]{Seq: slices.Values(vs), Back: backwardOf(vs), N: len(vs), Known: true}
}

// ItemsBFwd is the same source with no reverse half, to isolate what the second
// closure costs.
func ItemsBFwd[T any](vs ...T) SrcB[T] {
	return SrcB[T]{Seq: slices.Values(vs), N: len(vs), Known: true}
}

func backwardOf[T any](vs []T) iter.Seq[T] {
	return func(y func(T) bool) {
		for i := len(vs) - 1; i >= 0; i-- {
			if !y(vs[i]) {
				return
			}
		}
	}
}

func newVecB[T any](ss ...SrcB[T]) []T {
	n, known := 0, false
	for _, s := range ss {
		if s.Known {
			n += s.N
			known = true
		}
	}
	var o []T
	if known {
		o = make([]T, 0, n)
	}
	for _, s := range ss {
		for v := range s.Seq {
			o = append(o, v)
		}
	}
	return o
}

// --- Containers, one per proposal, unordered and ordered ---

type udictA struct{ m map[string]int }

func (d *udictA) Len() int { return len(d.m) }
func (d *udictA) Keys() iter.Seq[string] {
	return func(y func(string) bool) {
		for k := range d.m {
			if !y(k) {
				return
			}
		}
	}
}

type udictB struct{ m map[string]int }

// Unordered: Back stays nil, so B pays one closure where A pays a closure plus
// a box.
func (d *udictB) Keys() SrcB[string] {
	return SrcB[string]{
		Seq: func(y func(string) bool) {
			for k := range d.m {
				if !y(k) {
					return
				}
			}
		},
		N:     len(d.m),
		Known: true,
	}
}

type odictA struct{ ks []string }

func (d *odictA) Len() int               { return len(d.ks) }
func (d *odictA) Keys() iter.Seq[string] { return slices.Values(d.ks) }

type odictB struct{ ks []string }

// Ordered: Back is populated, so B pays two closures and no box.
func (d *odictB) Keys() SrcB[string] {
	return SrcB[string]{
		Seq:   slices.Values(d.ks),
		Back:  backwardOf(d.ks),
		N:     len(d.ks),
		Known: true,
	}
}

func fixtureKeys(n int) []string {
	ks := make([]string, n)
	for i := range ks {
		ks[i] = string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	return ks
}
