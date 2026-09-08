// Functions 2
// Lesson focus: function values, callbacks, returned functions, closures, and recursion.
// Topic: functions can be values, arguments, return values, closures, and recursive calls.
// Args/Input: examples use numbers, names, and simple counters.
// Usage: use these patterns when behavior should be reusable or customizable.
// Key idea: functions are values in Go, so they can be stored, passed, and returned.
package main

import "fmt"

/* Helper section 1: Basic operations used as function values */
func add(a, b int) int {
	return a + b
}

func multiply(a, b int) int {
	return a * b
}

/* Helper section 2: Higher-order function helpers */
/* Function that receives another function */
// operation is a parameter whose type is "function that receives two ints and returns one int".
// Use this when the caller should choose the behavior.
func calculate(a, b int, operation func(int, int) int) int {
	return operation(a, b)
}

/* Function that returns another function */
// The returned function still has access to greeting.
// Use this when you want to create a customized function.
func makeGreeter(greeting string) func(string) {
	return func(name string) {
		fmt.Printf("%s, %s\n", greeting, name)
	}
}

/* Closure: returned function remembers count */
// Use closures when a function needs private state.
func makeCounter() func() int {
	count := 0
	return func() int {
		count++ // count lives after makeCounter returns because the inner function uses it.
		return count
	}
}

/* Recursion: function calls itself */
// Use recursion for problems that repeat in smaller steps.
func factorial(n int) int {
	if n <= 1 {
		return 1 // Base case stops the recursion.
	}

	return n * factorial(n-1)
}

func main() {
	/* Section 1: Function values */
	fmt.Println("== 1. Function values ==")

	/* Function value */
	// Functions can be stored in variables.
	// Use this when you want to choose or replace behavior later.
	operation := add
	fmt.Println("Add:", operation(2, 3))

	// The same variable can store another function with the same signature.
	operation = multiply
	fmt.Println("Multiply:", operation(2, 3))

	/* Section 2: Function as an argument */
	fmt.Println("\n== 2. Function as an argument ==")

	/* Pass a function as an argument */
	// calculate decides when to call add.
	// Common use: callbacks, filters, sort logic, and custom calculations.
	total := calculate(10, 5, add)
	fmt.Println("Calculated total:", total)

	/* Section 3: Anonymous function stored in a variable */
	fmt.Println("\n== 3. Anonymous function stored in a variable ==")

	/* Anonymous function stored in a variable */
	// Anonymous means the function has no declared name.
	// Use this for small behavior needed only in one place.
	double := func(number int) int {
		return number * 2
	}
	fmt.Println("Double:", double(4))

	/* Section 4: Anonymous function called immediately */
	fmt.Println("\n== 4. Anonymous function called immediately ==")

	/* Anonymous function called immediately */
	// Use this when you want a quick calculation with its own local variables.
	message := func(name string) string {
		return "Hello, " + name
	}("Ana")
	fmt.Println(message)

	/* Section 5: Returned function */
	fmt.Println("\n== 5. Returned function ==")

	/* Returned function */
	// makeGreeter creates a reusable greeter with a saved greeting.
	sayHi := makeGreeter("Hi")
	sayHi("Ben")

	/* Section 6: Closure remembers its own state */
	fmt.Println("\n== 6. Closure remembers its own state ==")

	/* Closure remembers its own state */
	// Each call updates the same captured count variable.
	// Common use: counters, configuration, and simple generators.
	nextNumber := makeCounter()
	fmt.Println("Counter:", nextNumber())
	fmt.Println("Counter:", nextNumber())
	fmt.Println("Counter:", nextNumber())

	/* Section 7: Recursive function */
	fmt.Println("\n== 7. Recursive function ==")

	/* Recursive function */
	// factorial keeps calling itself until it reaches the base case.
	fmt.Println("Factorial of 5:", factorial(5))

	/* Section 8: Final advanced function rules */
	fmt.Println("\n== 8. Final advanced function rules ==")

	// Use function values when behavior should be chosen at runtime.
	// Use closures when a function needs private remembered state.
	// Use recursion only when the repeated problem becomes smaller each step.
	fmt.Println("Functions can also be values, callbacks, closures, and recursive logic.")
}
