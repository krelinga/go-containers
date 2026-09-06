package list

import (
	"fmt"
	"iter"
	"runtime"
	"slices"
	"testing"
	"unsafe"
)

var (
	sinkI int
	sinkS []int
)

// ---- two node layouts -----------------------------------------------------

// nodeA has no back-pointer. A stale cursor is undetectable.
type nodeA[V any] struct {
	prev, next *nodeA[V]
	v          V
}
type listA[V any] struct {
	head, tail *nodeA[V]
	len        int
}
type curA[V any] struct{ n *nodeA[V] }

func (l *listA[V]) get(c curA[V]) V { return c.n.v }

func (l *listA[V]) push(v V) curA[V] {
	n := &nodeA[V]{v: v, prev: l.tail}
	if l.tail != nil {
		l.tail.next = n
	} else {
		l.head = n
	}
	l.tail = n
	l.len++
	return curA[V]{n}
}

func (l *listA[V]) remove(c curA[V]) {
	n := c.n
	if n.prev != nil {
		n.prev.next = n.next
	} else {
		l.head = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	} else {
		l.tail = n.prev
	}
	l.len--
}

// insertBefore is Remove's inverse, so a benchmark can measure the pair without
// rebuilding its fixture inside the timed loop.
func (l *listA[V]) insertBefore(n, at *nodeA[V]) {
	n.prev, n.next = at.prev, at
	if at.prev != nil {
		at.prev.next = n
	} else {
		l.head = n
	}
	at.prev = n
	l.len++
}

// nodeB carries a back-pointer to its owning list, so both a stale cursor
// (list nilled on removal) and a foreign one (list differs) are detectable.
type nodeB[V any] struct {
	prev, next *nodeB[V]
	list       *listB[V]
	v          V
}
type listB[V any] struct {
	head, tail *nodeB[V]
	len        int
}
type curB[V any] struct{ n *nodeB[V] }

func (l *listB[V]) get(c curB[V]) V {
	if c.n.list != l {
		panic("containers: stale or foreign cursor")
	}
	return c.n.v
}

func (l *listB[V]) push(v V) curB[V] {
	n := &nodeB[V]{v: v, prev: l.tail, list: l}
	if l.tail != nil {
		l.tail.next = n
	} else {
		l.head = n
	}
	l.tail = n
	l.len++
	return curB[V]{n}
}

// nodeC has no back-pointer but tombstones a removed node by pointing its prev
// at itself, which a live node can never satisfy.
type nodeC[V any] struct {
	prev, next *nodeC[V]
	v          V
}
type listC[V any] struct {
	head, tail *nodeC[V]
	len        int
}
type curC[V any] struct{ n *nodeC[V] }

func (l *listC[V]) get(c curC[V]) V {
	if c.n.prev == c.n {
		panic("containers: stale cursor")
	}
	return c.n.v
}

func (l *listC[V]) push(v V) curC[V] {
	n := &nodeC[V]{v: v, prev: l.tail}
	if l.tail != nil {
		l.tail.next = n
	} else {
		l.head = n
	}
	l.tail = n
	l.len++
	return curC[V]{n}
}

func (l *listC[V]) remove(c curC[V]) {
	n := c.n
	if n.prev != nil {
		n.prev.next = n.next
	} else {
		l.head = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	} else {
		l.tail = n.prev
	}
	n.prev, n.next = n, nil // tombstone
	l.len--
}

func buildA(n int) (*listA[int], []curA[int]) {
	l := &listA[int]{}
	cs := make([]curA[int], n)
	for i := range n {
		cs[i] = l.push(i)
	}
	return l, cs
}

func buildB(n int) (*listB[int], []curB[int]) {
	l := &listB[int]{}
	cs := make([]curB[int], n)
	for i := range n {
		cs[i] = l.push(i)
	}
	return l, cs
}

// ---- 1. removal, which is the reason the container exists -----------------

// Remove the middle element and splice it back: symmetric for both structures,
// so no fixture rebuild is needed inside the timed loop.
func BenchmarkRemoveReinsertMiddle(b *testing.B) {
	for _, n := range []int{64, 1024, 16384, 262144} {
		l, cs := buildA(n)
		target := cs[n/2].n
		after := target.next

		s := make([]int, n)
		for i := range s {
			s[i] = i
		}
		mid := n / 2

		b.Run(fmt.Sprintf("list/%d", n), func(b *testing.B) {
			for b.Loop() {
				l.remove(curA[int]{target})
				l.insertBefore(target, after)
			}
			sinkI = l.len
		})
		b.Run(fmt.Sprintf("slice/%d", n), func(b *testing.B) {
			for b.Loop() {
				v := s[mid]
				s = slices.Delete(s, mid, mid+1)
				s = slices.Insert(s, mid, v)
			}
			sinkS = s
		})
	}
}

// ---- 2. iteration, raw and through an iterator ----------------------------

func (l *listA[V]) all() iter.Seq2[curA[V], V] {
	return func(yield func(curA[V], V) bool) {
		for n := l.head; n != nil; n = n.next {
			if !yield(curA[V]{n}, n.v) {
				return
			}
		}
	}
}

func seqOf(s []int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for _, v := range s {
			if !yield(v) {
				return
			}
		}
	}
}

// BenchmarkIterate compares raw against raw and iterator against iterator. The
// pairing matters: a raw slice range against a list's iter.Seq measures the
// iterator, not the pointer chasing.
func BenchmarkIterate(b *testing.B) {
	for _, n := range []int{1024, 262144} {
		l, _ := buildA(n)
		s := make([]int, n)
		for i := range s {
			s[i] = i
		}
		b.Run(fmt.Sprintf("raw-list/%d", n), func(b *testing.B) {
			for b.Loop() {
				t := 0
				for nd := l.head; nd != nil; nd = nd.next {
					t += nd.v
				}
				sinkI = t
			}
		})
		b.Run(fmt.Sprintf("raw-slice/%d", n), func(b *testing.B) {
			for b.Loop() {
				t := 0
				for _, v := range s {
					t += v
				}
				sinkI = t
			}
		})
		b.Run(fmt.Sprintf("seq-list/%d", n), func(b *testing.B) {
			for b.Loop() {
				t := 0
				for _, v := range l.all() {
					t += v
				}
				sinkI = t
			}
		})
		b.Run(fmt.Sprintf("seq-slice/%d", n), func(b *testing.B) {
			for b.Loop() {
				t := 0
				for v := range seqOf(s) {
					t += v
				}
				sinkI = t
			}
		})
	}
}

// ---- 3. what cursor validation costs --------------------------------------

func BenchmarkGetValidation(b *testing.B) {
	la, ca := buildA(1024)
	lb, cb := buildB(1024)
	lc := &listC[int]{}
	var cc curC[int]
	for i := range 1024 {
		c := lc.push(i)
		if i == 512 {
			cc = c
		}
	}
	mid := 512

	b.Run("unchecked", func(b *testing.B) {
		for b.Loop() {
			sinkI = la.get(ca[mid])
		}
	})
	b.Run("tombstone", func(b *testing.B) {
		for b.Loop() {
			sinkI = lc.get(cc)
		}
	})
	b.Run("backpointer", func(b *testing.B) {
		for b.Loop() {
			sinkI = lb.get(cb[mid])
		}
	})
}

// The cost that recurs: an extra pointer per node is re-scanned every GC cycle.
func BenchmarkGCCycle(b *testing.B) {
	const n = 200000
	b.Run("retained=none", func(b *testing.B) {
		runtime.GC()
		for b.Loop() {
			runtime.GC()
		}
	})
	b.Run("retained=2ptr", func(b *testing.B) {
		l, cs := buildA(n)
		runtime.GC()
		for b.Loop() {
			runtime.GC()
		}
		b.StopTimer()
		runtime.KeepAlive(l)
		runtime.KeepAlive(cs)
	})
	b.Run("retained=3ptr", func(b *testing.B) {
		l, cs := buildB(n)
		runtime.GC()
		for b.Loop() {
			runtime.GC()
		}
		b.StopTimer()
		runtime.KeepAlive(l)
		runtime.KeepAlive(cs)
	})
}

func BenchmarkBuild(b *testing.B) {
	for _, n := range []int{1024, 65536} {
		b.Run(fmt.Sprintf("2ptr/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				l, _ := buildA(n)
				sinkI = l.len
			}
		})
		b.Run(fmt.Sprintf("3ptr/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				l, _ := buildB(n)
				sinkI = l.len
			}
		})
	}
}

func BenchmarkNoOp(b *testing.B) {
	l, _ := buildA(64)
	for b.Loop() {
		sinkI = l.len
	}
}

// ---- layout and detection, reported rather than asserted ------------------

func TestNodeLayout(t *testing.T) {
	t.Logf("2ptr node, V=int:     %d bytes", unsafe.Sizeof(nodeA[int]{}))
	t.Logf("3ptr node, V=int:     %d bytes", unsafe.Sizeof(nodeB[int]{}))
	t.Logf("2ptr node, V=string:  %d bytes", unsafe.Sizeof(nodeA[string]{}))
	t.Logf("3ptr node, V=string:  %d bytes", unsafe.Sizeof(nodeB[string]{}))
}

// TestDetection records what each mechanism catches. The tombstone catches
// staleness for free; only the back-pointer catches a cursor from another list.
func TestDetection(t *testing.T) {
	caught := func(f func()) (c bool) {
		defer func() { c = recover() != nil }()
		f()
		return
	}

	lc := &listC[int]{}
	c1 := lc.push(1)
	lc.push(2)
	lc.remove(c1)
	t.Logf("tombstone,  stale cursor:   caught=%v", caught(func() { lc.get(c1) }))

	otherC := &listC[int]{}
	oc := otherC.push(99)
	t.Logf("tombstone,  foreign cursor: caught=%v", caught(func() { lc.get(oc) }))

	lb, cb := buildB(2)
	lb.list3Remove(cb[0])
	t.Logf("backpointer, stale cursor:   caught=%v", caught(func() { lb.get(cb[0]) }))

	otherB, ob := buildB(1)
	_ = otherB
	t.Logf("backpointer, foreign cursor: caught=%v", caught(func() { lb.get(ob[0]) }))
}

func (l *listB[V]) list3Remove(c curB[V]) {
	n := c.n
	if n.prev != nil {
		n.prev.next = n.next
	} else {
		l.head = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	} else {
		l.tail = n.prev
	}
	n.prev, n.next, n.list = nil, nil, nil // release, and mark stale
	l.len--
}
