// Conditionals
// Topic: if, else if, and else control which code runs based on boolean expressions.
// Args/Input: examples check age, score, and map access values.
// Usage: use conditionals for validation, grading, permissions, and decision logic.
// Key idea: conditions must evaluate to bool and decide program flow.
package main

import "fmt"

func main() {
	/* if-else: two possible branches */
	age := 17
	if age >= 18 {
		fmt.Println("Es Mayor de edad!")
	} else {
		fmt.Println("Es Menor de edad!")
	}

	/* else-if chain: multiple possible branches */
	score := 85
	if score >= 90 {
		fmt.Println("Grado A")
	} else if score >= 80 {
		fmt.Println("Grado B")
	} else if score >= 70 {
		fmt.Println("Grado C")
	} else if score >= 60 {
		fmt.Println("Grado D")
	} else {
		fmt.Println("Reprobado")
	}

	/* if with map lookup: value + ok flag */
	userAccess := map[string]bool{
		"jane": true,
		"john": false,
		"xavi": true,
	}

	if hasAccess, ok := userAccess["jane"]; ok && hasAccess { // ok confirms key exists
		fmt.Println("Access the system!")
	} else {
		fmt.Println("Access not granted.")
	}
}
