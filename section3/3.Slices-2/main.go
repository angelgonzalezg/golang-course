// Slices 2
// Topic: slice expressions create views with their own len and cap.
// Args/Input: examples slice one original []int in different ways.
// Usage: understand low:high ranges, capacity, contains, and append behavior.
// Key idea: slicing changes the view, not necessarily the underlying data.
package main

import (
	"fmt"
	"slices"
)

func main() {
	original := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	fmt.Printf("Original: %v, len: %d, cap: %d\n", original, len(original), cap(original))

	slice1 := original[2:5] // low:high, excludes high
	fmt.Printf("slice1: %v, len: %d, cap: %d\n", slice1, len(slice1), cap(slice1))

	slice2 := original[:4] // low:high (high not included)
	fmt.Printf("slice2: %v, len: %d, cap: %d\n", slice2, len(slice2), cap(slice2))

	slice3 := original[6:] // low:end
	fmt.Printf("slice3: %v, len: %d, cap: %d\n", slice3, len(slice3), cap(slice3))

	slice4 := original[:] // start:end, full slice
	fmt.Printf("slice4: %v, len: %d, cap: %d\n", slice4, len(slice4), cap(slice4))

	exists := slices.Contains(slice4, 8) // Check if value exists
	fmt.Println(exists)

	slice4 = append(slice4, 1000) // May grow capacity
	fmt.Printf("modified(slice4): %v, len: %d, cap: %d\n", slice4, len(slice4), cap(slice4))

}
