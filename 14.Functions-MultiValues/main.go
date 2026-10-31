// Functions with Multiple Return Values
// Topic: Go functions can return more than one value.
// Args/Input: examples use division, user lookup, and name splitting.
// Usage: return result + error, value + ok, or related values together.
// Key idea: multiple returns make success, failure, and extra data explicit.
package main

import (
	"errors"
	"fmt"
	"strings"
)

/* Helper section 1: Notes about return names */
/*
Named vs non-named return values:
- Use non-named returns when the types already make the results clear.
- Use named returns when the names make the result easier to understand.
- Named results can still use explicit return expressions.
- Prefer explicit returns so the reader can see which values leave the function.
*/

/* Helper section 2: Multiple return helper functions */
/* Return result + error */
// Non-named returns are clear here: result first, error second.
func divide(a, b float64) (float64, error) {
	if b == 0 {
		// Return a zero result plus an error when the operation is invalid.
		return 0, errors.New("cannot divide by zero")
	}

	// nil means there was no error.
	return a / b, nil
}

/* Return value + ok */
// Non-named returns are also clear for the common value + ok pattern.
func findUser(id int) (string, bool) {
	users := map[int]string{
		1: "Ana",
		2: "Ben",
	}

	// Map lookups also use the value + ok pattern.
	name, ok := users[id]
	return name, ok
}

/* Return two related values */
// This could use named returns, but explicit returns keep the logic easy to follow.
func minMax(numbers []int) (int, int, bool) {
	if len(numbers) == 0 {
		// false tells the caller that min and max are not useful.
		return 0, 0, false
	}

	min := numbers[0]
	max := numbers[0]
	for _, number := range numbers {
		if number < min {
			min = number
		}
		if number > max {
			max = number
		}
	}

	return min, max, true
}

/* Named return values */
// firstName and lastName are declared in the function signature.
// Names help because (string, string) alone would not say which value is which.
// This example treats the first word as firstName and the remaining words as lastName.
// Personal naming conventions vary; this is a simplified text-splitting example.
func splitName(fullName string) (firstName, lastName string) {
	parts := strings.Fields(fullName)
	if len(parts) == 0 {
		return "", ""
	}

	firstName = parts[0]
	if len(parts) > 1 {
		lastName = strings.Join(parts[1:], " ")
	}

	return firstName, lastName
}

func main() {
	/* Section 1: Return result + error */
	fmt.Println("== 1. Return result + error ==")

	result, err := divide(10, 2)
	// Always check the error before using the result.
	if err != nil {
		fmt.Println("Divide error:", err)
	} else {
		fmt.Println("Divide result:", result)
	}

	/* Section 2: Ignore one returned value with _ */
	fmt.Println("\n== 2. Ignore one returned value with _ ==")

	_, err = divide(10, 0) // Use _ when you do not need a returned value.
	// Here we only care if an error happened.
	if err != nil {
		fmt.Println("Divide error:", err)
	}

	/* Section 3: Return value + ok */
	fmt.Println("\n== 3. Return value + ok ==")

	// ok makes the success/failure result explicit.
	if name, ok := findUser(1); ok {
		fmt.Println("Found user:", name)
	} else {
		fmt.Println("User not found.")
	}

	if name, ok := findUser(99); ok {
		fmt.Println("Found user:", name)
	} else {
		fmt.Println("User 99 was not found; the returned name is not a result.")
	}

	/* Section 4: Return multiple related values */
	fmt.Println("\n== 4. Return multiple related values ==")

	// Three return values can still be handled clearly.
	if min, max, ok := minMax([]int{8, 3, 10, 5}); ok {
		fmt.Printf("Min: %d, Max: %d\n", min, max)
	}

	// An empty slice has no minimum or maximum; check ok before using either value.
	if min, max, ok := minMax(nil); ok {
		fmt.Printf("Min: %d, Max: %d\n", min, max)
	} else {
		fmt.Println("An empty slice has no minimum or maximum.")
	}

	/* Section 5: Named return values */
	fmt.Println("\n== 5. Named return values ==")

	firstName, lastName := splitName("Angel Gonzalez")
	fmt.Println("First name:", firstName)
	fmt.Println("Last name:", lastName)

	// Keep all remaining words instead of silently discarding part of the input.
	firstName, lastName = splitName("Ana Garcia Lopez")
	fmt.Printf("Multiple words: first=%q last=%q\n", firstName, lastName)

	firstName, lastName = splitName("Ada")
	fmt.Printf("One word: first=%q last=%q\n", firstName, lastName)

	firstName, lastName = splitName("   ")
	fmt.Printf("Empty input: first=%q last=%q\n", firstName, lastName)

	/* Section 6: Final multiple return rules */
	fmt.Println("\n== 6. Final multiple return rules ==")

	// Use (value, error) when an operation can fail.
	// Use (value, ok) when absence is normal and not an error.
	// Keep the order of returned values predictable and easy to read.
	fmt.Println("Multiple returns make success, failure, and extra data explicit.")
}
