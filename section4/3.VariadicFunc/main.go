// Variadic Functions
// Topic: a function can accept zero or more arguments of the same type.
// Args/Input: numbers ...int is received inside the function as a []int slice.
// Usage: call with values like sum(1, 2, 3), with no values like config(), or pass a slice using nums...
// Key idea: variadic parameters are regular slices inside the function.
package main

import "fmt"

func sum(numbers ...int) int {
	total := 0
	for _, number := range numbers { // Iterate over all received numbers
		total += number
	}
	return total
}

func config(numbers ...int) {
	if len(numbers) > 0 { // Check optional arguments
		first := numbers[0] // First value in the variadic slice
		fmt.Println("First number:", first)
	} else {
		fmt.Println("Default number")
	}
}

func main() {
	fmt.Println(sum(1, 2, 3, 4, 5)) // Multiple args
	config()                        // Zero args
}
