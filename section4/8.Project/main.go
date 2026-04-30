// Project: Math Helpers with Errors
// Topic: combines variadic functions, defer, custom errors, and safe error handling.
// Args/Input: Sum accepts many integers; SafeDivision receives two integers and may return an error.
// Usage: return errors for invalid operations and handle them in main instead of crashing.
// Key idea: safe functions return errors so callers decide how to handle failures.
package main

import (
	"fmt"
	"strings"
)

const (
	divisionOp     = "Division"
	divisionErrMsg = "division by zero is not allowed"
)

type MathError struct {
	Operation string
	InputA    int
	InputB    int
	Message   string
}

// Error formats MathError so it satisfies Go's error interface.
func (e MathError) Error() string {
	var inputs []string
	if e.Operation == divisionOp { // Add operation-specific details
		inputs = append(inputs, fmt.Sprintf("a=%d", e.InputA))
		inputs = append(inputs, fmt.Sprintf("b=%d", e.InputB))
	}
	return fmt.Sprintf("Math error in %s (%s): %s",
		e.Operation,
		strings.Join(inputs, ","),
		e.Message)
}

func Sum(numbers ...int) int {
	defer fmt.Println("Sum finished") // Runs when Sum finishes

	total := 0
	for _, n := range numbers { // Add each provided value
		total += n
	}
	return total
}

func SafeDivision(a, b int) (int, error) {
	if b == 0 { // Avoid division-by-zero panic
		return 0, &MathError{
			Operation: divisionOp,
			InputA:    a,
			InputB:    b,
			Message:   divisionErrMsg,
		}
	}
	return a / b, nil
}

func main() {
	fmt.Println(Sum(1, 2, 3))

	value, err := SafeDivision(10, 0)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("SafeDivision", value)
}
