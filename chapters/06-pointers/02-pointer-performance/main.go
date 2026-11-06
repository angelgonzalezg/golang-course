// Pointer Performance
// Topic: distinguish value-copy semantics, pointer aliasing, and heap allocations.
// Usage: run this walkthrough, then use the benchmarks to measure specific cases.
// Key idea: choose ownership first; the compiler and use site determine actual costs.
package main

import (
	"fmt"
	"runtime"
)

const (
	kib = 1024
	mib = 1024 * kib
)

// Three bounded sizes show the effect of payload size without 100 MB objects.
type SmallPayload struct {
	Data [16]byte
}

type MediumPayload struct {
	Data [4 * kib]byte
}

type LargePayload struct {
	Data [mib]byte
}

// Return a value when the caller needs an independently editable snapshot.
// This payload contains an array of bytes; structs with slices/maps need separate cloning.
func existingSmallValue(source *SmallPayload) SmallPayload { return *source }

// Return this pointer when sharing the existing object's identity is intended.
// No new payload is constructed, but retaining the pointer keeps its target alive.
func existingSmallPointer(source *SmallPayload) *SmallPayload { return source }

// More bytes can mean more copy work; measure rather than using a fixed size cutoff.
func existingMediumValue(source *MediumPayload) MediumPayload    { return *source }
func existingMediumPointer(source *MediumPayload) *MediumPayload { return source }

// A large returned value can also require a heap temporary, adding allocation/GC work.
func existingLargeValue(source *LargePayload) LargePayload    { return *source }
func existingLargePointer(source *LargePayload) *LargePayload { return source }

// Use a value result for small, independent data without a shared-identity requirement.
func newSmallValue(seed byte) SmallPayload {
	var payload SmallPayload
	payload.Data[0] = seed
	payload.Data[len(payload.Data)-1] = seed + 1
	return payload
}

// Returning the address of a local variable is safe. Inlining and escape
// analysis can keep it local to a caller that does not retain the pointer.
// Each call creates a distinct object. A pointer API is useful when callers need
// stable identity or shared mutation; its allocation cost depends on how they use it.
func newSmallPointer(seed byte) *SmallPayload {
	payload := newSmallValue(seed)
	return &payload
}

func main() {
	fmt.Printf("Toolchain: %s, %s/%s, GOMAXPROCS=%d\n",
		runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0))

	fmt.Println("\n== 1. Copies and aliases have different ownership ==")
	source := newSmallValue(1)
	// Value in use: editing the snapshot should preserve the source array.
	copied := existingSmallValue(&source)
	// Pointer in use: edits through this alias should reach the original object.
	aliased := existingSmallPointer(&source)
	copied.Data[0] = 10
	fmt.Printf("after editing the copy: source=%d, copy=%d\n", source.Data[0], copied.Data[0])
	aliased.Data[0] = 20
	fmt.Printf("after editing the pointer target: source=%d, copy=%d\n", source.Data[0], copied.Data[0])

	fmt.Println("\n== 2. A returned pointer can own fresh data ==")
	// These pointers refer to distinct objects. Returning *T does not imply that
	// separate calls share one target; read the constructor's ownership contract.
	first := newSmallPointer(1)
	second := newSmallPointer(1)
	first.Data[0] = 99
	fmt.Printf("independent results: first=%d, second=%d\n", first.Data[0], second.Data[0])
	fmt.Println("Returning a pointer does not by itself decide stack or heap placement.")

	fmt.Println("\n== 3. Measure a specific use site ==")
	fmt.Println("Payloads: 16 B, 4 KiB, and 1 MiB; KiB=1024 B, MiB=1048576 B.")
	fmt.Println("ExistingData compares a value snapshot with an alias to existing storage.")
	fmt.Println("NewSmall compares caller-local results with results retained in global sinks.")
	fmt.Println("Run from the repository root:")
	fmt.Println("go test -run='^$' -bench=. -benchmem ./chapters/06-pointers/02-pointer-performance")
	fmt.Println("Read README.md for escape analysis and benchmark interpretation.")
}
