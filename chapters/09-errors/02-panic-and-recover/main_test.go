package main

import (
	"errors"
	"fmt"
	"testing"
)

func TestSafeDivide(t *testing.T) {
	tests := []struct {
		name       string
		a, b, want int
		wantError  bool
	}{
		{"success", 10, 2, 5, false},
		{"integer truncation", -7, 2, -3, false},
		{"valid zero", 0, 5, 0, false},
		{"recovered panic", 10, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := safeDivide(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("result = %d, want %d", got, tt.want)
			}
			if tt.wantError {
				if !errors.Is(err, ErrRecoveredPanic) {
					t.Fatalf("recovered error = %v, want ErrRecoveredPanic", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func Example_directRecovery() {
	recoverOutsideDeferDoesNothing()
	recoverPanic()
	fmt.Println("caller continues")
	// Output:
	// recover outside defer: <nil>
	// recovered value: panic handled by recoverPanic
	// caller continues
}

func Example_indirectRecovery() {
	protectBoundary("outer", indirectRecoverDoesNothing)
	fmt.Println("caller continues")
	// Output:
	// recover inside a helper: <nil>
	// outer recovered: indirect recover did not stop the panic
	// caller continues
}

func Example_cleanupDuringPanic() {
	protectBoundary("outer", func() {
		panicWithCleanup()
		fmt.Println("this line is skipped")
	})
	fmt.Println("caller continues")
	// Output:
	// starting work
	// cleanup still runs before the panic leaves the function
	// outer recovered: something broke
	// caller continues
}

func Example_repanic() {
	protectBoundary("outer", recoverAndRepanic)
	// Output:
	// logging before re-panic: this panic is not handled here
	// outer recovered: this panic is not handled here
}
