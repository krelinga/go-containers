package sliceadapter

func sumRawIndexed(s []int) int {
	t := 0
	for i := 0; i < len(s); i++ {
		t += s[i]
	}
	return t
}

func sumAdapterIndexed(s Slice[int]) int {
	t := 0
	for i := 0; i < len(s); i++ {
		t += s[i]
	}
	return t
}

func sumAdapterViaAt(s Slice[int]) int {
	t := 0
	for i := 0; i < s.Len(); i++ {
		t += s.At(i)
	}
	return t
}

func sumVectorViaAt(v *Vector[int]) int {
	t := 0
	for i := 0; i < v.Len(); i++ {
		t += v.At(i)
	}
	return t
}
