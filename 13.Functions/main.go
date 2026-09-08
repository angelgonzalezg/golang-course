// Functions
// Topic: functions group reusable logic and can receive parameters and return values.
// Args/Input: examples use strings, ints, floats, and booleans.
// Usage: use functions to organize repeated logic into small named actions.
// Key idea: functions separate inputs, work, and outputs.
package main

import "fmt"

/* Helper section 1: Basic function examples */
/* Function with no parameters and no return value */
func printCourseTitle() {
	fmt.Println("Golang Course")
}

/* Function with one parameter */
func greet(name string) {
	fmt.Printf("Hello, %s\n", name)
}

/* Function with two parameters of the same type */
func greetStudent(firstName, lastName string) {
	fmt.Printf("Welcome, %s %s\n", firstName, lastName)
}

/* Function that returns a value */
func add(a, b int) int {
	return a + b
}

/* Function that uses another function */
func printTotal(subtotal, shipping int) {
	total := add(subtotal, shipping)
	fmt.Printf("Total: %d\n", total)
}

/* Function with validation and early return */
func calculateArea(width, height float64) float64 {
	if width <= 0 || height <= 0 {
		fmt.Println("Width and height must be greater than zero.")
		return 0
	}

	return width * height
}

/* Function that returns a boolean */
func isAdult(age int) bool {
	return age >= 18
}

/* Parameters are copies of the values passed in */
func addBonus(score int) int {
	score += 10
	return score
}

func main() {
	/* Section 1: Function with no parameters and no return value */
	fmt.Println("== 1. Function with no parameters and no return value ==")

	printCourseTitle()

	/* Section 2: Function parameters */
	fmt.Println("\n== 2. Function parameters ==")

	// Arguments are the actual values passed into parameters.
	greet("Alice")
	greetStudent("Alice", "Wonderland")

	/* Section 3: Function return values */
	fmt.Println("\n== 3. Function return values ==")

	sum := add(1, 2)
	fmt.Println("1 + 2 =", sum)

	/* Section 4: Function that uses another function */
	fmt.Println("\n== 4. Function that uses another function ==")

	printTotal(20, 3)

	/* Section 5: Validation and early return */
	fmt.Println("\n== 5. Validation and early return ==")

	area := calculateArea(4, 4)
	fmt.Println("Area:", area)

	invalidArea := calculateArea(-2, 5)
	fmt.Println("Invalid area:", invalidArea)

	/* Section 6: Function that returns a boolean */
	fmt.Println("\n== 6. Function that returns a boolean ==")

	age := 21
	if isAdult(age) {
		fmt.Println("Adult user.")
	} else {
		fmt.Println("User is under 18.")
	}

	/* Section 7: Parameters are copies */
	fmt.Println("\n== 7. Parameters are copies ==")

	score := 80
	newScore := addBonus(score)
	fmt.Println("Original score:", score)
	fmt.Println("New score:", newScore)

	/* Section 8: Final function rules */
	fmt.Println("\n== 8. Final function rules ==")

	// Keep functions small and give them names that explain their job.
	// Parameters are inputs, return values are outputs.
	// Use early returns to handle invalid input clearly.
	fmt.Println("Functions help organize code into reusable actions.")
}
