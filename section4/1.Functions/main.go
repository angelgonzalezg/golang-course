// Functions
// Topic: functions group reusable logic and can receive parameters and return values.
// Args/Input: examples receive names, ints, and float64 dimensions.
// Usage: call functions from main to organize greeting, math, and validation logic.
// Key idea: functions make code reusable by separating inputs, logic, and outputs.
package main

import "fmt"

func greet(name string) {
	fmt.Printf("Hello, %s\n", name)
}

/* Function signature: name + parameters */
func add(a, b int) {
	fmt.Printf("%d + %d = %d\n", a, b, a+b)
}

func calculateArea(width float64, height float64) float64 {
	if width < 0 || height < 0 { // Validate invalid dimensions
		fmt.Println("Error: width and height must be positive")
	}
	return width * height
}

func main() {
	greet("Alice Wonderland")
	add(1, 2)
	area := calculateArea(4, 4)
	fmt.Println(area)
}
