// Values
// Topic: Go programs work with different basic values: strings, numbers, booleans, slices, and nil.
// Args/Input: this example does not receive input; it prints literal values directly.
// Usage: use fmt.Println/Printf to inspect values and understand how each type is displayed.
// Key idea: every value has a type, and printing helps reveal how Go represents it.
package main

import "fmt"

func main() {

	// String value
	fmt.Printf("Hello World\n")

	// Integer and float operations
	fmt.Println(2 * 2)

	fmt.Println(3.1416 * 2)

	// Boolean values
	fmt.Println(true, false)

	// Slice literal printed with field/value format
	fmt.Printf("%+v\n", []int{1, 2, 3})

	// any has nil as its zero value
	var t any
	fmt.Printf("%+v\n", t)
}
