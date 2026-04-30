// Slices
// Topic: slices are dynamic views over arrays and can grow with append.
// Args/Input: examples create, append, slice ranges, and clone slices.
// Usage: use slices for most list-like data when size can change.
// Key idea: slices are flexible, but multiple slices may share the same backing array.
package main

import (
	"fmt"
	"slices"
)

func main() {

	/* Initialize a slice literal */
	names := []string{"Alice", "John", "Mark"}
	fmt.Println(names)

	/* make creates len 3, cap 5 */
	items := make([]int, 3, 5)
	fmt.Printf("Items: %+v, Len: %d, Capacity: %d\n", items, len(items), cap(items))

	items = append(items, 1)
	items = append(items, 2)

	/* append grows capacity when needed */
	items = append(items, 3)
	items = append(items, 4)
	fmt.Printf("Items: %+v, Len: %d, Capacity: %d\n", items, len(items), cap(items))

	/* Slice range from index 3 to 6 */
	fmt.Printf("%+v\n", items[3:7])

	/* Slices can share the same backing array */
	slice1 := []int{1, 2, 3}
	fmt.Printf("%+v\n", items[3:7])

	slice2 := slice1[0:2]
	fmt.Printf("%+v\n", slice2)
	slice2[0] = 5

	slice3 := slice1[1:3]
	fmt.Printf("%+v\n", slice3)
	slice3[1] = 10

	fmt.Printf("%+v\n", slice1)

	/* Copying a slice without sharing memory */

	dupSlice1 := slices.Clone(slice1) // slices package go 1.21+

	//dupSlice1 := make([]int, len(slice1))
	//copy(dupSlice1, slice1) // built-in copy() function

	// dupSlice1 := append([]int(nil), slice1...) // using nil slice as the base

	dupSlice1[2] = 15
	fmt.Printf("%+v\n", slice1)
	fmt.Printf("%+v\n", dupSlice1)

}
