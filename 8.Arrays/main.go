// Arrays
// Topic: arrays store a fixed number of elements of the same type.
// Args/Input: examples create arrays, update elements, compare arrays, and pass arrays to functions.
// Usage: use arrays when the size is known and should not change.
// Key idea: array length is part of its type, so [3]int and [4]int are different types.
package main

import "fmt"

/* Helper section 1: Array helper functions */
func changeArrayCopy(values [3]int) {
	// This changes only the copy received by the function.
	values[0] = 999
	fmt.Println("inside changeArrayCopy:", values)
}

func updateArray(values [3]int) [3]int {
	// Return the changed array when the caller needs the new value.
	values[0] = 999
	return values
}

func main() {
	/* Section 1: Declare an array */
	fmt.Println("== 1. Declare an array ==")

	// Fixed-size array of 3 ints; zero values are 0.
	var numbers [3]int
	fmt.Println(numbers)

	/* Section 2: Assign and read by index */
	fmt.Println("\n== 2. Assign and read by index ==")

	numbers[0] = 10
	numbers[1] = 20
	numbers[2] = 30

	fmt.Println(numbers)
	fmt.Println("first value:", numbers[0])
	fmt.Println("last value:", numbers[2])

	/* Section 3: Array length */
	fmt.Println("\n== 3. Array length ==")

	fmt.Println("length:", len(numbers))

	// The index must be between 0 and len(array)-1.
	// fmt.Println(numbers[3]) // This would not compile.

	/* Section 4: Array literals */
	fmt.Println("\n== 4. Array literals ==")

	primes := [4]int{2, 3, 5, 7}
	fmt.Println("primes:", primes)

	// ... lets Go count the array length from the values.
	colors := [...]string{"red", "green", "blue"}
	fmt.Println("colors:", colors)
	fmt.Printf("colors type: %T\n", colors)

	/* Section 5: Partial array literals */
	fmt.Println("\n== 5. Partial array literals ==")

	// Missing values use the zero value for the element type.
	var scores = [5]int{90, 80}
	fmt.Println("scores:", scores)

	// You can set values at specific indexes.
	weekend := [7]bool{0: true, 6: true}
	fmt.Println("weekend days:", weekend)

	/* Section 6: Update array elements */
	fmt.Println("\n== 6. Update array elements ==")

	primes[3] = 11
	fmt.Println("updated primes:", primes)

	/* Section 7: Loop with indexes */
	fmt.Println("\n== 7. Loop with indexes ==")

	for index := 0; index < len(primes); index++ {
		fmt.Printf("primes[%d] = %d\n", index, primes[index])
	}

	/* Section 8: Range over an array */
	fmt.Println("\n== 8. Range over an array ==")

	for index, color := range colors {
		fmt.Printf("colors[%d] = %s\n", index, color)
	}

	// Use _ when you do not need the index.
	for _, color := range colors {
		fmt.Println("color value:", color)
	}

	/* Section 9: Compare arrays */
	fmt.Println("\n== 9. Compare arrays ==")

	// Arrays can be compared when their element type is comparable.
	a := [3]int{1, 2, 3}
	b := [3]int{1, 2, 3}
	c := [3]int{3, 2, 1}

	fmt.Println("a == b:", a == b)
	fmt.Println("a == c:", a == c)

	/* Section 10: Array length is part of the type */
	fmt.Println("\n== 10. Array length is part of the type ==")

	twoNumbers := [2]int{1, 2}
	threeNumbers := [3]int{1, 2, 3}

	fmt.Printf("twoNumbers type: %T\n", twoNumbers)
	fmt.Printf("threeNumbers type: %T\n", threeNumbers)

	// twoNumbers = threeNumbers // This would not compile.

	/* Section 11: Arrays are copied by value */
	fmt.Println("\n== 11. Arrays are copied by value ==")

	original := [3]int{1, 2, 3}
	copyOfOriginal := original
	copyOfOriginal[0] = 100

	fmt.Println("original:", original)
	fmt.Println("copy:", copyOfOriginal)

	/* Section 12: Arrays passed to functions are copied */
	fmt.Println("\n== 12. Arrays passed to functions are copied ==")

	changeArrayCopy(original)
	fmt.Println("after changeArrayCopy:", original)

	original = updateArray(original)
	fmt.Println("after updateArray:", original)

	/* Section 13: Multidimensional arrays */
	fmt.Println("\n== 13. Multidimensional arrays ==")

	// Multidimensional array: 2 rows x 3 columns.
	matrix := [2][3]int{
		{1, 2, 3},
		{4, 5, 6},
	}

	fmt.Println(matrix)
	fmt.Println("row 1, column 2:", matrix[1][2])

	/* Section 14: Array to slice preview */
	fmt.Println("\n== 14. Array to slice preview ==")

	// Slices are covered next. [:] creates a slice view of the array.
	sliceFromArray := primes[:]
	fmt.Println("slice from primes:", sliceFromArray)

	/* Section 15: Final array rules */
	fmt.Println("\n== 15. Final array rules ==")

	// Arrays are fixed-size values.
	// Use arrays for fixed data, like small grids or exact-size buffers.
	// Use slices when the list needs to grow or shrink.
	fmt.Println("Arrays are useful when the size is part of the meaning.")
}
