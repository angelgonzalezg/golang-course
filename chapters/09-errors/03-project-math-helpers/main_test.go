package main

import (
	"errors"
	"fmt"
	"math"
	"testing"
)

func TestSum(t *testing.T) {
	tests := []struct {
		name    string
		numbers []int
		want    int
	}{
		{"no arguments", nil, 0},
		{"empty slice", []int{}, 0},
		{"positive", []int{1, 2, 3}, 6},
		{"negative", []int{-1, -2, -3}, -6},
		{"mixed signs", []int{-5, 8, -2}, 1},
		{"zero result", []int{5, -5}, 0},
		{"maximum", []int{math.MaxInt}, math.MaxInt},
		{"minimum", []int{math.MinInt}, math.MinInt},
		{"reach maximum", []int{math.MaxInt - 1, 1}, math.MaxInt},
		{"reach minimum", []int{math.MinInt + 1, -1}, math.MinInt},
		{"zero at maximum", []int{math.MaxInt, 0}, math.MaxInt},
		{"zero at minimum", []int{math.MinInt, 0}, math.MinInt},
		{"cancel extremes", []int{math.MinInt, math.MaxInt}, -1},
		{"cancel extremes in reverse", []int{math.MaxInt, math.MinInt, 1}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Sum(tt.numbers...)
			if err != nil || got != tt.want {
				t.Fatalf("Sum(%v) = (%d, %v), want (%d, nil)", tt.numbers, got, err, tt.want)
			}
		})
	}
}

func assertMathError(t *testing.T, err, cause error, operation string, a, b int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a math error")
	}
	// Inspect through an extra application wrapper, not just MathError itself.
	wrapped := fmt.Errorf("calculation: %w", err)
	if !errors.Is(wrapped, cause) {
		t.Fatalf("error %v does not preserve cause %v", wrapped, cause)
	}
	var detail *MathError
	if !errors.As(wrapped, &detail) {
		t.Fatalf("error %v does not expose *MathError", wrapped)
	}
	if detail.Operation != operation || detail.InputA != a || detail.InputB != b {
		t.Errorf("error details = %+v, want %s(%d, %d)", detail, operation, a, b)
	}
}

func TestSumRejectsIntermediateOverflow(t *testing.T) {
	tests := []struct {
		name    string
		numbers []int
		a, b    int
	}{
		{"above maximum", []int{math.MaxInt, 1}, math.MaxInt, 1},
		{"below minimum", []int{math.MinInt, -1}, math.MinInt, -1},
		{"two large positives", []int{math.MaxInt, math.MaxInt}, math.MaxInt, math.MaxInt},
		{"two large negatives", []int{math.MinInt, math.MinInt}, math.MinInt, math.MinInt},
		{"maximum as next operand", []int{1, math.MaxInt}, 1, math.MaxInt},
		{"minimum as next operand", []int{-1, math.MinInt}, -1, math.MinInt},
		{"later positive overflow", []int{1, math.MaxInt - 1, 1}, math.MaxInt, 1},
		{"later negative overflow", []int{-1, math.MinInt + 1, -1}, math.MinInt, -1},
		{"later cancellation cannot undo overflow", []int{math.MaxInt, 1, -1}, math.MaxInt, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Sum(tt.numbers...)
			if got != 0 {
				t.Errorf("failed sum returned %d; want zero, not a partial result", got)
			}
			assertMathError(t, err, ErrOverflow, "sum", tt.a, tt.b)
			if errors.Is(err, ErrDivisionByZero) {
				t.Error("overflow incorrectly matches division by zero")
			}
		})
	}
}

func TestSafeDivision(t *testing.T) {
	tests := []struct {
		name       string
		a, b, want int
	}{
		{"exact", 10, 2, 5},
		{"truncate positive", 7, 2, 3},
		{"truncate negative dividend", -7, 2, -3},
		{"truncate negative divisor", 7, -2, -3},
		{"both negative", -7, -2, 3},
		{"zero is a valid result", 0, 5, 0},
		{"magnitude below divisor", 1, 2, 0},
		{"maximum", math.MaxInt, 1, math.MaxInt},
		{"minimum", math.MinInt, 1, math.MinInt},
		{"negative boundary", math.MinInt, 2, math.MinInt / 2},
		{"minimum divisor", math.MinInt, math.MinInt, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeDivision(tt.a, tt.b)
			if err != nil || got != tt.want {
				t.Fatalf("SafeDivision(%d, %d) = (%d, %v), want (%d, nil)", tt.a, tt.b, got, err, tt.want)
			}
		})
	}
}

func TestSafeDivisionErrors(t *testing.T) {
	tests := []struct {
		name  string
		a, b  int
		cause error
	}{
		{"positive divided by zero", 10, 0, ErrDivisionByZero},
		{"zero divided by zero", 0, 0, ErrDivisionByZero},
		{"negative divided by zero", math.MinInt, 0, ErrDivisionByZero},
		{"unrepresentable quotient", math.MinInt, -1, ErrOverflow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeDivision(tt.a, tt.b)
			if got != 0 {
				t.Errorf("failed division returned %d; want zero", got)
			}
			assertMathError(t, err, tt.cause, "division", tt.a, tt.b)
			otherCause := ErrOverflow
			if tt.cause == ErrOverflow {
				otherCause = ErrDivisionByZero
			}
			if errors.Is(err, otherCause) {
				t.Errorf("error %v incorrectly matches %v", err, otherCause)
			}
		})
	}
}

func Example_errorInspection() {
	_, err := SafeDivision(10, 0)
	wrapped := fmt.Errorf("calculate ratio: %w", err)
	fmt.Println(wrapped)
	fmt.Println("division by zero:", errors.Is(wrapped, ErrDivisionByZero))
	var detail *MathError
	if errors.As(wrapped, &detail) {
		fmt.Println(detail.Operation, detail.InputA, detail.InputB)
	}
	// Output:
	// calculate ratio: division(10, 0): division by zero
	// division by zero: true
	// division 10 0
}
