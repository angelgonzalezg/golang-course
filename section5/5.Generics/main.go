// Generics
// Topic: generics let functions work with different types while staying type-safe.
// Args/Input: Sum receives variadic numbers of a generic type T.
// Usage: use a type parameter like T when the same logic should work for multiple types.
// Key idea: T is a placeholder type chosen at compile time and limited by a constraint.
package main

import "fmt"

// Number is a type constraint for T.
// It restricts T to int, float32, or float64 so + is allowed safely.
type Number interface {
	int | float32 | float64
}

// Sum is a generic function
// [T Number] means:
// - T is a type parameter (placeholder type)
// - T must satisfy the Number constraint
// - numbers ...T and the return value all use that same chosen type
func Sum[T Number](numbers ...T) T {

	// total has type T, so it matches the provided numeric type.
	var total T

	// numbers is a variadic parameter (...T)
	// This allows passing multiple values of type T
	for _, number := range numbers {
		total += number
	}

	return total
}

func main() {

	// Go infers T from the provided values.
	// Mixed numeric constants here infer float64.
	v := Sum(10, 20, 3.3)

	// %T prints the type of v
	// This helps demonstrate generics behavior
	fmt.Printf("%T\n", v)

	// Output:
	// float64
}
