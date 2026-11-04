package main

import "testing"

// Section 1 benchmarks compare two ways of returning existing data:
// 1. Return a value, which copies the payload.
// 2. Return a pointer, which copies only the memory address.
//
// Section 2 benchmarks compare two ways of returning new local data:
// 1. Return a new local value.
// 2. Return a pointer to a new local value.
// Returning a pointer to local data can make that data escape to the heap.
//
// testing.B is the type Go uses for benchmark functions.
// The benchmark runner creates it and passes it to each Benchmark... function.
// b.N is the number of loop runs chosen by Go for that specific benchmark.
//
// Run all benchmarks (use default/all CPUs):
// Better when code uses goroutines (run concurrently), real application behavior on the machine
// go test -bench=. -benchmem ./chapters/06-pointers/02-pointer-performance
//
// Run setting the number of logical CPUs manually
// Better for basic performance, simpler comparisons, can be single-threaded, less scheduling noise)
// -count=n rerun the test to help if numbers are stable
// GOMAXPROCS=1 go test -bench=. -benchmem -count=5 ./chapters/06-pointers/02-pointer-performance
//
// How to read the output:
// BenchmarkReturn10BytesValue-12  1000000000  0.72 ns/op  0 B/op  0 allocs/op
// - 1000000000 is the final b.N chosen by Go.
// - ns/op is the average time for one operation.
// - B/op shows bytes allocated per operation.
// - allocs/op shows allocations per operation.

/* Section 1: existing data */

/* 10 bytes */
func BenchmarkReturn10BytesValue(b *testing.B) {
	// Prepare sample data before the timer starts.
	preparePerformancePayloads()

	// Show allocations in the benchmark output.
	b.ReportAllocs()

	// Ignore setup time and measure only the loop below.
	b.ResetTimer()

	// Go chooses b.N automatically.
	// Fast code gets more runs; slow code gets fewer runs.
	for i := 0; i < b.N; i++ {
		// Store the result globally so the compiler keeps the work.
		value10Bytes = return10BytesValue()
	}
}

func BenchmarkReturn10BytesPointer(b *testing.B) {
	// Same benchmark shape, but this returns a pointer instead of a full value.
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pointer10Bytes = return10BytesPointer()
	}
}

/* 100 bytes */
func BenchmarkReturn100BytesValue(b *testing.B) {
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		value100Bytes = return100BytesValue()
	}
}

func BenchmarkReturn100BytesPointer(b *testing.B) {
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pointer100Bytes = return100BytesPointer()
	}
}

/* 1 KB */
func BenchmarkReturn1KBValue(b *testing.B) {
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		value1KB = return1KBValue()
	}
}

func BenchmarkReturn1KBPointer(b *testing.B) {
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pointer1KB = return1KBPointer()
	}
}

/* 10 KB */
func BenchmarkReturn10KBValue(b *testing.B) {
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		value10KB = return10KBValue()
	}
}

func BenchmarkReturn10KBPointer(b *testing.B) {
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pointer10KB = return10KBPointer()
	}
}

/* 100 KB */
func BenchmarkReturn100KBValue(b *testing.B) {
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		value100KB = return100KBValue()
	}
}

func BenchmarkReturn100KBPointer(b *testing.B) {
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pointer100KB = return100KBPointer()
	}
}

/* 1 MB */
func BenchmarkReturn1MBValue(b *testing.B) {
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		value1MB = return1MBValue()
	}
}

func BenchmarkReturn1MBPointer(b *testing.B) {
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pointer1MB = return1MBPointer()
	}
}

/* 10 MB */
func BenchmarkReturn10MBValue(b *testing.B) {
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		value10MB = return10MBValue()
	}
}

func BenchmarkReturn10MBPointer(b *testing.B) {
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pointer10MB = return10MBPointer()
	}
}

/* 100 MB */
func BenchmarkReturn100MBValue(b *testing.B) {
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		value100MB = return100MBValue()
	}
}

func BenchmarkReturn100MBPointer(b *testing.B) {
	preparePerformancePayloads()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pointer100MB = return100MBPointer()
	}
}

/* Section 2: new local data */

/* 100 bytes */
func BenchmarkReturnNewLocal100BytesValue(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		value100Bytes = returnNewLocal100BytesValue()
	}
}

func BenchmarkReturnNewLocal100BytesPointer(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pointer100Bytes = returnNewLocal100BytesPointer()
	}
}

/* 1 MB */
func BenchmarkReturnNewLocal1MBValue(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		value1MB = returnNewLocal1MBValue()
	}
}

func BenchmarkReturnNewLocal1MBPointer(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pointer1MB = returnNewLocal1MBPointer()
	}
}

/* 100 MB */
func BenchmarkReturnNewLocal100MBValue(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		value100MB = returnNewLocal100MBValue()
	}
}

func BenchmarkReturnNewLocal100MBPointer(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pointer100MB = returnNewLocal100MBPointer()
	}
}
