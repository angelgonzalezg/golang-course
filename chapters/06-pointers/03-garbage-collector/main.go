// Garbage Collector
// Topic: Go's garbage collector finds heap memory that is no longer reachable.
// Args/Input: examples use slices, maps, buffers, object references, sync.Pool, and runtime tuning.
// Usage: reduce unnecessary allocations, release references, and understand basic GC tuning knobs.
// Key idea: less short-lived heap memory usually means less garbage-collector work.
package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
)

/* Helper section 1: Sizes and example data types */
const (
	kib      = 1024
	mib      = 1024 * kib
	maxInt64 = 1<<63 - 1
)

type Receipt struct {
	ID         int
	TotalCents int
	Paid       bool
}

type largeObject struct {
	data [32 * kib]byte
}

/* Helper section 2: Global sinks and reusable pool */
var sinkInt int
var sinkIDs []int
var sinkMap map[int]int
var sinkBuffers [][]byte
var sinkReceipts []Receipt
var sinkReceiptPointers []*Receipt

var byteBufferPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, 256)
	},
}

/* Helper section 3: Memory reporting helpers */
func formatBytes(bytes uint64) string {
	switch {
	case bytes >= mib:
		return fmt.Sprintf("%.2f MiB", float64(bytes)/mib)
	case bytes >= kib:
		return fmt.Sprintf("%.2f KiB", float64(bytes)/kib)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func formatMemoryLimit(limit int64) string {
	if limit == maxInt64 {
		return "unlimited"
	}

	if limit < 0 {
		return "unknown"
	}

	return formatBytes(uint64(limit))
}

func readMemoryStats() runtime.MemStats {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats
}

func printMemory(label string) {
	stats := readMemoryStats()
	fmt.Printf(
		"%s: HeapAlloc=%s, HeapObjects=%d, NumGC=%d, NextGC=%s\n",
		label,
		formatBytes(stats.HeapAlloc),
		stats.HeapObjects,
		stats.NumGC,
		formatBytes(stats.NextGC),
	)
}

func measureAllocations(label string, work func()) {
	// runtime.GC is used here only to make this lesson easier to observe.
	// In regular application code, let the runtime decide when GC should run.
	runtime.GC()
	before := readMemoryStats()

	work()

	after := readMemoryStats()
	fmt.Printf(
		"%s: TotalAlloc +%s, Mallocs +%d\n",
		label,
		formatBytes(after.TotalAlloc-before.TotalAlloc),
		after.Mallocs-before.Mallocs,
	)
}

/* Helper section 4: Allocation examples */
func makeTemporaryGarbage() {
	garbage := make([][]byte, 3_000)

	for i := range garbage {
		garbage[i] = make([]byte, kib)
		garbage[i][0] = byte(i)
	}

	sinkInt += len(garbage)
}

func updateReceiptValue(receipt Receipt) Receipt {
	receipt.Paid = true
	return receipt
}

func buildReceiptValues(count int) []Receipt {
	receipts := make([]Receipt, 0, count)

	for i := 0; i < count; i++ {
		receipts = append(receipts, Receipt{
			ID:         i,
			TotalCents: 1000 + i,
			Paid:       i%2 == 0,
		})
	}

	return receipts
}

func buildReceiptPointers(count int) []*Receipt {
	receipts := make([]*Receipt, 0, count)

	for i := 0; i < count; i++ {
		receipt := &Receipt{
			ID:         i,
			TotalCents: 1000 + i,
			Paid:       i%2 == 0,
		}
		receipts = append(receipts, receipt)
	}

	return receipts
}

/* Helper section 5: Buffer reuse examples */
func buildMessagesWithNewBuffers(count int) [][]byte {
	buffers := make([][]byte, 0, count)

	for i := 0; i < count; i++ {
		buffer := make([]byte, 0, 64)
		buffer = append(buffer, "request-id:"...)
		buffer = append(buffer, byte('0'+i%10))
		buffers = append(buffers, buffer)
	}

	return buffers
}

func buildMessagesWithReusedBuffer(count int) int {
	buffer := make([]byte, 0, 64)
	totalBytes := 0

	for i := 0; i < count; i++ {
		buffer = buffer[:0]
		buffer = append(buffer, "request-id:"...)
		buffer = append(buffer, byte('0'+i%10))
		totalBytes += len(buffer)
	}

	return totalBytes
}

/* Helper section 6: Collection preallocation examples */
func buildIDsWithoutPrealloc(count int) []int {
	ids := []int{}

	for i := 0; i < count; i++ {
		ids = append(ids, i)
	}

	return ids
}

func buildIDsWithPrealloc(count int) []int {
	ids := make([]int, 0, count)

	for i := 0; i < count; i++ {
		ids = append(ids, i)
	}

	return ids
}

func buildLookupWithoutPrealloc(count int) map[int]int {
	lookup := map[int]int{}

	for i := 0; i < count; i++ {
		lookup[i] = i * 10
	}

	return lookup
}

func buildLookupWithPrealloc(count int) map[int]int {
	lookup := make(map[int]int, count)

	for i := 0; i < count; i++ {
		lookup[i] = i * 10
	}

	return lookup
}

/* Helper section 7: Reference and pooling helpers */
func makeObjectCache(count int) []*largeObject {
	cache := make([]*largeObject, count)

	for i := range cache {
		object := &largeObject{}
		object.data[0] = byte(i)
		cache[i] = object
	}

	return cache
}

func clearObjectCache(cache []*largeObject) {
	for i := range cache {
		cache[i] = nil
	}
}

func writeWithPool(message string) int {
	buffer := byteBufferPool.Get().([]byte)
	buffer = buffer[:0]
	buffer = append(buffer, message...)

	bytesWritten := len(buffer)
	byteBufferPool.Put(buffer)
	return bytesWritten
}

func main() {
	/* Section 1: What the garbage collector cleans */
	fmt.Println("== 1. What the garbage collector cleans ==")

	printMemory("startup")
	makeTemporaryGarbage()
	printMemory("after temporary heap allocations")

	// For the lesson, force one collection so the change is visible.
	runtime.GC()
	printMemory("after forced GC")

	/* Section 2: Prefer simple values when possible */
	fmt.Println("\n== 2. Prefer simple values when possible ==")

	receipt := Receipt{ID: 1, TotalCents: 2599}
	paidReceipt := updateReceiptValue(receipt)

	// Small values can often stay on the stack.
	// Stack memory is not heap garbage, so it gives the GC less work.
	fmt.Printf("original receipt: %+v\n", receipt)
	fmt.Printf("updated copy:     %+v\n", paidReceipt)

	/* Section 3: Prefer slices of structs when possible */
	fmt.Println("\n== 3. Prefer slices of structs when possible ==")

	// []Receipt stores the receipts together in one slice backing array.
	// []*Receipt stores pointers, and each &Receipt usually becomes a separate heap object.
	measureAllocations("slice of struct values", func() {
		sinkReceipts = buildReceiptValues(20_000)
	})
	sinkReceipts = nil

	measureAllocations("slice of pointers to structs", func() {
		sinkReceiptPointers = buildReceiptPointers(20_000)
	})
	sinkReceiptPointers = nil

	// Use pointer slices when you need nil entries, shared mutable objects,
	// or to avoid copying very large structs. Use them deliberately.
	fmt.Println("Rule: prefer []Struct first; use []*Struct when the pointer behavior matters.")

	/* Section 4: Reuse temporary buffers */
	fmt.Println("\n== 4. Reuse temporary buffers ==")

	// Creating many temporary buffers can create many short-lived heap objects.
	measureAllocations("new buffer per message", func() {
		sinkBuffers = buildMessagesWithNewBuffers(5_000)
	})
	sinkBuffers = nil

	// Reusing one buffer is useful when each result is temporary.
	measureAllocations("reuse one buffer", func() {
		sinkInt += buildMessagesWithReusedBuffer(5_000)
	})

	/* Section 5: Preallocate slices and maps */
	fmt.Println("\n== 5. Preallocate slices and maps ==")

	// If you know the expected size, capacity avoids repeated growth work.
	measureAllocations("slice without prealloc", func() {
		sinkIDs = buildIDsWithoutPrealloc(20_000)
	})

	measureAllocations("slice with prealloc", func() {
		sinkIDs = buildIDsWithPrealloc(20_000)
	})
	sinkIDs = nil

	measureAllocations("map without prealloc", func() {
		sinkMap = buildLookupWithoutPrealloc(20_000)
	})

	measureAllocations("map with prealloc", func() {
		sinkMap = buildLookupWithPrealloc(20_000)
	})
	sinkMap = nil

	/* Section 6: Clear references in long-lived containers */
	fmt.Println("\n== 6. Clear references in long-lived containers ==")

	cache := makeObjectCache(300)
	runtime.GC()
	printMemory("cache holding object references")

	// A long-lived slice keeps the objects alive while it still stores pointers to them.
	clearObjectCache(cache)
	runtime.GC()
	printMemory("after clearing object references")
	runtime.KeepAlive(cache)

	/* Section 7: Use sync.Pool for hot temporary objects */
	fmt.Println("\n== 7. Use sync.Pool for hot temporary objects ==")

	// sync.Pool is for temporary reusable objects.
	// It is not a permanent cache; the GC can drop pooled values.
	for i := 0; i < 3; i++ {
		bytesWritten := writeWithPool("pooled temporary buffer")
		fmt.Println("bytes written with pooled buffer:", bytesWritten)
	}

	/* Section 8: Tune GC frequency with GOGC */
	fmt.Println("\n== 8. Tune GC frequency with GOGC ==")

	// GOGC controls how much new heap data is allowed before the next GC.
	// Lower values usually mean lower memory usage and more GC CPU work.
	// Higher values usually mean higher memory usage and less frequent GC work.
	previousGCPercent := debug.SetGCPercent(200)
	fmt.Println("temporarily set GC target percent to 200")
	fmt.Println("previous GC target percent:", previousGCPercent)
	debug.SetGCPercent(previousGCPercent)

	/* Section 9: Tune the soft memory limit */
	fmt.Println("\n== 9. Tune the soft memory limit ==")

	// GOMEMLIMIT or debug.SetMemoryLimit asks Go to stay near a memory limit.
	// The runtime may run GC more often to respect the limit.
	previousLimit := debug.SetMemoryLimit(128 * mib)
	fmt.Println("temporarily set memory limit to 128 MiB")
	fmt.Println("previous memory limit:", formatMemoryLimit(previousLimit))
	debug.SetMemoryLimit(previousLimit)

	/* Section 10: Observe GC behavior */
	fmt.Println("\n== 10. Observe GC behavior ==")

	// These are useful commands to try from the terminal:
	fmt.Println("GODEBUG=gctrace=1 go run ./chapters/06-pointers/03-garbage-collector")
	fmt.Println("GOGC=50 go run ./chapters/06-pointers/03-garbage-collector")
	fmt.Println("GOMEMLIMIT=128MiB go run ./chapters/06-pointers/03-garbage-collector")

	/* Section 11: Final GC rule */
	fmt.Println("\n== 11. Final GC rule ==")

	// Good GC habits usually mean fewer unnecessary heap allocations,
	// shorter-lived references, and tuning only after measuring.
	fmt.Println("Reduce allocations first. Tune GC after measuring real memory behavior.")
}
