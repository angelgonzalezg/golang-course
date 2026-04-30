// Functions 2
// Topic: demonstrates three function patterns: recursion, closures, and anonymous functions.
// Args/Input: factorial receives n; intSeq has no args but returns a function; logger receives msg.
// Usage: use recursion for repeated self-calls, closures for saved state, and anonymous funcs for quick logic.
// Key idea: functions are values, can keep state, and can call themselves.
package main

import "fmt"

// Recursion: factorial calls itself until it reaches the base case.
func factorial(n int) int {
	if n <= 1 { // Base case stops recursion
		return 1
	}
	return n * factorial(n-1) // Recursive call
}

// Closure: returned function remembers and updates i between calls.
func intSeq() func() int {
	i := 0
	return func() int {
		i++ // i is captured from the outer function
		return i
	}
}

func main() {

	fmt.Println(factorial(5))

	nextInt := intSeq()
	fmt.Println(nextInt())
	fmt.Println(nextInt())
	fmt.Println(nextInt())

	// Anonymous function: function value stored in a variable.
	logger := func(msg string) {
		fmt.Println(msg)
	}

	logger("Hello World")
	logger("Hello World")

}
