// Error Handling
// Topic: errors are returned as values and checked explicitly.
// Args/Input: divide receives ints; DoSomething returns a custom error.
// Usage: use errors.Is for sentinel errors and errors.As for custom error types.
// Key idea: errors are normal values that can be compared, wrapped, or type-checked.
package main

import (
	"errors"
	"fmt"
	"time"
)

var ErrDivisionByZero = errors.New("division by zero")
var ErrNumTooLarge = errors.New("number too large")

/* Custom error type with extra fields */
type MyError struct {
	Op      string
	Code    int
	Message string
	Time    time.Time
}

func (e *MyError) Error() string {
	// Error method makes MyError implement the error interface.
	return fmt.Sprintf("codigo %d: %s", e.Code, e.Message)
}

func NewOpError(op string, code int, message string) *MyError {
	return &MyError{
		Op:      op,
		Code:    code,
		Message: message,
		Time:    time.Now().UTC(),
	}
}

func DoSomething() error {
	return NewOpError(
		"doSomething",
		100,
		"do something failed",
	)
}

func divide(a, b int) (int, error) {
	if b == 0 { // Sentinel error
		return 0, ErrDivisionByZero
	}

	if a > 1000 { // Another sentinel error
		return 0, ErrNumTooLarge
	}
	return a / b, nil
}

func main() {
	/* Case 1 - Simple errors with errors.Is */
	// value, err := divide(1001, 1)
	// if err != nil {
	// 	if errors.Is(err, ErrDivisionByZero) {
	// 		fmt.Println("divide by zero")
	// 	} else if errors.Is(err, ErrNumTooLarge) {
	// 		fmt.Println("number too large")
	// 	}
	// 	return
	// }
	// fmt.Println(value)

	/* Case 2 - Custom errors with errors.As */
	err := DoSomething()
	if err != nil {

		// print the custom error
		fmt.Println("error:", err)

		// extract the structured data
		var myErr *MyError
		if errors.As(err, &myErr) {
			fmt.Println("Operation:", myErr.Op)
			fmt.Println("Code:", myErr.Code)
			fmt.Println("Message:", myErr.Message)
			fmt.Println("Time:", myErr.Time)
		}
	}
}
