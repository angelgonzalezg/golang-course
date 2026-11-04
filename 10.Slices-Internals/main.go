// Slices-Internals
// Lesson focus: slice internals, backing arrays, capacity, append, and shared data.
// Topic: slice expressions create views with their own len and cap.
// Args/Input: examples use []int values, slices helpers, and append.
// Usage: understand shared backing arrays, append behavior, full slice expressions, buffers, and cloning.
// Key idea: slicing changes the view; it does not automatically copy the underlying data.
package main

import (
	"fmt"
	"slices"
)

/* Helper section 1: Slice printing helpers */
func printSlice(label string, values []int) {
	// Printing len and cap makes slice behavior easier to see.
	fmt.Printf("%s: %v, len: %d, cap: %d\n", label, values, len(values), cap(values))
}

func printByteBuffer(label string, values []byte) {
	// %q prints bytes as readable text when possible.
	fmt.Printf("%s: %q, len: %d, cap: %d\n", label, values, len(values), cap(values))
}

/* Helper section 2: Slice mutation helpers */
func changeFirstElement(values []int) {
	values[0] = 999
	printSlice("inside changeFirstElement", values)
}

func appendWithEnoughCapacity(values []int) {
	printSlice("inside appendWithEnoughCapacity before append", values)
	values = append(values, 30, 40)
	printSlice("inside appendWithEnoughCapacity after append", values)
}

func appendWithoutEnoughCapacity(values []int) {
	printSlice("inside appendWithoutEnoughCapacity before append", values)
	values = append(values, 30, 40)
	printSlice("inside appendWithoutEnoughCapacity after append", values)
}

func appendAndReturn(values []int) []int {
	values = append(values, 30, 40)
	return values
}

func main() {
	/* Section 1: Slice expressions */
	fmt.Println("== 1. Slice expressions ==")

	original := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	printSlice("original", original)

	// slice[low:high] includes low and excludes high.
	middle := original[2:5]
	printSlice("original[2:5]", middle)

	// Missing low means "start at 0".
	start := original[:4]
	printSlice("original[:4]", start)

	// Missing high means "go to the end".
	end := original[6:]
	printSlice("original[6:]", end)

	// [:] creates a slice view of the whole thing.
	all := original[:]
	printSlice("original[:]", all)

	/* Section 2: Len and cap after slicing */
	fmt.Println("\n== 2. Len and cap after slicing ==")

	// len is the visible number of elements.
	// cap is how far the slice can grow before append must allocate.
	view := original[3:6]
	printSlice("view original[3:6]", view)
	fmt.Println("view sees indexes 3, 4, and 5 from original.")
	fmt.Println("view capacity continues from index 3 to the end of original.")

	/* Section 3: Shared backing array */
	fmt.Println("\n== 3. Shared backing array ==")

	sharedBase := []int{10, 20, 30, 40, 50}
	sharedView := sharedBase[1:4]
	printSlice("sharedBase before change", sharedBase)
	printSlice("sharedView before change", sharedView)

	// Changing the view changes the backing array.
	sharedView[0] = 99
	printSlice("sharedBase after view change", sharedBase)
	printSlice("sharedView after change", sharedView)

	/* Section 4: Append can reuse hidden capacity */
	fmt.Println("\n== 4. Append can reuse hidden capacity ==")

	appendBase := []int{1, 2, 3, 4}
	appendView := appendBase[:2]
	printSlice("appendBase before append", appendBase)
	printSlice("appendView before append", appendView)

	// appendView has extra capacity, so append can reuse appendBase's backing array.
	appendView = append(appendView, 99)
	printSlice("appendBase after append", appendBase)
	printSlice("appendView after append", appendView)

	/* Section 5: Full slice expression */
	fmt.Println("\n== 5. Full slice expression ==")

	protectedBase := []int{1, 2, 3, 4}
	protectedView := protectedBase[:2:2] // low:high:max
	printSlice("protectedBase before append", protectedBase)
	printSlice("protectedView before append", protectedView)

	// Limiting capacity does not copy data: existing elements are still shared.
	// It also does not release the original backing array.
	protectedView[0] = 42
	printSlice("protectedBase after existing element change", protectedBase)

	// The max value limits capacity, so append must allocate a new backing array.
	protectedView = append(protectedView, 99)
	printSlice("protectedBase after append", protectedBase)
	printSlice("protectedView after append", protectedView)

	/* Section 6: Slices as buffers */
	fmt.Println("\n== 6. Slices as buffers ==")

	// A buffer is a slice used as temporary storage.
	// Start with len 0 and enough capacity for the expected work.
	buffer := make([]byte, 0, 20)
	printByteBuffer("empty buffer", buffer)

	// append adds data while reusing the allocated capacity.
	buffer = append(buffer, 'G', 'o')
	buffer = append(buffer, ' ')
	buffer = append(buffer, 's', 'l', 'i', 'c', 'e', 's')
	printByteBuffer("after building text", buffer)
	fmt.Println("as string:", string(buffer))

	// Reuse the same backing array by setting the length back to 0.
	buffer = buffer[:0]
	printByteBuffer("after reset", buffer)

	buffer = append(buffer, 'r', 'e', 'u', 's', 'e')
	printByteBuffer("after reuse", buffer)
	fmt.Println("as string:", string(buffer))

	/* Section 7: Passing slices to functions */
	fmt.Println("\n== 7. Passing slices to functions ==")

	// A slice is a small value with three parts: pointer, len, and cap.
	// Passing a slice to a function copies those three parts.
	// Both copies can still point to the same backing array.
	functionBase := []int{1, 2, 3}
	printSlice("functionBase before function", functionBase)
	changeFirstElement(functionBase)
	printSlice("functionBase after element change", functionBase)

	// If append has enough capacity, it writes into the shared backing array.
	// The function's copy gets a new len, but the caller's len does not change.
	hasRoom := make([]int, 2, 4)
	hasRoom[0] = 10
	hasRoom[1] = 20
	printSlice("hasRoom before function", hasRoom)
	appendWithEnoughCapacity(hasRoom)
	printSlice("hasRoom after function", hasRoom)
	printSlice("hasRoom resliced to reveal shared memory", hasRoom[:4])

	// If append does not have enough capacity, Go allocates a new backing array.
	// The function's copy gets a new pointer, len, and cap.
	// The caller's slice still points to the old backing array.
	noRoom := []int{10, 20}
	printSlice("noRoom before function", noRoom)
	appendWithoutEnoughCapacity(noRoom)
	printSlice("noRoom after function", noRoom)

	// Return the slice when the caller needs append's new len, cap, or pointer.
	noRoom = appendAndReturn(noRoom)
	printSlice("noRoom after appendAndReturn", noRoom)

	/* Section 8: Clone before modifying */
	fmt.Println("\n== 8. Clone before modifying ==")

	source := []int{100, 200, 300}
	copyView := source[:2]
	clonedView := slices.Clone(copyView)
	clonedView[0] = 777

	printSlice("source after clone change", source)
	printSlice("copyView", copyView)
	printSlice("clonedView", clonedView)

	/* Section 9: Useful slices helpers */
	fmt.Println("\n== 9. Useful slices helpers ==")

	numbers := []int{3, 1, 4, 1, 5}
	fmt.Println("numbers contains 4:", slices.Contains(numbers, 4))
	fmt.Println("index of 5:", slices.Index(numbers, 5))

	// Delete shifts elements in the backing array and clears the obsolete tail.
	// Save its return value because the length changes. Clone first to preserve numbers.
	withoutMiddle := slices.Delete(slices.Clone(numbers), 1, 3)
	printSlice("after Delete [1:3)", withoutMiddle)

	// Insert can reuse the backing array or allocate a new one; always save the result.
	withInserted := slices.Insert(slices.Clone(numbers), 2, 9, 9)
	printSlice("after Insert at index 2", withInserted)
	printSlice("numbers preserved by cloning before Delete and Insert", numbers)

	/* Section 9.1: Clear, reset, and release a slice */
	fmt.Println("\n== 9.1. Clear, reset, and release a slice ==")

	reusable := []int{10, 20, 30}
	clear(reusable) // Sets visible elements to zero without changing len or cap.
	printSlice("after clear", reusable)

	reusable[0] = 99
	reusable = reusable[:0] // Keeps the backing array for reuse without clearing it.
	printSlice("after resetting length", reusable)
	printSlice("reslicing reveals data kept after reset", reusable[:cap(reusable)])

	reusable = nil // Drops this reference; another view could still keep the array alive.
	printSlice("after assigning nil", reusable)
	// For slices holding references, clear obsolete elements when they should no longer retain data.

	/* Section 10: Final slice rule */
	fmt.Println("\n== 10. Final slice rule ==")

	// Slices are small values that point to backing arrays.
	// Clone for independent element storage; nested references need their own copies.
	// Use buffers when you want to build temporary data with append.
	// Use full slice expressions when append should not modify hidden shared capacity.
	fmt.Println("Slicing creates views; cloning copies elements, including any references they contain.")
}
