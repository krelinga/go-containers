package containers

import (
	"testing"
	"unsafe"
)

// TestContainerLayout locks ADR 0002 decision 5's rule in place for every
// container that carries noCopy: the field must be declared FIRST. A zero-sized
// field in trailing position forces the struct to be padded, with no compile
// error and no failing behaviour test.
//
// sorteddict_layout_test.go asserts the same rule for sortedEntry, whose value
// field has the mirror-image constraint.
//
// This is an internal test because the property is about layout; there is no way
// to observe it through the public API.
func TestContainerLayout(t *testing.T) {
	var (
		mapWidth   = unsafe.Sizeof(map[int]struct{}(nil))
		sliceWidth = unsafe.Sizeof([]int(nil))
	)
	cases := []struct {
		name      string
		got, want uintptr
	}{
		{"HashSet", unsafe.Sizeof(HashSet[int]{}), mapWidth},
		{"HashDict", unsafe.Sizeof(HashDict[string, int]{}), mapWidth},
		{"SortedSet", unsafe.Sizeof(SortedSet[int]{}), sliceWidth},
		{"SortedDict", unsafe.Sizeof(SortedDict[string, int]{}), sliceWidth},
		{"Vector", unsafe.Sizeof(Vector[int]{}), sliceWidth},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s is %d bytes, want %d (the bare backing field). "+
				"Is noCopy declared last?", tc.name, tc.got, tc.want)
		}
	}
}
