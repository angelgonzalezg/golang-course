// Conditionals
// Topic: if, else if, else, logical operators, and short statements.
// Args/Input: examples use age, score, login state, numbers, and map lookups.
// Usage: use if statements for validation, choices, permissions, and decisions.
// Key idea: every if condition must be true or false.
package main

import "fmt"

func main() {
	/* Section 1: Basic if and if-else */
	fmt.Println("== 1. Basic if and if-else ==")

	/* Basic if: run code only when the condition is true */
	// Go does not need parentheses around the condition.
	temperature := 31
	if temperature > 30 {
		fmt.Println("It is a hot day.")
	}

	/* if-else: choose between two paths */
	age := 18
	if age >= 18 {
		fmt.Println("You are an adult.")
	} else {
		fmt.Println("You are under 18.")
	}

	/* Section 2: Comparison operators */
	fmt.Println("\n== 2. Comparison operators ==")

	/* Comparison operators: ==, !=, >, <, >=, <= */
	wallet := 20
	price := 15
	if wallet >= price {
		fmt.Println("You can buy the item.")
	}
	if wallet != price {
		fmt.Println("Your wallet amount is different from the price.")
	}

	/* Section 3: else-if chains */
	fmt.Println("\n== 3. else-if chains ==")

	/* else-if chain: choose one result from many options */
	score := 85
	if score >= 90 {
		fmt.Println("Grade: A")
	} else if score >= 80 {
		fmt.Println("Grade: B")
	} else if score >= 70 {
		fmt.Println("Grade: C")
	} else if score >= 60 {
		fmt.Println("Grade: D")
	} else {
		fmt.Println("Grade: F")
	}

	/* Section 4: Logical operators */
	fmt.Println("\n== 4. Logical operators ==")

	/* && means both conditions must be true */
	hasID := true
	if age >= 18 && hasID {
		fmt.Println("You can enter the event.")
	}

	/* || means at least one condition must be true */
	hasTicket := false
	if hasTicket || age < 12 {
		fmt.Println("You can ride the bus for free.")
	} else {
		fmt.Println("Please buy a bus ticket.")
	}

	/* ! flips true to false, or false to true */
	isLoggedIn := false
	if !isLoggedIn {
		fmt.Println("Please log in first.")
	}

	/* Section 5: Boolean variables */
	fmt.Println("\n== 5. Boolean variables ==")

	/* Boolean variables can be used directly */
	isWeekend := true
	if isWeekend {
		fmt.Println("Time to rest.")
	}

	/* Section 6: if with a short statement */
	fmt.Println("\n== 6. if with a short statement ==")

	/* if with a short statement (scoped variable): value exists only in this if */
	number := 10
	if remainder := number % 2; remainder == 0 {
		fmt.Println("The number is even.")
	} else {
		fmt.Println("The number is odd.")
	}
	// remainder cannot be used here because it only exists inside the if.
	// fmt.Printf("remainder is %d", remainder)

	/* Section 7: if with map lookup */
	fmt.Println("\n== 7. if with map lookup ==")

	/* if with map lookup: ok tells us if the key exists */
	userAccess := map[string]bool{
		"jane":  true,
		"john":  false,
		"sofia": true,
	}

	if hasAccess, ok := userAccess["jane"]; ok && hasAccess {
		fmt.Println("Jane can access the system.")
	} else {
		fmt.Println("Jane cannot access the system.")
	}

	if hasAccess, ok := userAccess["john"]; !ok {
		fmt.Println("John was not found.")
	} else if hasAccess {
		fmt.Println("John can access the system.")
	} else {
		fmt.Println("John exists, but access is disabled.")
	}

	if hasAccess, ok := userAccess["maria"]; !ok {
		fmt.Println("Maria was not found.")
	} else if hasAccess {
		fmt.Println("Maria can access the system.")
	} else {
		fmt.Println("Maria exists, but access is disabled.")
	}

	/* Section 8: Nested if */
	fmt.Println("\n== 8. Nested if ==")

	/* Nested if: useful when one decision depends on another */
	balance := 100
	withdrawAmount := 40
	if withdrawAmount > 0 {
		if balance >= withdrawAmount {
			fmt.Println("Withdrawal approved.")
		} else {
			fmt.Println("Not enough balance.")
		}
	} else {
		fmt.Println("Withdrawal amount must be positive.")
	}

	/* Section 9: Conditions must be booleans */
	fmt.Println("\n== 9. Conditions must be booleans ==")

	/* Go does not use numbers as conditions */
	// Use a comparison like > 0 instead.
	messageCount := 3
	if messageCount > 0 {
		fmt.Println("You have new messages.")
	}

	/* Section 10: Final conditional rules */
	fmt.Println("\n== 10. Final conditional rules ==")

	// Use if for validation, permissions, and choices.
	// Keep conditions readable, especially when using && and ||.
	// Use short statements when the temporary value is only needed by the if.
	fmt.Println("Conditionals let your program choose what to do next.")
}
