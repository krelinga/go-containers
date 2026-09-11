package refcontainers

import "testing"

// The assertions in hierarchy.go are only worth something if the wrong shapes
// FAIL. These record what must not compile, since Go cannot express a negative
// assertion inline.
//
//	var _ hKeys[int]              = hVector[int]{}          // missing method Has
//	var _ hValues[string]         = hHashSet[string]{}      // missing method ValueSlice
//	var _ hMutableKeys[string]    = hHashSetView[string]{}  // missing method Add
//
// All three were run and all three fail with those messages.

func TestShapeVocabularyIsDisjoint(t *testing.T) {
	// A sequence is not a Keys: positions are not keys (ADR 0016, 0017).
	var _ hPositionValues[int, string] = hVector[string]{}
	var _ hValues[string] = hVector[string]{}

	// A set is key-only: it has no value side.
	var _ hKeys[string] = hHashSet[string]{}

	// A dict is both, which is what lets one function read values out of a
	// dict OR a sequence.
	var _ hKeys[string] = hMap[string, int]{}
	var _ hValues[int] = hMap[string, int]{}
	var _ hKeyValues[string, int] = hMap[string, int]{}

	t.Log("keys, values and positions are separate vocabularies; dicts have two of them")
}

// The payoff of naming interfaces for shape rather than for container kind.
func TestOneFunctionReadsValuesFromEverything(t *testing.T) {
	got := []int{
		hSum(hMap[string, int]{}),
		hSum(hSortedDict[string, int]{}),
		hSum(hVector[int]{}),
		hSum(hVectorView[int]{}),
		hSum(hSliceView[int]{}),
	}
	if len(got) != 5 {
		t.Fatal("unreachable")
	}
	t.Log("one Values[int] parameter accepts a dict, a sorted dict, a vector, and two views")
}
