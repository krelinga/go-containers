package views

import (
	"cmp"
	"iter"
	"maps"
	"testing"
)

// ---- an element type that is mutable through a pointer -------------------

type Item struct{ Name string }

// ItemView is an element-level read-only projection. The library cannot invent
// one of these; only the caller who owns Item can write it.
type ItemView struct{ i *Item }

func (v ItemView) Name() string { return v.i.Name }

// ---- a stand-in container -------------------------------------------------

type Dict[K cmp.Ordered, V any] struct{ m map[K]V }

func (d *Dict[K, V]) Get(k K) (V, bool)    { v, ok := d.m[k]; return v, ok }
func (d *Dict[K, V]) Len() int             { return len(d.m) }
func (d *Dict[K, V]) All() iter.Seq2[K, V] { return maps.All(d.m) }
func (d *Dict[K, V]) Set(k K, v V)         { d.m[k] = v }

// The read-only contract a view is meant to satisfy.
type ReadDict[K cmp.Ordered, V any] interface {
	Len() int
	Get(K) (V, bool)
	All() iter.Seq2[K, V]
}

// ---- shape 1: shallow view, one concrete pointer -------------------------

type ShallowView[K cmp.Ordered, V any] struct{ d *Dict[K, V] }

func (v ShallowView[K, V]) Len() int             { return v.d.Len() }
func (v ShallowView[K, V]) Get(k K) (V, bool)    { return v.d.Get(k) }
func (v ShallowView[K, V]) All() iter.Seq2[K, V] { return v.d.All() }

func (d *Dict[K, V]) View() ShallowView[K, V] { return ShallowView[K, V]{d} }

// ---- shape 2: per-contract view, holding an interface --------------------

type ContractView[K cmp.Ordered, V any] struct{ d ReadDict[K, V] }

func (v ContractView[K, V]) Len() int             { return v.d.Len() }
func (v ContractView[K, V]) Get(k K) (V, bool)    { return v.d.Get(k) }
func (v ContractView[K, V]) All() iter.Seq2[K, V] { return v.d.All() }

func SealDict[K cmp.Ordered, V any](d ReadDict[K, V]) ContractView[K, V] {
	return ContractView[K, V]{d}
}

// ---- shape 3: projecting view carrying the conversion as a field ---------

type FieldProjView[K cmp.Ordered, V, R any] struct {
	d *Dict[K, V]
	f func(V) R
}

func (v FieldProjView[K, V, R]) Len() int { return v.d.Len() }
func (v FieldProjView[K, V, R]) Get(k K) (R, bool) {
	raw, ok := v.d.Get(k)
	if !ok {
		var z R
		return z, false
	}
	return v.f(raw), true
}

// A free function, because a method cannot introduce R.
func ViewFunc[K cmp.Ordered, V, R any](d *Dict[K, V], f func(V) R) FieldProjView[K, V, R] {
	return FieldProjView[K, V, R]{d, f}
}

// ---- shape 4: projecting view carrying the conversion as a TYPE ----------

type Converter[V, R any] interface{ Convert(V) R }

type WitnessView[K cmp.Ordered, V, R any, C Converter[V, R]] struct{ d *Dict[K, V] }

func (v WitnessView[K, V, R, C]) Len() int { return v.d.Len() }
func (v WitnessView[K, V, R, C]) Get(k K) (R, bool) {
	raw, ok := v.d.Get(k)
	if !ok {
		var z R
		return z, false
	}
	var conv C // zero value; the converter must be stateless
	return conv.Convert(raw), true
}

type itemConv struct{}

func (itemConv) Convert(i *Item) ItemView { return ItemView{i} }

type idConv[V any] struct{}

func (idConv[V]) Convert(v V) V { return v }

// ---- aliases, which is how the type-parameter form recovers ergonomics ----

// Generic alias, partially applied: the identity case leaves K and V free.
type WitnessShallow[K cmp.Ordered, V any] = WitnessView[K, V, V, idConv[V]]

// Fully instantiated alias, naming a projecting view once.
type ItemDictView = WitnessView[string, *Item, ItemView, itemConv]

// A method may return a generic alias -- it introduces no new type parameter.
func (d *Dict[K, V]) WitnessView() WitnessShallow[K, V] {
	return WitnessShallow[K, V]{d}
}

// ===========================================================================
// Measurements
// ===========================================================================

type anyView interface{ Len() int }

var (
	sinkAny  anyView
	sinkItem *Item
	sinkIV   ItemView
	sinkB    bool
	sinkI    int
)

func fixture() *Dict[string, *Item] {
	return &Dict[string, *Item]{m: map[string]*Item{"a": {Name: "orig"}}}
}

// TestEscapeHatches records what a read-only handle actually prevents. The
// tiers are not equivalent: only the first is invisible in review.
func TestEscapeHatches(t *testing.T) {
	d := fixture()

	// Handing out the bare contract: the holder can assert back and mutate.
	var bare ReadDict[string, *Item] = d
	if c, ok := bare.(*Dict[string, *Item]); ok {
		c.Set("b", &Item{})
		t.Logf("bare contract, type assert to container:  SUCCEEDED (Len now %d)", d.Len())
	}

	// Handing out a view: the dynamic type is the view, not the container.
	var sealed anyView = d.View()
	_, ok := sealed.(*Dict[string, *Item])
	t.Logf("view, type assert to container:            %v", ok)

	t.Log("view, reflect read of unexported field:   succeeds (see RESULTS.md)")
	t.Log("view, reflect write through that field:   blocked by reflect itself")
	t.Log("view, reflect+unsafe:                     succeeds; requires importing unsafe")
}

// TestShallowLeaks is the reason projection is on the table at all.
func TestShallowLeaks(t *testing.T) {
	d := fixture()

	item, _ := d.View().Get("a")
	item.Name = "MUTATED" // through a read-only view
	got, _ := d.Get("a")
	t.Logf("shallow view: element mutated through it?  %v (Name=%q)", got.Name == "MUTATED", got.Name)

	pv := ItemDictView{d}
	iv, _ := pv.Get("a")
	t.Logf("projecting view hands back %T, which exposes only Name()=%q", iv, iv.Name())
	t.Log("note: the projection wraps the same *Item, so it blocks writes THROUGH")
	t.Log("the view; it is not a snapshot.")
}

func TestSizes(t *testing.T) {
	t.Logf("shallow view (concrete ptr):        %d bytes", sizeOf(ShallowView[string, *Item]{}))
	t.Logf("per-contract view (interface):      %d bytes", sizeOf(ContractView[string, *Item]{}))
	t.Logf("projecting view, field-held func:   %d bytes", sizeOf(FieldProjView[string, *Item, ItemView]{}))
	t.Logf("projecting view, type-param witness:%d bytes", sizeOf(WitnessView[string, *Item, ItemView, itemConv]{}))
	t.Logf("the converter value itself:         %d bytes", sizeOf(itemConv{}))
}

// TestAliases records that aliases recover the ergonomics the witness costs.
func TestAliases(t *testing.T) {
	d := fixture()

	sv := d.WitnessView() // no type arguments at the call site
	_, ok := sv.Get("a")
	t.Logf("d.WitnessView() via generic alias: ok=%v, %d bytes", ok, sizeOf(sv))

	pv := ItemDictView{d} // five type arguments, spelled once in the alias
	iv, _ := pv.Get("a")
	t.Logf("ItemDictView{d} via instantiated alias: %T, %d bytes", iv, sizeOf(pv))

	// Aliases are identical types, not distinct ones.
	var long WitnessView[string, *Item, ItemView, itemConv] = pv
	var back ItemDictView = long
	_ = back
	t.Log("alias and long form are interchangeable in both directions")
}

func BenchmarkGet(b *testing.B) {
	d := fixture()
	sv := d.View()
	cv := SealDict[string, *Item](d)
	fv := ViewFunc(d, func(i *Item) ItemView { return ItemView{i} })
	wv := ItemDictView{d}
	wi := d.WitnessView()

	b.Run("direct", func(b *testing.B) {
		for b.Loop() {
			sinkItem, sinkB = d.Get("a")
		}
	})
	b.Run("shallow", func(b *testing.B) {
		for b.Loop() {
			sinkItem, sinkB = sv.Get("a")
		}
	})
	b.Run("per-contract", func(b *testing.B) {
		for b.Loop() {
			sinkItem, sinkB = cv.Get("a")
		}
	})
	b.Run("projecting-field", func(b *testing.B) {
		for b.Loop() {
			sinkIV, sinkB = fv.Get("a")
		}
	})
	b.Run("projecting-witness", func(b *testing.B) {
		for b.Loop() {
			sinkIV, sinkB = wv.Get("a")
		}
	})
	b.Run("witness-identity", func(b *testing.B) {
		for b.Loop() {
			sinkItem, sinkB = wi.Get("a")
		}
	})
}

// Boxing is the common case: passing a view where a contract is wanted.
func BenchmarkBox(b *testing.B) {
	d := fixture()
	sv := d.View()
	cv := SealDict[string, *Item](d)
	fv := ViewFunc(d, func(i *Item) ItemView { return ItemView{i} })
	wv := ItemDictView{d}

	b.Run("shallow", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkAny = sv
		}
	})
	b.Run("per-contract", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkAny = cv
		}
	})
	b.Run("projecting-field", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkAny = fv
		}
	})
	b.Run("projecting-witness", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkAny = wv
		}
	})
}

func BenchmarkNoOp(b *testing.B) {
	d := fixture()
	for b.Loop() {
		sinkI = d.Len()
	}
}
