package containers_test

import (
	"iter"

	containers "github.com/krelinga/go-containers"
)

// pairsOf materialises an iter.Seq2 into the pair slice the bulk methods take.
//
// Under ADR 0017 a bulk operation is variadic in its element type, so an
// iterator reaches one through a slice. In the library's own API that bridge is
// slices.Collect for the one-shape iterators; there is no stdlib equivalent for
// a Seq2, so tests use this.
func pairsOf[K, V any](seq iter.Seq2[K, V]) []containers.KeyValue[K, V] {
	var out []containers.KeyValue[K, V]
	for k, v := range seq {
		out = append(out, containers.KeyValue[K, V]{Key: k, Value: v})
	}
	return out
}
