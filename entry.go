package containers

// Entry is one slot/value pair, and is how bulk operations carry pairs.
//
// It exists because a bulk operation is variadic in its element type (ADR
// 0017), and a pair cannot be spread through a variadic parameter as two
// values.
//
// The first field is a Slot rather than a Key because one type serves both
// sides of the library: a dict's slot is its key, and a sequence's slot is its
// position (ADR 0018). The second is always a value.
//
//	other.SetAll(m.AllSlice()...)
//	v := NewVector(m.AllSlice()...)   // keyed by position
//
// The single-element form stays two arguments -- m.Set(k, v) -- because that is
// what reads well; Entry is for the bulk path only.
type Entry[S, V any] struct {
	Slot  S
	Value V
}
