// Defer
// Topic: defer schedules a function call to run after the surrounding function returns.
// Args/Input: examples defer print statements and function calls.
// Usage: commonly used for cleanup like closing files, unlocking mutexes, or logging completion.
// Key idea: deferred calls run after return in last-in-first-out order.
package main

import "fmt"

/* Single defer: deferred call runs after the normal statements */
func simpleDefer() {
	fmt.Println("Function simpleDefer: Start")
	defer fmt.Println("Function simpleDefer: Deferred") // Runs before simpleDefer exits
	fmt.Println("Function simpleDefer: Middle")
}

/* Multiple defers: run in LIFO order (last in, first out) */
func lifoSimpleDefer() {
	fmt.Println("Function lifoSimpleDefer: Start")
	defer fmt.Println("First: Deferred")
	defer fmt.Println("Second: Deferred")
	fmt.Println("Function lifoSimpleDefer: Middle")
}

func main() {
	defer func() {
		fmt.Println("Before the return of main()")
	}()

	defer simpleDefer() // Runs near the end of main
	lifoSimpleDefer()

	fmt.Println("Last in main()")
}
