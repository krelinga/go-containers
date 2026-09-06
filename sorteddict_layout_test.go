package containers

import (
	"testing"
	"unsafe"
)

// TestSortedEntryLayout locks the field order in place. Declaring the value
// field last would pad the struct whenever V is zero-sized, doubling the width
// of SortedDict[K, struct{}] with no compile error and no failing behaviour test.
//
// This is an internal test because the property is about an unexported type's
// layout; there is no way to observe it through the public API.
func TestSortedEntryLayout(t *testing.T) {
	cases := []struct {
		name       string
		got, wantK uintptr
	}{
		{"int key, empty value", unsafe.Sizeof(sortedEntry[int, struct{}]{}), unsafe.Sizeof(int(0))},
		{"string key, empty value", unsafe.Sizeof(sortedEntry[string, struct{}]{}), unsafe.Sizeof("")},
	}
	for _, tc := range cases {
		if tc.got != tc.wantK {
			t.Errorf("%s: sortedEntry is %d bytes, want %d (the bare key). "+
				"Is the value field declared last?", tc.name, tc.got, tc.wantK)
		}
	}
	// A non-empty value type must be unaffected by the ordering.
	if got, want := unsafe.Sizeof(sortedEntry[int, string]{}), unsafe.Sizeof(struct {
		s string
		i int
	}{}); got != want {
		t.Errorf("sortedEntry[int,string] is %d bytes, want %d", got, want)
	}
}
