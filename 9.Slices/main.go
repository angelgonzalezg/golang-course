// Slices
// Topic: slices are dynamic lists that can grow with append.
// Args/Input: examples use []int and []string values.
// Usage: create slices, read/update elements, append values, range over values, and copy slices.
// Key idea: use slices for most list-like data when the size can change.
package main

import (
	"fmt"
	"slices"
)

/* Helper section 1: Slice printing helpers */
func printIntSlice(label string, values []int) {
	// This helper prints the parts beginners usually need to inspect.
	fmt.Printf("%s: value=%v len=%d cap=%d isNil=%t\n", label, values, len(values), cap(values), values == nil)
}

func printStringSlice(label string, values []string) {
	// len is the visible length; cap is the available space before growing.
	fmt.Printf("%s: value=%v len=%d cap=%d isNil=%t\n", label, values, len(values), cap(values), values == nil)
}

func main() {
	/* Section 1: Nil slice vs empty slice */
	fmt.Println("== 1. Nil slice vs empty slice ==")

	/* Nil slice: zero value, no backing array. */
	// Prefer nil when "empty" and "not set" mean the same thing.
	var nilSlice []int
	printIntSlice("nil slice", nilSlice)

	/* Zero-length slice: initialized but empty. */
	// Use it when you want to return [] instead of nil, for example in JSON.
	emptySlice := []int{}
	printIntSlice("empty slice", emptySlice)

	// Both work with len, cap, range, and append.
	nilSlice = append(nilSlice, 1)
	emptySlice = append(emptySlice, 1)
	printIntSlice("nil slice after append", nilSlice)
	printIntSlice("empty slice after append", emptySlice)

	/* Section 2: Slice literal */
	fmt.Println("\n== 2. Slice literal ==")

	/* Initialize a slice literal */
	// Use a literal when you already know the values.
	names := []string{"Alice", "John", "Mark"}
	printStringSlice("names", names)

	// Read by index.
	fmt.Println("first name:", names[0])

	// Update by index.
	names[1] = "Jane"
	printStringSlice("names after update", names)

	/* Section 3: Make with length and capacity */
	fmt.Println("\n== 3. Make with length and capacity ==")

	// make([]Type, len, cap)
	// Use make when you know the size, but not the values yet.
	scores := make([]int, 3, 5)
	printIntSlice("scores from make", scores)

	// Existing positions start with the zero value of the element type.
	scores[0] = 70
	scores[1] = 80
	scores[2] = 90
	printIntSlice("scores after index updates", scores)

	/* Section 4: Append and capacity */
	fmt.Println("\n== 4. Append and capacity ==")

	// len: elements currently in the slice.
	// cap: space available before append may allocate.
	// When full, append grows capacity automatically.
	scores = append(scores, 95)
	scores = append(scores, 100)
	printIntSlice("scores after append within capacity", scores)

	scores = append(scores, 110)
	printIntSlice("scores after append grows capacity", scores)

	extraScores := []int{120, 130}
	scores = append(scores, extraScores...) // ... appends each value from another slice.
	printIntSlice("scores after appending another slice", scores)

	/* Section 5: Range over a slice */
	fmt.Println("\n== 5. Range over a slice ==")

	for index, score := range scores {
		fmt.Printf("scores[%d] = %d\n", index, score)
	}

	// Use _ when you do not need the index.
	total := 0
	for _, score := range scores {
		total += score
	}
	fmt.Println("total score:", total)

	// Use the index when you want to change the original slice.
	for index := range scores {
		scores[index] += 1
	}
	printIntSlice("scores after index update", scores)

	/* Section 6: Basic slice ranges */
	fmt.Println("\n== 6. Basic slice ranges ==")

	// slice[low:high] includes low and excludes high.
	firstThree := scores[0:3]
	printIntSlice("scores[0:3]", firstThree)

	// Missing low means start at 0.
	fromStart := scores[:2]
	printIntSlice("scores[:2]", fromStart)

	// Missing high means go to the end.
	toEnd := scores[3:]
	printIntSlice("scores[3:]", toEnd)

	// A slice range is a view, so changing it can change the original slice.
	firstThree[0] = 999
	printIntSlice("scores after firstThree change", scores)
	printIntSlice("firstThree after change", firstThree)

	/* Section 7: Copy and clone slices */
	fmt.Println("\n== 7. Copy and clone slices ==")

	/* Copy part of a slice */
	// copy stops at the shorter slice: len(dst) or len(src).
	sourceScores := []int{10, 20, 30, 40, 50}
	firstThreeCopy := make([]int, 3)
	copied := copy(firstThreeCopy, sourceScores)
	fmt.Printf("firstThreeCopy: copied=%d value=%v\n", copied, firstThreeCopy)

	/* Clone a slice */
	// slices.Clone creates a new slice with the same values.
	clonedScores := slices.Clone(sourceScores)
	clonedScores[0] = 999
	fmt.Printf("sourceScores after clone change: %v\n", sourceScores)
	fmt.Printf("clonedScores after change:       %v\n", clonedScores)

	/* Section 8: Arrays and slices */
	fmt.Println("\n== 8. Arrays and slices ==")

	/* Array to slice */
	// [:] means "all elements as a slice".
	sourceArray := [5]int{100, 200, 300, 400, 500}
	sliceFromArray := sourceArray[:]
	printIntSlice("slice from array", sliceFromArray)

	/* Slice to array copy */
	arrayCopiedFromSlice := [3]int{}
	copied = copy(arrayCopiedFromSlice[:], sourceScores)
	fmt.Printf("array copied from slice: copied=%d value=%v\n", copied, arrayCopiedFromSlice)

	/* Slice to array value */
	// This panics if the slice is too short.
	arrayValueFromSlice := [3]int(sourceScores[:3])
	fmt.Printf("array value from slice: value=%v\n", arrayValueFromSlice)

	/* Section 9: Final slice rule */
	fmt.Println("\n== 9. Final slice rule ==")

	// Slices are flexible and are the usual choice for changing list-like data.
	// The next lesson covers backing arrays, shared memory, and append gotchas.
	fmt.Println("Use slices for list-like data when the size can change.")
}
