// Variables
// Topic: variables store values that can change while the program runs.
// Args/Input: this example does not receive input; it creates and updates variables.
// Usage: use var for explicit declarations and := for short declarations inside functions.
// Key idea: a variable has a name, a type, a value, and a scope.
package main

import "fmt"

/* Helper section 1: Package-level variables */
// Package-level variables can be used by every function in this file.
var appName = "GolangCourse"
var totalRuns int

/* Helper section 2: Small helper functions */
func printPackageVariables() {
	totalRuns++
	fmt.Println("app name:", appName)
	fmt.Println("total runs:", totalRuns)
}

func showScope() {
	message := "inside showScope"
	fmt.Println(message)
}

func main() {
	/* Section 1: Declare first, assign later */
	fmt.Println("== 1. Declare first, assign later ==")

	var greeting string
	greeting = "Hello, variables!"
	fmt.Println(greeting)

	/* Section 2: Zero values */
	fmt.Println("\n== 2. Zero values ==")

	var emptyText string
	var count int
	var price float64
	var active bool
	var numbers []int

	fmt.Printf("string: %q\n", emptyText)
	fmt.Println("int:", count)
	fmt.Println("float64:", price)
	fmt.Println("bool:", active)
	fmt.Println("slice is nil:", numbers == nil)

	/* Section 3: Declare and initialize */
	fmt.Println("\n== 3. Declare and initialize ==")

	var firstName string = "Ana"
	var age int = 30
	var height = 1.68 // Go infers float64.

	fmt.Println(firstName)
	fmt.Println(age)
	fmt.Println(height)

	/* Section 4: Short declaration */
	fmt.Println("\n== 4. Short declaration ==")

	email := "ana@example.com"
	score := 95

	fmt.Println(email)
	fmt.Println(score)

	/* Section 5: Inspect inferred types */
	fmt.Println("\n== 5. Inspect inferred types ==")

	fmt.Printf("email type: %T\n", email)
	fmt.Printf("score type: %T\n", score)
	fmt.Printf("height type: %T\n", height)

	/* Section 6: Reassign variables */
	fmt.Println("\n== 6. Reassign variables ==")

	score = 100
	score += 5
	fmt.Println("updated score:", score)

	active = true
	fmt.Println("active:", active)

	/* Section 7: Multiple variables */
	fmt.Println("\n== 7. Multiple variables ==")

	var city, country string
	city = "Mexico City"
	country = "Mexico"
	fmt.Println(city, country)

	width, length := 4, 6
	area := width * length
	fmt.Println("area:", area)

	/* Section 8: Grouped variable declarations */
	fmt.Println("\n== 8. Grouped variable declarations ==")

	var (
		productName = "Notebook"
		quantity    = 3
		unitPrice   = 12.50
	)

	fmt.Println(productName)
	fmt.Println(quantity)
	fmt.Println(unitPrice)

	/* Section 9: Package-level variables */
	fmt.Println("\n== 9. Package-level variables ==")

	printPackageVariables()
	printPackageVariables()

	/* Section 10: Variable scope */
	fmt.Println("\n== 10. Variable scope ==")

	showScope()
	localMessage := "inside main"
	fmt.Println(localMessage)

	if active {
		status := "available"
		fmt.Println(status)
	}
	// status cannot be used here because it belongs to the if block.

	/* Section 11: Shadowing */
	fmt.Println("\n== 11. Shadowing ==")

	level := "beginner"
	fmt.Println("outer level:", level)

	if true {
		level := "advanced" // New variable inside this block.
		fmt.Println("inner level:", level)
	}

	fmt.Println("outer level again:", level)

	/* Section 12: Blank identifier */
	fmt.Println("\n== 12. Blank identifier ==")

	// Use _ only when a returned value is intentionally ignored.
	name, _ := firstAndLastName()
	fmt.Println("first name:", name)

	/* Section 13: Final variable rules */
	fmt.Println("\n== 13. Final variable rules ==")

	// Use var when you need the zero value or want to be explicit.
	// Use := for short local variables inside functions.
	// Avoid shadowing unless it makes the code clearer.
	fmt.Println("Variables let your program remember and update values.")
}

/* Helper section 3: Function used by the blank identifier example */
func firstAndLastName() (string, string) {
	return "Ana", "Lopez"
}
