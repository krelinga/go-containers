package viewkeys

import (
	"iter"
	"maps"
	"testing"
	"unsafe"
)

type Item struct{ Name string }
type ItemView struct{ i *Item }

func (v ItemView) Name() string { return v.i.Name }

// ---- contracts, with keys unconstrained -----------------------------------

type Elems2[K, V any] interface {
	Len() int
	All() iter.Seq2[K, V]
}
type Dict[K any, V any] interface {
	Elems2[K, V]
	Get(K) (V, bool)
}

// ---- container ------------------------------------------------------------

type HashDict[K comparable, V any] struct{ m map[K]V }

func (d *HashDict[K, V]) Len() int             { return len(d.m) }
func (d *HashDict[K, V]) Get(k K) (V, bool)    { v, ok := d.m[k]; return v, ok }
func (d *HashDict[K, V]) All() iter.Seq2[K, V] { return maps.All(d.m) }
func (d *HashDict[K, V]) Set(k K, v V)         { d.m[k] = v }

// ---- A: today's view. Values projected, keys pass through. ---------------

type ValueOnlyView[K comparable, V, NV any] struct {
	d *HashDict[K, V]
	f func(V) NV
}

func (v ValueOnlyView[K, V, NV]) Len() int { return v.d.Len() }
func (v ValueOnlyView[K, V, NV]) Get(k K) (NV, bool) {
	raw, ok := v.d.Get(k)
	if !ok {
		var z NV
		return z, false
	}
	return v.f(raw), true
}
func (v ValueOnlyView[K, V, NV]) All() iter.Seq2[K, NV] {
	seq, f := v.d.All(), v.f
	return func(yield func(K, NV) bool) {
		for k, raw := range seq {
			if !yield(k, f(raw)) {
				return
			}
		}
	}
}

// ---- B: proposed. Keys converted out and in, values converted out. -------

type KeyedView[K comparable, V, NK, NV any] struct {
	d      *HashDict[K, V]
	keyOut func(K) NK
	keyIn  func(NK) (K, bool)
	valOut func(V) NV
}

func (v KeyedView[K, V, NK, NV]) Len() int { return v.d.Len() }
func (v KeyedView[K, V, NK, NV]) Get(nk NK) (NV, bool) {
	k, ok := v.keyIn(nk)
	if !ok {
		var z NV
		return z, false
	}
	raw, ok := v.d.Get(k)
	if !ok {
		var z NV
		return z, false
	}
	return v.valOut(raw), true
}
func (v KeyedView[K, V, NK, NV]) All() iter.Seq2[NK, NV] {
	seq, ko, vo := v.d.All(), v.keyOut, v.valOut
	return func(yield func(NK, NV) bool) {
		for k, raw := range seq {
			if !yield(ko(k), vo(raw)) {
				return
			}
		}
	}
}

// ---- the payoff: which of these satisfies a contract? --------------------

// A value-only view over a *Item key satisfies Dict[*Item, ItemView]: the raw
// mutable key is still in the contract's own signature.
var _ Dict[*Item, ItemView] = ValueOnlyView[*Item, string, ItemView]{}

// A keyed view satisfies Dict[NK, NV] -- no mention of the raw types at all.
var _ Dict[string, ItemView] = KeyedView[*Item, string, string, ItemView]{}

var (
	sinkIV ItemView
	sinkB  bool
	sinkI  int
)

func TestSizesAndContracts(t *testing.T) {
	t.Logf("value-only view: %d bytes", unsafe.Sizeof(ValueOnlyView[string, *Item, ItemView]{}))
	t.Logf("keyed view:      %d bytes", unsafe.Sizeof(KeyedView[string, *Item, string, ItemView]{}))
	t.Log("value-only view over a *Item key satisfies Dict[*Item, ...] -- the raw key is in the contract")
	t.Log("keyed view satisfies Dict[string, ...] -- the raw key never appears")
}

// A failed inbound conversion must be a miss, not a panic or a wrong answer.
func TestFailedInbound(t *testing.T) {
	d := &HashDict[string, *Item]{m: map[string]*Item{"a": {Name: "x"}}}
	v := KeyedView[string, *Item, string, ItemView]{
		d:      d,
		keyOut: func(k string) string { return k },
		keyIn:  func(nk string) (string, bool) { return nk, nk != "forbidden" },
		valOut: func(i *Item) ItemView { return ItemView{i} },
	}
	if _, ok := v.Get("a"); !ok {
		t.Error("valid key should hit")
	}
	got, ok := v.Get("forbidden")
	t.Logf("inbound conversion failure -> %v, ok=%v (must be a miss)", got, ok)
	if ok {
		t.Error("failed inbound conversion reported a hit")
	}
}

func BenchmarkGet(b *testing.B) {
	d := &HashDict[string, *Item]{m: map[string]*Item{"a": {Name: "x"}}}
	vo := ValueOnlyView[string, *Item, ItemView]{d, func(i *Item) ItemView { return ItemView{i} }}
	kv := KeyedView[string, *Item, string, ItemView]{
		d:      d,
		keyOut: func(k string) string { return k },
		keyIn:  func(nk string) (string, bool) { return nk, true },
		valOut: func(i *Item) ItemView { return ItemView{i} },
	}
	b.Run("direct", func(b *testing.B) {
		for b.Loop() {
			_, sinkB = d.Get("a")
		}
	})
	b.Run("value-only", func(b *testing.B) {
		for b.Loop() {
			sinkIV, sinkB = vo.Get("a")
		}
	})
	b.Run("keyed", func(b *testing.B) {
		for b.Loop() {
			sinkIV, sinkB = kv.Get("a")
		}
	})
}

func BenchmarkIterate(b *testing.B) {
	d := &HashDict[string, *Item]{m: map[string]*Item{}}
	for i := range 1024 {
		d.Set(string(rune('a'+i%26))+string(rune('a'+i/26)), &Item{})
	}
	vo := ValueOnlyView[string, *Item, ItemView]{d, func(i *Item) ItemView { return ItemView{i} }}
	kv := KeyedView[string, *Item, string, ItemView]{
		d:      d,
		keyOut: func(k string) string { return k },
		keyIn:  func(nk string) (string, bool) { return nk, true },
		valOut: func(i *Item) ItemView { return ItemView{i} },
	}
	b.Run("value-only", func(b *testing.B) {
		for b.Loop() {
			n := 0
			for range vo.All() {
				n++
			}
			sinkI = n
		}
	})
	b.Run("keyed", func(b *testing.B) {
		for b.Loop() {
			n := 0
			for range kv.All() {
				n++
			}
			sinkI = n
		}
	})
}

func BenchmarkNoOp(b *testing.B) {
	d := &HashDict[string, *Item]{m: map[string]*Item{"a": {}}}
	for b.Loop() {
		sinkI = d.Len()
	}
}

// ---- the same fix for sets ------------------------------------------------

type Elems[T any] interface {
	Len() int
	All() iter.Seq[T]
}
type Set[T any] interface {
	Elems[T]
	Has(T) bool
}

type HashSet[T comparable] struct{ m map[T]struct{} }

func (s *HashSet[T]) Len() int         { return len(s.m) }
func (s *HashSet[T]) Has(t T) bool     { _, ok := s.m[t]; return ok }
func (s *HashSet[T]) All() iter.Seq[T] { return func(y func(T) bool) {} }

// Outbound only, as ADR 0011 shipped: Has takes T, All yields NT.
type OutOnlySetView[T comparable, NT any] struct {
	s *HashSet[T]
	f func(T) NT
}

func (v OutOnlySetView[T, NT]) Len() int     { return v.s.Len() }
func (v OutOnlySetView[T, NT]) Has(t T) bool { return v.s.Has(t) }
func (v OutOnlySetView[T, NT]) All() iter.Seq[NT] {
	return func(y func(NT) bool) {}
}

// Both directions: Has takes NT, All yields NT.
type KeyedSetView[T comparable, NT any] struct {
	s   *HashSet[T]
	out func(T) NT
	in  func(NT) (T, bool)
}

func (v KeyedSetView[T, NT]) Len() int { return v.s.Len() }
func (v KeyedSetView[T, NT]) Has(nt NT) bool {
	t, ok := v.in(nt)
	return ok && v.s.Has(t)
}
func (v KeyedSetView[T, NT]) All() iter.Seq[NT] { return func(y func(NT) bool) {} }

// The outbound-only view satisfies only the minimal contract...
var _ Elems[ItemView] = OutOnlySetView[*Item, ItemView]{}

// ...while the two-way view satisfies the full one, which ADR 0011 recorded as
// lost for projecting set views.
var _ Set[ItemView] = KeyedSetView[*Item, ItemView]{}

func TestSetContracts(t *testing.T) {
	t.Log("outbound-only set view satisfies Elems[NT] but NOT Set[NT] (Has takes T)")
	t.Log("two-way set view satisfies Set[NT]: Has takes NT and All yields NT")
}
