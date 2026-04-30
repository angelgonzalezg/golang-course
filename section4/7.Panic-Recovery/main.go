// Panic and Recovery
// Topic: panic stops normal execution; recover can catch a panic inside a deferred function.
// Args/Input: mightPanic receives a bool to decide whether to panic.
// Usage: prefer returning errors, but use recover to protect boundaries from unexpected panics.
// Key idea: recover only works inside deferred functions during a panic.
package main

import "fmt"

func mightPanic(shouldPanic bool) {
	if shouldPanic {
		panic("Something went wrong in mightPanic") // Triggers panic
	}

	fmt.Println("This function executed without panic")
}

func recoverable() {
	defer func() {
		if r := recover(); r != nil { // recover works only inside deferred function
			fmt.Println("Recovered from panic:", r)
		}
	}()

	mightPanic(true)
}

func main() {
	recoverable()
}
