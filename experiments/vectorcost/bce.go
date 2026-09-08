package vectorcost

// Two sums the compiler can be asked about directly.

func sumSliceIndexed(s []int) int {
	t := 0
	for i := 0; i < len(s); i++ {
		t += s[i]
	}
	return t
}

func sumVectorIndexed(v *Vector[int]) int {
	t := 0
	for i := 0; i < v.Len(); i++ {
		t += v.At(i)
	}
	return t
}
