// Variable Shadowing
// Topic: shadowing happens when a new variable uses the same name as an outer variable.
// Args/Input: examples use strings, ints, if blocks, short declarations, and package variables.
// Usage: understand when := creates a new variable instead of updating an existing one.
// Key idea: the inner variable hides the outer variable only inside its scope.
package main

import "fmt"

/* Helper section 1: Package value used for shadowing */
var appName = "Golang Course"

/* Helper section 2: Function used by if shadowing example */
func findUser() (string, error) {
	return "Alice", nil
}

func main() {
	/* Section 1: Basic shadowing */
	fmt.Println("== 1. Basic shadowing ==")

	/* Basic shadowing */
	name := "outer name"
	fmt.Printf("before block: %s\n", name)

	{
		// This creates a new name only inside this block.
		name := "inner name"
		fmt.Printf("inside block: %s\n", name)
	}

	fmt.Printf("after block: %s\n", name)

	/* Section 2: Shadowing in if statements */
	fmt.Println("\n== 2. Shadowing in if statements ==")

	/* Shadowing in if statements */
	user := "Guest"
	fmt.Printf("before if user: %s\n", user)
	// This user hides the outer user; err exists only inside the if and else blocks.
	if user, err := findUser(); err == nil {
		fmt.Printf("found user: %s\n", user)
	} else {
		fmt.Printf("error: %v\n", err)
	}
	fmt.Printf("after if user: %s\n", user) // Still Guest.

	/* Section 3: Short declaration in the same scope */
	fmt.Println("\n== 3. Short declaration in the same scope ==")

	/* Short declaration in the same scope */
	score := 90
	// := can reuse score here because penalty is a new variable.
	score, penalty := 85, 5
	fmt.Printf("score=%d penalty=%d\n", score, penalty)

	// This would not compile because no new variable is created:
	// score := 100

	/* Section 4: Common mistake */
	fmt.Println("\n== 4. Common mistake ==")

	/* Common mistake: accidental shadowing */
	total := 100
	if total > 50 {
		// This creates a new total inside the if block.
		total := total - 20
		fmt.Printf("inside if total: %d\n", total)
	}
	fmt.Printf("outside if total: %d\n", total)

	/* Section 5: Fix with assignment */
	fmt.Println("\n== 5. Fix with assignment ==")

	/* Fix: use = to update the existing variable */
	total = 100
	if total > 50 {
		total = total - 20
	}
	fmt.Printf("updated total: %d\n", total)

	/* Section 6: Shadowing a package variable */
	fmt.Println("\n== 6. Shadowing a package variable ==")

	/* Shadowing a package variable */
	// Local appName hides the package-level appName inside main.
	appName := "Shadowing Lesson"
	fmt.Printf("local appName: %s\n", appName)

	/* Section 7: Avoiding confusion */
	fmt.Println("\n== 7. Avoiding confusion ==")

	/* Avoiding confusion */
	// Use a different name when both values matter.
	outerStatus := "active"
	{
		innerStatus := "paused"
		fmt.Printf("outer status: %s\n", outerStatus)
		fmt.Printf("inner status: %s\n", innerStatus)
	}

	/* Section 8: Final shadowing rules */
	fmt.Println("\n== 8. Final shadowing rules ==")

	// In the same block, := requires at least one new non-blank name.
	// In an inner block, := can declare a new variable that hides an outer one.
	// = updates an existing variable.
	// Shadowing is allowed, but it should make code clearer, not surprising.
	fmt.Println("Shadowing hides an outer variable only inside the inner scope.")
}
