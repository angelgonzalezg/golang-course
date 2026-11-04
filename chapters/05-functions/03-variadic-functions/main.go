// Variadic Functions
// Topic: a variadic function accepts zero or more arguments of the same type.
// Args/Input: examples use numbers, names, labels, strings, and any values.
// Usage: call with many values, no values, or pass a slice using values...
// Key idea: a variadic parameter acts like a slice inside the function.
package main

import (
	"fmt"
	"strings"
)

/* Helper section 1: Variadic helper functions */
/* numbers ...int means this function accepts zero or more int values */
func sum(numbers ...int) int {
	total := 0

	// Inside the function, numbers behaves like a []int slice.
	for _, number := range numbers {
		total += number
	}

	return total
}

/* Variadic functions can receive zero arguments */
func printNames(names ...string) {
	// len tells us how many arguments were passed.
	if len(names) == 0 {
		fmt.Println("No names provided.")
		return
	}

	for _, name := range names {
		fmt.Println("Name:", name)
	}
}

/* Regular parameters must come before the variadic parameter */
// A function can have only one variadic parameter, and it must be last.
func printScores(label string, scores ...int) {
	fmt.Println(label)
	for _, score := range scores {
		fmt.Println("Score:", score)
	}
}

/* Return a safe default when no values are provided */
func average(numbers ...int) float64 {
	if len(numbers) == 0 {
		return 0
	}

	// numbers... expands the slice into separate arguments.
	total := sum(numbers...)
	return float64(total) / float64(len(numbers))
}

/* Forward variadic values to another variadic function */
func printTotal(label string, numbers ...int) {
	total := sum(numbers...)
	fmt.Printf("%s: %d\n", label, total)
}

/* Build text from many string parts */
func joinWords(separator string, words ...string) string {
	return strings.Join(words, separator)
}

/* any accepts values of different types */
func printValues(values ...any) {
	for index, value := range values {
		fmt.Printf("values[%d] = %v (%T)\n", index, value, value)
	}
}

/* The variadic parameter is a slice inside the function */
func updateFirst(numbers ...int) {
	if len(numbers) == 0 {
		return
	}

	numbers[0] = 999
	fmt.Println("inside updateFirst:", numbers)
}

func main() {
	/* Section 1: Call with separate values */
	fmt.Println("== 1. Call with separate values ==")

	/* Call with separate values */
	fmt.Println("Sum with values:", sum(1, 2, 3))

	/* Section 2: Call with no values */
	fmt.Println("\n== 2. Call with no values ==")

	/* Call with no values */
	fmt.Println("Sum with no values:", sum())

	/* Section 3: Pass a slice using ... */
	fmt.Println("\n== 3. Pass a slice using ... ==")

	/* Pass a slice using ... */
	numbers := []int{4, 5, 6}
	fmt.Println("Sum from slice:", sum(numbers...))

	/* Section 4: Variadic string function */
	fmt.Println("\n== 4. Variadic string function ==")

	/* A variadic string function */
	printNames()
	printNames("Ana", "Ben")

	/* Section 5: Regular arguments before variadic arguments */
	fmt.Println("\n== 5. Regular arguments before variadic arguments ==")

	/* One regular argument, then many variadic arguments */
	printScores("Exam scores:", 90, 85, 100)

	/* Section 6: Reuse and forward variadic arguments */
	fmt.Println("\n== 6. Reuse and forward variadic arguments ==")

	/* Reuse a variadic function inside another function */
	fmt.Println("Average:", average(10, 20, 30))
	fmt.Println("Average with no values:", average())
	printTotal("Total from printTotal", 7, 8, 9)

	/* Section 7: Variadic strings with a regular parameter */
	fmt.Println("\n== 7. Variadic strings with a regular parameter ==")

	// The separator is regular; words is variadic.
	fmt.Println(joinWords(" ", "Go", "is", "fun"))
	fmt.Println(joinWords("-", "2026", "06", "13"))

	/* Section 8: Variadic any */
	fmt.Println("\n== 8. Variadic any ==")

	// fmt.Println works like this: it can receive many values of different types.
	printValues("Ana", 30, true, 9.5)

	/* Section 9: Variadic parameter shares a slice when called with ... */
	fmt.Println("\n== 9. Variadic parameter shares a slice when called with ... ==")

	values := []int{1, 2, 3}
	updateFirst(values...)
	fmt.Println("after updateFirst with slice:", values)

	// Separate arguments create the variadic slice for the function call.
	updateFirst(10, 20, 30)

	/* Section 10: Final variadic rules */
	fmt.Println("\n== 10. Final variadic rules ==")

	// Use variadic functions when callers should pass any number of values.
	// Use values... to pass an existing slice.
	// Put the variadic parameter last.
	fmt.Println("Variadic functions are flexible, and inside they work like slices.")
}
