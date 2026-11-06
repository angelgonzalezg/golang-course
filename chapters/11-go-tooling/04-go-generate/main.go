// Go Generate
// Topic: transform a maintained input into deterministic, formatted Go source.
// Usage: go generate ./chapters/11-go-tooling/04-go-generate, then go run that package.
package main

import "fmt"

func main() {
	for value := range statusNames {
		fmt.Printf("%d -> %s\n", value, Status(value))
	}
	fmt.Println("unknown:", Status(-1))
}
