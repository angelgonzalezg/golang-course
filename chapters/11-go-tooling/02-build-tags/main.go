// Build Tags
// Topic: complementary constraints select one implementation at compile time.
// Usage: go run [-tags=diagnostic] ./chapters/11-go-tooling/02-build-tags
package main

import "fmt"

func main() {
	// Both selected implementations provide the same function. Changing the mode
	// requires another build; it is not a flag that the running program can toggle.
	fmt.Println("build mode:", buildMode())
}
