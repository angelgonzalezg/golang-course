// Arrays
// Topic: arrays store a fixed number of elements of the same type.
// Args/Input: examples create one-dimensional arrays and a small matrix.
// Usage: use arrays when the size is known and should not change.
// Key idea: array length is part of its type, so arrays have fixed size.
package main

import "fmt"

func main() {
	// Fixed-size array of 2 ints; zero values are 0.
	var numbers [2]int
	fmt.Printf("%+v\n", numbers)
	//[1][2][]

	// Assign values by index.
	numbers[0] = 1
	numbers[1] = 2

	fmt.Printf("%+v\n", numbers)

	primes := [4]int{2, 3, 5, 7} // Array literal
	fmt.Printf("%+v\n", primes)

	// Update an element by index.
	primes[3] = 11
	fmt.Printf("%+v\n", primes)

	// Loop using indexes.
	for i := 0; i < len(primes); i++ {
		fmt.Printf("%d\n", primes[i])
	}

	// Multidimensional array: 2 rows x 3 columns.
	var matrix [2][3]int
	matrix[0][0] = 1
	matrix[0][1] = 2
	matrix[1][2] = 3

	fmt.Printf("%+v\n", matrix)
}
