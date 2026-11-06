// Packages and Imports
// Topic: a command uses a library package, its exported API, and an internal helper.
// Usage: go run ./chapters/10-modules-and-packages/01-packages-and-imports
// Key idea: a module groups packages; package boundaries define the visible API.
package main

import (
	"errors"
	"fmt"

	// Import path = the root go.mod's module path + the package's directory path.
	"golangcourse/chapters/10-modules-and-packages/01-packages-and-imports/greeting"
)

func main() {
	fmt.Println("== 1. Use an exported type and method ==")
	var english greeting.Greeter
	message, err := english.Greet("  Ana\t María  ")
	if err != nil {
		fmt.Println("greet:", err)
		return
	}
	fmt.Println(message)

	fmt.Println("\n== 2. Initialize through the package API ==")
	spanish, err := greeting.New(greeting.Spanish)
	if err != nil {
		fmt.Println("configure greeting:", err)
		return
	}
	// spanish.prefix = "Hi" // Does not compile: the field belongs to package greeting.
	// greeting.prefixFor(greeting.English) // Does not compile: the function is unexported.
	for _, name := range []string{"  Luis  ", "\t"} {
		message, err := spanish.Greet(name)
		if err != nil {
			fmt.Println("blank name:", errors.Is(err, greeting.ErrBlankName))
			continue
		}
		fmt.Println(message)
	}

	fmt.Println("\n== 3. Inspect an exported error across a package boundary ==")
	_, err = greeting.New(greeting.Language(99))
	fmt.Println("unsupported language:", errors.Is(err, greeting.ErrUnsupportedLanguage))
	// Greet calls internal/textutil.CleanName. Its uppercase name permits access
	// from greeting; internal prevents imports from outside this lesson's subtree.
}
