package iteration

import "testing"

func BenchmarkVariadicMutators(b *testing.B) {
	b.Run("vector/Append(e)", func(b *testing.B) {
		b.ReportAllocs()
		v := &vec[int]{es: make([]int, 0, 1<<20)}
		for b.Loop() {
			if len(v.es) == cap(v.es) {
				v.es = v.es[:0]
			}
			v.AppendOne(1)
		}
	})
	b.Run("vector/Append(e...)", func(b *testing.B) {
		b.ReportAllocs()
		v := &vec[int]{es: make([]int, 0, 1<<20)}
		for b.Loop() {
			if len(v.es) == cap(v.es) {
				v.es = v.es[:0]
			}
			v.AppendVar(1)
		}
	})

	s := &set[int]{m: make(map[int]struct{}, 1024)}
	b.Run("set/Add(v)", func(b *testing.B) {
		b.ReportAllocs()
		i := 0
		for b.Loop() {
			s.AddOne(i & 1023)
			i++
		}
	})
	b.Run("set/Add(v...)", func(b *testing.B) {
		b.ReportAllocs()
		i := 0
		for b.Loop() {
			s.AddVar(i & 1023)
			i++
		}
	})
	b.Run("set/Delete(k)", func(b *testing.B) {
		b.ReportAllocs()
		i := 0
		for b.Loop() {
			s.DeleteOne(i & 1023)
			i++
		}
	})
	b.Run("set/Delete(k...)", func(b *testing.B) {
		b.ReportAllocs()
		i := 0
		for b.Loop() {
			s.DeleteVar(i & 1023)
			i++
		}
	})
}

// The same comparison with inlining disabled, bounding the other end.
func BenchmarkVariadicNoInline(b *testing.B) {
	b.Run("vector/Append(e)", func(b *testing.B) {
		b.ReportAllocs()
		v := &vec[int]{es: make([]int, 0, 1<<20)}
		for b.Loop() {
			if len(v.es) == cap(v.es) {
				v.es = v.es[:0]
			}
			v.AppendOneNI(1)
		}
	})
	b.Run("vector/Append(e...)", func(b *testing.B) {
		b.ReportAllocs()
		v := &vec[int]{es: make([]int, 0, 1<<20)}
		for b.Loop() {
			if len(v.es) == cap(v.es) {
				v.es = v.es[:0]
			}
			v.AppendVarNI(1)
		}
	})
	s := &set[int]{m: make(map[int]struct{}, 1024)}
	b.Run("set/Add(v)", func(b *testing.B) {
		b.ReportAllocs()
		i := 0
		for b.Loop() {
			s.AddOneNI(i & 1023)
			i++
		}
	})
	b.Run("set/Add(v...)", func(b *testing.B) {
		b.ReportAllocs()
		i := 0
		for b.Loop() {
			s.AddVarNI(i & 1023)
			i++
		}
	})
}
