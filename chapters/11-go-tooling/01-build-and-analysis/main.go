// Build and Analysis
// Topic: run and build a complete package, inspect its target, and set build metadata.
// Usage: go run ./chapters/11-go-tooling/01-build-and-analysis
package main

import "fmt"

// A string variable can be set by the linker with -ldflags='-X main.version=...'.
// Keep a useful default so an ordinary go run or go build needs no extra flags.
var version = "development"

func main() {
	fmt.Println("Go tooling demo")
	fmt.Println("version:", version)
	// targetDescription lives in target.go. Build the package, rather than listing
	// only main.go, to include all source files selected for this build.
	fmt.Println(targetDescription())
}
