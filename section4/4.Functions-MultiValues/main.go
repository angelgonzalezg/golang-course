// Functions with Multiple Return Values
// Topic: Go functions can return more than one value, commonly result + error.
// Args/Input: divide receives two ints; splitName receives one full name string.
// Usage: return errors for invalid cases and use named returns when it improves clarity.
// Key idea: multiple returns make result-plus-error handling simple and explicit.
package main

import (
	"errors"
	"fmt"
	"strings"
)

func divide(a, b int) (int, error) {
	if b == 0 { // Return error instead of panicking
		return 0, errors.New("Divide by zero")
	}
	return a / b, nil
}

/* Named return values: firstName and lastName can be returned with plain return */
func splitName(fullName string) (firstName, lastName string) {
	parts := strings.Split(fullName, " ") // Split by space
	firstName = parts[0]
	lastName = parts[1]
	return
}

func main() {
	value, err := divide(10, 0)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(value)
	}

	firstName, lastName := splitName("Angel Gonzalez")
	println(firstName, lastName)
}
