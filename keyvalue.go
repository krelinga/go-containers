package containers

// KeyValue is one key/value pair, and is how bulk operations carry pairs.
//
// It exists because a bulk operation is variadic in its element type (ADR
// 0017), and a pair cannot be spread through a variadic parameter as two
// values. A dict's AllSlice yields these, and its SetAll consumes them:
//
//	other.SetAll(d.AllSlice()...)
//
// The single-element form stays two arguments — d.Set(k, v) — because that is
// what reads well; KeyValue is for the bulk path only.
type KeyValue[K, V any] struct {
	Key   K
	Value V
}
