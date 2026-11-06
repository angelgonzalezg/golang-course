// Module Versions
// Topic: require selects dependency versions; replace supplies local source code.
// Usage from this module: go run .
// Key idea: incompatible major versions use distinct import paths and can coexist.
package main

import (
	"errors"
	"fmt"

	// Both packages declare "package greeting". Aliases resolve the name collision
	// in this file; they do not change either package's import path or module version.
	greetingv1 "example.com/course/greeting"
	greetingv2 "example.com/course/greeting/v2"
)

func main() {
	fmt.Println("== 1. Version 1 returns a string ==")
	fmt.Println(greetingv1.Hello("Ana"))
	fmt.Println("blank name:", greetingv1.Hello(""))

	fmt.Println("\n== 2. Version 2 returns a string and an error ==")
	for _, name := range []string{"Ana", ""} {
		message, err := greetingv2.Hello(name)
		if err != nil {
			fmt.Println("blank name rejected:", errors.Is(err, greetingv2.ErrBlankName))
			continue
		}
		fmt.Println(message)
	}
}
