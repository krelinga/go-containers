package copycost

// Typed sinks. Assigning results to an `any` sink would box the value and add
// an allocation plus ~12ns to every measurement, so each benchmark stores into
// a sink of its own concrete type instead.
var (
	sinkInt    []int
	sinkBig    []Big
	sinkPtrful []Ptrful
	sinkMap    map[int]int
	sinkScalar int
	sinkAny    any
)

// Small is pointer-free, 8 bytes.
type Small = int

// Big is pointer-free, 64 bytes.
type Big [8]int64

// Ptrful carries two pointer words per element, so the GC must scan it.
type Ptrful struct {
	Name string
	Tags *string
}

func makePtrful(n int) []Ptrful {
	s := "tag"
	src := make([]Ptrful, n)
	for i := range src {
		src[i] = Ptrful{Name: "somename", Tags: &s}
	}
	return src
}
