// Garbage Collector
// Topic: Go's garbage collector finds heap memory that is no longer reachable.
// Args/Input: examples use slices, maps, buffers, object references, sync.Pool, and runtime tuning.
// Usage: reduce unnecessary allocations, release references, and understand basic GC tuning knobs.
// Key idea: less short-lived heap memory usually means less garbage-collector work.
package main

import (
	"fmt"
	"math"
	"runtime"
	"runtime/metrics"
	"strconv"
	"sync"
)

/* Helper section 1: Sizes and example data types */
const (
	kib                     = 1024
	mib                     = 1024 * kib
	maxPooledBufferCapacity = 64 * kib
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
var sinkIDs []int
var sinkMap map[int]int
var sinkBuffers [][]byte
var sinkReceipts []Receipt
var sinkReceiptPointers []*Receipt
var sinkChecksum uint64

type messageBuffer struct {
	data []byte
}

var byteBufferPool = sync.Pool{
	New: func() any {
		// A pointer fits in the interface without boxing a slice header each Put.
		return &messageBuffer{data: make([]byte, 0, 256)}
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
	if limit == math.MaxInt64 {
		return "disabled (MaxInt64)"
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
	// Process-wide snapshots include runtime activity; they are observations,
	// not exact allocation contracts. Repeated benchmarks provide better comparisons.
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
func makeTemporaryBuffers() [][]byte {
	garbage := make([][]byte, 3_000)

	for i := range garbage {
		garbage[i] = make([]byte, kib)
		garbage[i][0] = byte(i)
	}

	return garbage
}

// Return a small scalar-only copy when the original receipt should stay unchanged.
func updateReceiptValue(receipt Receipt) Receipt {
	receipt.Paid = true
	return receipt
}

// buildReceiptValues stores records directly in one backing array.
// Receipt has no pointer fields, so the GC has no element pointers to scan.
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

// buildReceiptPointers stores pointers to separately created receipts.
// The GC follows the pointer array to its live targets; this layout can add
// allocations and reference scanning compared with the pointer-free value array.
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
func appendRequestID(buffer []byte, id int) []byte {
	buffer = append(buffer, "request-id:"...)
	return strconv.AppendInt(buffer, int64(id), 10)
}

// checksumBytes reads every byte synchronously and never retains the slice.
// This simple checksum makes consumed data observable; it is not cryptography.
func checksumBytes(buffer []byte) uint64 {
	var checksum uint64
	for _, value := range buffer {
		checksum = checksum*31 + uint64(value)
	}
	return checksum
}

// Retaining all messages requires independent byte storage for each result.
func buildMessagesWithNewBuffers(count int) [][]byte {
	buffers := make([][]byte, 0, count)

	for i := 0; i < count; i++ {
		buffer := make([]byte, 0, 64)
		buffer = appendRequestID(buffer, i)
		buffers = append(buffers, buffer)
	}

	return buffers
}

// Both processing variants build and checksum the same messages without keeping
// them. A new source-level buffer does not guarantee a heap allocation.
func processMessagesWithNewBuffers(count int) uint64 {
	var checksum uint64
	for i := 0; i < count; i++ {
		buffer := make([]byte, 0, 64)
		buffer = appendRequestID(buffer, i)
		checksum += checksumBytes(buffer)
	}
	return checksum
}

func processMessagesWithReusedBuffer(count int) uint64 {
	buffer := make([]byte, 0, 64)
	var checksum uint64

	for i := 0; i < count; i++ {
		buffer = buffer[:0]
		buffer = appendRequestID(buffer, i)
		// Consume before reuse; saving these views would make messages alias.
		checksum += checksumBytes(buffer)
	}

	return checksum
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
	clear(cache) // Clears visible slots, preserving len, cap, and the backing array.
}

// prepareForPool defines a per-buffer retention policy; the pool's total size
// is not bounded by this cap. It does not erase bytes in the backing array.
func prepareForPool(buffer *messageBuffer) bool {
	if cap(buffer.data) > maxPooledBufferCapacity {
		buffer.data = nil
		return false
	}
	buffer.data = buffer.data[:0]
	return true
}

func checksumWithPool(message string) uint64 {
	buffer := byteBufferPool.Get().(*messageBuffer)
	buffer.data = append(buffer.data[:0], message...)
	checksum := checksumBytes(buffer.data)
	// Reuse can reduce repeated allocation work, while pooled capacity retains memory.
	if prepareForPool(buffer) {
		byteBufferPool.Put(buffer)
	}
	// Do not access buffer or return aliases into its data after Put.
	return checksum
}

func checksumWithLocalBuffer(message string) uint64 {
	buffer := make([]byte, 0, 256)
	buffer = append(buffer, message...)
	return checksumBytes(buffer)
}

func main() {
	/* Section 1: What the garbage collector cleans */
	fmt.Println("== 1. What the garbage collector cleans ==")

	printMemory("startup")
	sinkBuffers = makeTemporaryBuffers()
	printMemory("while buffers are reachable")
	sinkBuffers = nil

	// For the lesson, force one collection so the change is visible.
	runtime.GC()
	printMemory("after releasing buffers and forcing GC")
	fmt.Println("Heap reclamation does not imply an immediate drop in process RSS.")

	/* Section 2: Prefer simple values when possible */
	fmt.Println("\n== 2. Prefer simple values when possible ==")

	receipt := Receipt{ID: 1, TotalCents: 2599}
	paidReceipt := updateReceiptValue(receipt)

	// Small values can often stay on the stack.
	// The compiler decides placement. Pointers stored on stacks can still be GC roots.
	fmt.Printf("original receipt: %+v\n", receipt)
	fmt.Printf("updated copy:     %+v\n", paidReceipt)

	/* Section 3: Choose between slices of values and slices of pointers */
	fmt.Println("\n== 3. Slices of values and slices of pointers ==")

	// []Receipt (values): start here for small records stored and processed together
	// when per-object identity and nil entries are unnecessary. Elements remain mutable.
	measureAllocations("slice of struct values", func() {
		sinkReceipts = buildReceiptValues(20_000)
	})
	sinkReceipts = nil

	// []*Receipt (pointers): use for objects shared across collections, identity
	// preserved as a slice grows, or nil entries. For large structs, benchmark copy
	// costs against allocations and GC work before choosing pointers for performance.
	measureAllocations("slice of pointers to structs", func() {
		sinkReceiptPointers = buildReceiptPointers(20_000)
	})
	sinkReceiptPointers = nil

	fmt.Println("Rule for []T (values): use for small records processed together when per-object identity and nil entries are unnecessary.")
	fmt.Println("Rule for []*T (pointers): use for shared object identity or nil entries; for large structs, measure copying against allocation and GC costs.")

	/* Section 4: Reuse temporary buffers */
	fmt.Println("\n== 4. Reuse temporary buffers ==")

	measureAllocations("new temporary buffer per message", func() {
		sinkChecksum = processMessagesWithNewBuffers(5_000)
	})
	newBufferChecksum := sinkChecksum

	// Reusing one buffer is useful when each result is temporary.
	measureAllocations("reuse one buffer", func() {
		sinkChecksum = processMessagesWithReusedBuffer(5_000)
	})
	fmt.Println("same consumed messages:", newBufferChecksum == sinkChecksum)

	retained := buildMessagesWithNewBuffers(3)
	fmt.Printf("independently retained messages: %q\n", retained)

	/* Section 5: Preallocate slices and maps */
	fmt.Println("\n== 5. Preallocate slices and maps ==")

	// Slice capacity avoids growth while it is sufficient; map size is only a hint.
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

	cache := makeObjectCache(128)
	runtime.GC()
	printMemory("cache holding object references")
	// Keep the original object graph alive through the observation above, even
	// if the compiler sees that the next use only overwrites the slice's slots.
	runtime.KeepAlive(cache)

	// A long-lived slice keeps the objects alive while it still stores pointers to them.
	// Clearing removes those references; any other aliases would still keep targets alive.
	clearObjectCache(cache[:cap(cache)])
	runtime.GC()
	printMemory("after clearing object references")
	runtime.KeepAlive(cache)

	/* Section 7: Use sync.Pool for hot temporary objects */
	fmt.Println("\n== 7. Use sync.Pool for hot temporary objects ==")

	// sync.Pool is for temporary reusable objects.
	// Get may return a new object at any time; correctness cannot require reuse.
	for i := 0; i < 3; i++ {
		checksum := checksumWithPool("pooled temporary buffer")
		fmt.Println("checksum with pooled buffer:", checksum)
	}

	/* Section 8: Tune GC frequency with GOGC */
	fmt.Println("\n== 8. Tune GC frequency with GOGC ==")

	// The growth target accounts for the live heap and GC roots. It is a target,
	// not a hard byte limit, and the memory limit can cause earlier collections.
	// Lower values usually mean lower memory usage and more GC CPU work.
	// Higher values usually mean higher memory usage and less frequent GC work.
	settings := []metrics.Sample{{Name: "/gc/gogc:percent"}, {Name: "/gc/gomemlimit:bytes"}}
	metrics.Read(settings) // Observe the experiment's settings without overriding them.
	gcPercent := settings[0].Value.Uint64()
	if gcPercent == math.MaxUint64 {
		fmt.Println("configured GOGC: off")
	} else {
		fmt.Println("configured GOGC:", gcPercent)
	}

	/* Section 9: Tune the soft memory limit */
	fmt.Println("\n== 9. Tune the soft memory limit ==")

	// This is a soft limit for Go runtime-managed memory, not all process memory.
	// The runtime may exceed it; it cannot collect reachable data to satisfy it.
	fmt.Println("configured memory limit:", formatMemoryLimit(int64(settings[1].Value.Uint64())))
	stats := readMemoryStats()
	fmt.Println("runtime-managed memory (Sys - HeapReleased):", formatBytes(stats.Sys-stats.HeapReleased))

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
