// For Loops
// Topic: Go uses only the for keyword for loop logic.
// Args/Input: examples iterate counters and a slice of numbers.
// Usage: for can work as C-style, while-style, infinite, and range loops.
// Key idea: Go has one loop keyword, but it covers several loop patterns.
package main

import "fmt"

func main() {

	/* C-style for: init; condition; post */
	sum := 0
	for i := 0; i < 10; i++ {
		sum += i
	}
	fmt.Println(sum) // Output: 45

	/* While-style for: only condition */
	sum2 := 1
	for sum2 < 1000 {
		sum2 += sum2
	}
	fmt.Println(sum2) // Output: 1024

	/* Infinite loop: use break to exit */
	counter := 0
	for {
		if counter >= 3 {
			break
		}
		fmt.Println(counter)
		counter++
	}

	/* Range loop: iterate over collections */
	numbers := []int{1, 2, 3, 4, 5}

	// Loop through a slice and get both index and value
	for index, value := range numbers {
		fmt.Printf("Index: %d, Value: %d\n", index, value)
	}

	// To ignore the index, use the blank identifier (_)
	for _, value := range numbers {
		fmt.Println("Value:", value)
	}
}
