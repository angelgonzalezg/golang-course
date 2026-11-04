// Pointer Performance
// Topic: compares returning structs by value vs returning pointers to structs.
// Args/Input: examples use fixed-size arrays from 10 bytes to 100 MB.
// Usage: run this file to see how copying cost grows as values get larger.
// Key idea: returning a pointer copies an address, but returning a value copies the data.
package main

import (
	"fmt"
	"runtime"
	"time"
)

/* Helper section overview: payloads, sinks, return helpers, and timing helpers */
/* Section 1: Payload sizes */
const (
	payload10BytesSize  = 10
	payload100BytesSize = 100
	payload1KBSize      = 1_000
	payload10KBSize     = 10 * 1_000
	payload100KBSize    = 100 * 1_000
	payload1MBSize      = 1_000 * 1_000
	payload10MBSize     = 10 * 1_000 * 1_000
	payload100MBSize    = 100 * 1_000 * 1_000
)

/* Section 2: Fixed-size payload types */
// Each struct has an array so the value has a predictable size.
type Payload10Bytes struct {
	Data [payload10BytesSize]byte
}

type Payload100Bytes struct {
	Data [payload100BytesSize]byte
}

type Payload1KB struct {
	Data [payload1KBSize]byte
}

type Payload10KB struct {
	Data [payload10KBSize]byte
}

type Payload100KB struct {
	Data [payload100KBSize]byte
}

type Payload1MB struct {
	Data [payload1MBSize]byte
}

type Payload10MB struct {
	Data [payload10MBSize]byte
}

type Payload100MB struct {
	Data [payload100MBSize]byte
}

type PerformanceResult struct {
	Label           string
	Bytes           int
	Times           int
	ValueDuration   time.Duration
	PointerDuration time.Duration
}

/* Section 3: Shared payloads and result sinks */
// Shared values let us compare returning an existing value vs its address.
var shared10Bytes Payload10Bytes
var shared100Bytes Payload100Bytes
var shared1KB Payload1KB
var shared10KB Payload10KB
var shared100KB Payload100KB
var shared1MB Payload1MB
var shared10MB Payload10MB
var shared100MB Payload100MB

var value10Bytes Payload10Bytes
var value100Bytes Payload100Bytes
var value1KB Payload1KB
var value10KB Payload10KB
var value100KB Payload100KB
var value1MB Payload1MB
var value10MB Payload10MB
var value100MB Payload100MB

var pointer10Bytes *Payload10Bytes
var pointer100Bytes *Payload100Bytes
var pointer1KB *Payload1KB
var pointer10KB *Payload10KB
var pointer100KB *Payload100KB
var pointer1MB *Payload1MB
var pointer10MB *Payload10MB
var pointer100MB *Payload100MB

// The checksum helps keep measured work from being optimized away.
var performanceChecksum int

/* Section 4: Returning existing data */
func return10BytesValue() Payload10Bytes {
	return shared10Bytes
}

func return10BytesPointer() *Payload10Bytes {
	return &shared10Bytes
}

func return100BytesValue() Payload100Bytes {
	return shared100Bytes
}

func return100BytesPointer() *Payload100Bytes {
	return &shared100Bytes
}

func return1KBValue() Payload1KB {
	return shared1KB
}

func return1KBPointer() *Payload1KB {
	return &shared1KB
}

func return10KBValue() Payload10KB {
	return shared10KB
}

func return10KBPointer() *Payload10KB {
	return &shared10KB
}

func return100KBValue() Payload100KB {
	return shared100KB
}

func return100KBPointer() *Payload100KB {
	return &shared100KB
}

func return1MBValue() Payload1MB {
	return shared1MB
}

func return1MBPointer() *Payload1MB {
	return &shared1MB
}

func return10MBValue() Payload10MB {
	return shared10MB
}

func return10MBPointer() *Payload10MB {
	return &shared10MB
}

func return100MBValue() Payload100MB {
	return shared100MB
}

func return100MBPointer() *Payload100MB {
	return &shared100MB
}

/* Section 5: Returning new local data */
// Returning a pointer to local data is allowed in Go.
// The compiler moves the data to the heap when it must outlive the function.
func returnNewLocal100BytesValue() Payload100Bytes {
	var payload Payload100Bytes
	payload.Data[0] = 1
	payload.Data[len(payload.Data)-1] = 2
	return payload
}

func returnNewLocal100BytesPointer() *Payload100Bytes {
	var payload Payload100Bytes
	payload.Data[0] = 1
	payload.Data[len(payload.Data)-1] = 2
	return &payload
}

func returnNewLocal1MBValue() Payload1MB {
	var payload Payload1MB
	payload.Data[0] = 1
	payload.Data[len(payload.Data)-1] = 2
	return payload
}

func returnNewLocal1MBPointer() *Payload1MB {
	var payload Payload1MB
	payload.Data[0] = 1
	payload.Data[len(payload.Data)-1] = 2
	return &payload
}

func returnNewLocal100MBValue() Payload100MB {
	var payload Payload100MB
	payload.Data[0] = 1
	payload.Data[len(payload.Data)-1] = 2
	return payload
}

func returnNewLocal100MBPointer() *Payload100MB {
	var payload Payload100MB
	payload.Data[0] = 1
	payload.Data[len(payload.Data)-1] = 2
	return &payload
}

/* Section 6: Manual timing helpers */
func measureWork(times int, work func()) time.Duration {
	start := time.Now()

	for i := 0; i < times; i++ {
		work()
	}

	return time.Since(start)
}

/* Section 7: Existing-data measurements */
func measure10BytesValueReturn(times int) time.Duration {
	return measureWork(times, func() {
		value10Bytes = return10BytesValue()
		performanceChecksum += int(value10Bytes.Data[0])
		performanceChecksum += int(value10Bytes.Data[len(value10Bytes.Data)-1])
	})
}

func measure10BytesPointerReturn(times int) time.Duration {
	return measureWork(times, func() {
		pointer10Bytes = return10BytesPointer()
		performanceChecksum += int(pointer10Bytes.Data[0])
		performanceChecksum += int(pointer10Bytes.Data[len(pointer10Bytes.Data)-1])
	})
}

func measure100BytesValueReturn(times int) time.Duration {
	return measureWork(times, func() {
		value100Bytes = return100BytesValue()
		performanceChecksum += int(value100Bytes.Data[0])
		performanceChecksum += int(value100Bytes.Data[len(value100Bytes.Data)-1])
	})
}

func measure100BytesPointerReturn(times int) time.Duration {
	return measureWork(times, func() {
		pointer100Bytes = return100BytesPointer()
		performanceChecksum += int(pointer100Bytes.Data[0])
		performanceChecksum += int(pointer100Bytes.Data[len(pointer100Bytes.Data)-1])
	})
}

func measure1KBValueReturn(times int) time.Duration {
	return measureWork(times, func() {
		value1KB = return1KBValue()
		performanceChecksum += int(value1KB.Data[0])
		performanceChecksum += int(value1KB.Data[len(value1KB.Data)-1])
	})
}

func measure1KBPointerReturn(times int) time.Duration {
	return measureWork(times, func() {
		pointer1KB = return1KBPointer()
		performanceChecksum += int(pointer1KB.Data[0])
		performanceChecksum += int(pointer1KB.Data[len(pointer1KB.Data)-1])
	})
}

func measure10KBValueReturn(times int) time.Duration {
	return measureWork(times, func() {
		value10KB = return10KBValue()
		performanceChecksum += int(value10KB.Data[0])
		performanceChecksum += int(value10KB.Data[len(value10KB.Data)-1])
	})
}

func measure10KBPointerReturn(times int) time.Duration {
	return measureWork(times, func() {
		pointer10KB = return10KBPointer()
		performanceChecksum += int(pointer10KB.Data[0])
		performanceChecksum += int(pointer10KB.Data[len(pointer10KB.Data)-1])
	})
}

func measure100KBValueReturn(times int) time.Duration {
	return measureWork(times, func() {
		value100KB = return100KBValue()
		performanceChecksum += int(value100KB.Data[0])
		performanceChecksum += int(value100KB.Data[len(value100KB.Data)-1])
	})
}

func measure100KBPointerReturn(times int) time.Duration {
	return measureWork(times, func() {
		pointer100KB = return100KBPointer()
		performanceChecksum += int(pointer100KB.Data[0])
		performanceChecksum += int(pointer100KB.Data[len(pointer100KB.Data)-1])
	})
}

func measure1MBValueReturn(times int) time.Duration {
	return measureWork(times, func() {
		value1MB = return1MBValue()
		performanceChecksum += int(value1MB.Data[0])
		performanceChecksum += int(value1MB.Data[len(value1MB.Data)-1])
	})
}

func measure1MBPointerReturn(times int) time.Duration {
	return measureWork(times, func() {
		pointer1MB = return1MBPointer()
		performanceChecksum += int(pointer1MB.Data[0])
		performanceChecksum += int(pointer1MB.Data[len(pointer1MB.Data)-1])
	})
}

func measure10MBValueReturn(times int) time.Duration {
	return measureWork(times, func() {
		value10MB = return10MBValue()
		performanceChecksum += int(value10MB.Data[0])
		performanceChecksum += int(value10MB.Data[len(value10MB.Data)-1])
	})
}

func measure10MBPointerReturn(times int) time.Duration {
	return measureWork(times, func() {
		pointer10MB = return10MBPointer()
		performanceChecksum += int(pointer10MB.Data[0])
		performanceChecksum += int(pointer10MB.Data[len(pointer10MB.Data)-1])
	})
}

func measure100MBValueReturn(times int) time.Duration {
	return measureWork(times, func() {
		value100MB = return100MBValue()
		performanceChecksum += int(value100MB.Data[0])
		performanceChecksum += int(value100MB.Data[len(value100MB.Data)-1])
	})
}

func measure100MBPointerReturn(times int) time.Duration {
	return measureWork(times, func() {
		pointer100MB = return100MBPointer()
		performanceChecksum += int(pointer100MB.Data[0])
		performanceChecksum += int(pointer100MB.Data[len(pointer100MB.Data)-1])
	})
}

/* Section 8: New-local-data measurements */
func measureLocal100BytesValueReturn(times int) time.Duration {
	return measureWork(times, func() {
		value100Bytes = returnNewLocal100BytesValue()
		performanceChecksum += int(value100Bytes.Data[0])
		performanceChecksum += int(value100Bytes.Data[len(value100Bytes.Data)-1])
	})
}

func measureLocal100BytesPointerReturn(times int) time.Duration {
	return measureWork(times, func() {
		pointer100Bytes = returnNewLocal100BytesPointer()
		performanceChecksum += int(pointer100Bytes.Data[0])
		performanceChecksum += int(pointer100Bytes.Data[len(pointer100Bytes.Data)-1])
	})
}

func measureLocal1MBValueReturn(times int) time.Duration {
	return measureWork(times, func() {
		value1MB = returnNewLocal1MBValue()
		performanceChecksum += int(value1MB.Data[0])
		performanceChecksum += int(value1MB.Data[len(value1MB.Data)-1])
	})
}

func measureLocal1MBPointerReturn(times int) time.Duration {
	return measureWork(times, func() {
		pointer1MB = returnNewLocal1MBPointer()
		performanceChecksum += int(pointer1MB.Data[0])
		performanceChecksum += int(pointer1MB.Data[len(pointer1MB.Data)-1])
	})
}

func measureLocal100MBValueReturn(times int) time.Duration {
	return measureWork(times, func() {
		value100MB = returnNewLocal100MBValue()
		performanceChecksum += int(value100MB.Data[0])
		performanceChecksum += int(value100MB.Data[len(value100MB.Data)-1])
	})
}

func measureLocal100MBPointerReturn(times int) time.Duration {
	return measureWork(times, func() {
		pointer100MB = returnNewLocal100MBPointer()
		performanceChecksum += int(pointer100MB.Data[0])
		performanceChecksum += int(pointer100MB.Data[len(pointer100MB.Data)-1])
	})
}

/* Section 9: Prepare sample data */
func preparePerformancePayloads() {
	shared10Bytes.Data[0] = 1
	shared10Bytes.Data[len(shared10Bytes.Data)-1] = 2
	shared100Bytes.Data[0] = 1
	shared100Bytes.Data[len(shared100Bytes.Data)-1] = 2
	shared1KB.Data[0] = 1
	shared1KB.Data[len(shared1KB.Data)-1] = 2
	shared10KB.Data[0] = 1
	shared10KB.Data[len(shared10KB.Data)-1] = 2
	shared100KB.Data[0] = 1
	shared100KB.Data[len(shared100KB.Data)-1] = 2
	shared1MB.Data[0] = 1
	shared1MB.Data[len(shared1MB.Data)-1] = 2
	shared10MB.Data[0] = 1
	shared10MB.Data[len(shared10MB.Data)-1] = 2
	shared100MB.Data[0] = 1
	shared100MB.Data[len(shared100MB.Data)-1] = 2
}

/* Section 10: Run and print manual results */
func runPerformanceCase(label string, bytes, times int, valueFn, pointerFn func(int) time.Duration) PerformanceResult {
	return PerformanceResult{
		Label:           label,
		Bytes:           bytes,
		Times:           times,
		ValueDuration:   valueFn(times),
		PointerDuration: pointerFn(times),
	}
}

func averageDuration(duration time.Duration, times int) time.Duration {
	if times <= 0 {
		return 0
	}

	return duration / time.Duration(times)
}

func printPerformanceResults(results []PerformanceResult) {
	fmt.Printf("%-10s %10s %10s %14s %14s\n", "Payload", "Bytes", "Runs", "Value avg", "Pointer avg")

	for _, result := range results {
		fmt.Printf(
			"%-10s %10d %10d %14s %14s\n",
			result.Label,
			result.Bytes,
			result.Times,
			averageDuration(result.ValueDuration, result.Times),
			averageDuration(result.PointerDuration, result.Times),
		)
	}
}

func main() {
	/* Performance example: value vs pointer */
	// This is a quick demo, not a real benchmark.
	//
	// For better measurements, use the benchmarks in main_test.go:
	// go test -bench=. ./chapters/06-pointers/02-pointer-performance
	// Runs every benchmark in this folder.
	// go test -bench=. -benchmem ./chapters/06-pointers/02-pointer-performance
	// Also shows memory allocations.
	// go test -bench=. -benchmem -count=5 ./chapters/06-pointers/02-pointer-performance
	// Repeats the benchmarks five times to spot noisy results.
	//
	// In this example, 1 KB = 1,000 bytes and 1 MB = 1,000,000 bytes.
	preparePerformancePayloads()

	/* Section 1: returning existing data */
	// The value functions copy existing data.
	// The pointer functions return an address to existing data.
	existingDataResults := []PerformanceResult{
		runPerformanceCase("10 B", payload10BytesSize, 2_000_000, measure10BytesValueReturn, measure10BytesPointerReturn),
		runPerformanceCase("100 B", payload100BytesSize, 1_000_000, measure100BytesValueReturn, measure100BytesPointerReturn),
		runPerformanceCase("1 KB", payload1KBSize, 200_000, measure1KBValueReturn, measure1KBPointerReturn),
		runPerformanceCase("10 KB", payload10KBSize, 50_000, measure10KBValueReturn, measure10KBPointerReturn),
		runPerformanceCase("100 KB", payload100KBSize, 5_000, measure100KBValueReturn, measure100KBPointerReturn),
		runPerformanceCase("1 MB", payload1MBSize, 200, measure1MBValueReturn, measure1MBPointerReturn),
		runPerformanceCase("10 MB", payload10MBSize, 20, measure10MBValueReturn, measure10MBPointerReturn),
		runPerformanceCase("100 MB", payload100MBSize, 2, measure100MBValueReturn, measure100MBPointerReturn),
	}

	fmt.Println("Go version:", runtime.Version())
	fmt.Println("\nSection 1: existing data")
	printPerformanceResults(existingDataResults)

	/* Section 2: returning new local data */
	// These functions create a new value inside the function before returning it.
	// Returning &localData can make that local data escape to the heap.
	localDataResults := []PerformanceResult{
		runPerformanceCase("100 B", payload100BytesSize, 1_000_000, measureLocal100BytesValueReturn, measureLocal100BytesPointerReturn),
		runPerformanceCase("1 MB", payload1MBSize, 100, measureLocal1MBValueReturn, measureLocal1MBPointerReturn),
		runPerformanceCase("100 MB", payload100MBSize, 1, measureLocal100MBValueReturn, measureLocal100MBPointerReturn),
	}

	fmt.Println("\nSection 2: new local data")
	printPerformanceResults(localDataResults)
	fmt.Println("Performance checksum:", performanceChecksum)
}
