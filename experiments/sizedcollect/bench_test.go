package sizedcollect

import (
	"fmt"
	"iter"
	"math/rand"
	"slices"
	"testing"
)

var sizes = []int{16, 256, 4096, 65536, 1 << 20}

func source(n int) []int {
	vs := make([]int, n)
	for i := range vs {
		vs[i] = i
	}
	rand.New(rand.NewSource(1)).Shuffle(n, func(i, j int) { vs[i], vs[j] = vs[j], vs[i] })
	return vs
}

func seqOf(vs []int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for _, v := range vs {
			if !yield(v) {
				return
			}
		}
	}
}

var sinkInts []int

// ---- collection alone -----------------------------------------------------

// collectUnknown is what CollectSortedSet does today.
func collectUnknown(seq iter.Seq[int]) []int {
	var vs []int
	for v := range seq {
		vs = append(vs, v)
	}
	return vs
}

// collectKnown is what it could do if the length were available.
func collectKnown(seq iter.Seq[int], n int) []int {
	vs := make([]int, 0, n)
	for v := range seq {
		vs = append(vs, v)
	}
	return vs
}

func BenchmarkCollect(b *testing.B) {
	for _, n := range sizes {
		src := source(n)
		b.Run(fmt.Sprintf("unknown/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkInts = collectUnknown(seqOf(src))
			}
		})
		b.Run(fmt.Sprintf("known/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkInts = collectKnown(seqOf(src), n)
			}
		})
		// No iterator at all: the ceiling.
		b.Run(fmt.Sprintf("clone/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkInts = slices.Clone(src)
			}
		})
	}
}

// ---- full sorted-set construction -----------------------------------------

func sortDistinct(vs []int) []int {
	slices.Sort(vs)
	return slices.Compact(vs)
}

func BenchmarkBuildSortedSet(b *testing.B) {
	for _, n := range sizes {
		src := source(n)
		b.Run(fmt.Sprintf("unknown/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkInts = sortDistinct(collectUnknown(seqOf(src)))
			}
		})
		b.Run(fmt.Sprintf("known/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkInts = sortDistinct(collectKnown(seqOf(src), n))
			}
		})
		b.Run(fmt.Sprintf("clone/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sinkInts = sortDistinct(slices.Clone(src))
			}
		})
	}
}

// ---- how many reallocations does the unknown path actually do? ------------

func TestGrowthSteps(t *testing.T) {
	for _, n := range []int{16, 256, 4096, 65536} {
		var vs []int
		caps, grows := []int{}, 0
		last := 0
		for i := range n {
			vs = append(vs, i)
			if cap(vs) != last {
				last = cap(vs)
				grows++
				if len(caps) < 12 {
					caps = append(caps, last)
				}
			}
		}
		t.Logf("n=%-6d reallocations=%-3d final cap=%-8d (overshoot %+d) caps=%v...",
			n, grows, cap(vs), cap(vs)-n, caps)
	}
}

func BenchmarkNoOp(b *testing.B) {
	src := source(16)
	for b.Loop() {
		sinkInts = src
	}
}
