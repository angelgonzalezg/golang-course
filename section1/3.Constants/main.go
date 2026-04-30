// Constants
// Topic: constants are values that cannot change while the program runs.
// Args/Input: this file prints fixed typed constants: strings and numbers.
// Usage: use const for stable values like hosts, app names, rates, and math values.
// Key idea: constants are immutable and are evaluated at compile time.
package main

import "fmt"

const HOST string = "localhost" // Package-level constant

func main() {

	fmt.Println(HOST)

	const AppName string = "Go" // Local constant
	fmt.Println(AppName)

	// Numeric constants can use different number types.
	const pi float64 = 3.1416
	fmt.Println(pi)

	const rate float32 = 5.5
	fmt.Println(rate)

}
