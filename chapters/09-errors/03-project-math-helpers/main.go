// Project: Math Helpers with Errors
// Topic: checked integer arithmetic, sentinel errors, and custom error details.
// Input: Sum accepts integers; SafeDivision receives a dividend and divisor.
// Usage: check err before using a result; inspect errors with errors.Is and errors.As.
package main

import (
	"errors"
	"fmt"
	"math"
)

var (
	ErrDivisionByZero = errors.New("division by zero")
	ErrOverflow       = errors.New("integer overflow")
)

// MathError describes the pair of operands that caused an operation to fail.
// For Sum, InputA is the running total and InputB is the next number.
type MathError struct {
	Operation string
	InputA    int
	InputB    int
	Err       error
}

func (e *MathError) Error() string {
	return fmt.Sprintf("%s(%d, %d): %v", e.Operation, e.InputA, e.InputB, e.Err)
}

// Unwrap lets errors.Is inspect the cause through MathError and other wrappers.
func (e *MathError) Unwrap() error {
	return e.Err
}

// Sum adds from left to right. No arguments produce (0, nil).
// An intermediate overflow returns (0, error), never a partial sum.
func Sum(numbers ...int) (int, error) {
	total := 0
	for _, number := range numbers {
		// Check before adding. The subtractions themselves stay in the int range.
		if (number > 0 && total > math.MaxInt-number) ||
			(number < 0 && total < math.MinInt-number) {
			return 0, &MathError{
				Operation: "sum",
				InputA:    total,
				InputB:    number,
				Err:       ErrOverflow,
			}
		}
		total += number
	}
	return total, nil
}

// SafeDivision performs integer division, truncating toward zero.
// It rejects division by zero and a quotient outside the int range.
func SafeDivision(a, b int) (int, error) {
	var cause error
	switch {
	case b == 0:
		cause = ErrDivisionByZero
	case a == math.MinInt && b == -1:
		// Go defines this overflowing division to return MinInt. This helper
		// instead reports that the mathematical quotient cannot fit in int.
		cause = ErrOverflow
	default:
		return a / b, nil
	}

	return 0, &MathError{
		Operation: "division",
		InputA:    a,
		InputB:    b,
		Err:       cause,
	}
}

func main() {
	// Completion output belongs to the caller; arithmetic helpers do not print.
	defer fmt.Println("\nMath examples finished.")

	fmt.Println("== Sums ==")
	for _, numbers := range [][]int{{1, 2, 3}, {}, {-5, 8, -2}, {math.MaxInt, 1}} {
		value, err := Sum(numbers...)
		if err != nil {
			fmt.Printf("Sum(%v) failed: %v\n", numbers, err)
			continue // The returned value is not a successful result.
		}
		fmt.Printf("Sum(%v) = %d\n", numbers, value)
	}

	fmt.Println("\n== Integer division ==")
	for _, inputs := range [][2]int{{10, 2}, {7, 2}, {-7, 2}, {0, 5}, {10, 0}, {math.MinInt, -1}} {
		a, b := inputs[0], inputs[1]
		value, err := SafeDivision(a, b)
		if err != nil {
			fmt.Printf("SafeDivision(%d, %d) failed: %v\n", a, b, err)
			continue
		}
		fmt.Printf("SafeDivision(%d, %d) = %d\n", a, b, value)
	}

	fmt.Println("\n== Inspecting a wrapped error ==")
	value, err := SafeDivision(10, 0)
	if err != nil {
		wrapped := fmt.Errorf("calculate ratio: %w", err)
		fmt.Println(wrapped)
		// Is answers which cause occurred, even through both wrappers.
		fmt.Println("division by zero:", errors.Is(wrapped, ErrDivisionByZero))
		// As extracts structured details; callers do not need to parse text.
		var mathErr *MathError
		if errors.As(wrapped, &mathErr) {
			fmt.Printf("operation=%s, a=%d, b=%d\n", mathErr.Operation, mathErr.InputA, mathErr.InputB)
		}
		return
	}
	fmt.Println("ratio:", value)
}
