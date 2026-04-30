// Variables
// Topic: variables store values that can change while the program runs.
// Args/Input: this example creates strings and numbers using var and :=.
// Usage: use var for explicit declarations and := for short inferred declarations inside functions.
// Key idea: variables have zero values and can use explicit or inferred types.
package main

import "fmt"

func main() {
	var greeting string // Zero value is an empty string
	greeting = "Hello World!"
	fmt.Println(greeting)

	// int zero value starts at 0
	var count int
	count += 1
	fmt.Println(count)

	var firstName, lastName string
	firstName = "John"
	lastName = "Doe"
	fmt.Println(firstName, lastName)

	email := "test@email.com" // Short declaration infers string
	println(email)

	// Inferred int value
	age := 30
	println(age)

	var year = 2026
	println(year)
}
