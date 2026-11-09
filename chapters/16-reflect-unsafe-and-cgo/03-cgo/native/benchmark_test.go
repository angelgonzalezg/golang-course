package native

import (
	"strconv"
	"testing"
)

func sumBytesGo(data []byte) uint64 {
	var sum uint64
	for _, value := range data {
		sum += uint64(value)
	}
	return sum
}

func BenchmarkSumBytes(b *testing.B) {
	for _, size := range []int{16, 4096} {
		data := make([]byte, size)
		for index := range data {
			data[index] = byte(index)
		}
		want := sumBytesGo(data)
		b.Run(strconv.Itoa(size)+"B", func(b *testing.B) {
			for _, implementation := range []struct {
				name string
				fn   func([]byte) uint64
			}{
				{name: "Go", fn: sumBytesGo},
				{name: "Cgo", fn: SumBytes},
			} {
				b.Run(implementation.name, func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(len(data)))
					var result uint64
					for b.Loop() {
						result = implementation.fn(data)
					}
					if result != want {
						b.Fatalf("sum=%d, want %d", result, want)
					}
				})
			}
		})
	}
}
