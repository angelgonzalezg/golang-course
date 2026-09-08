// Switch Statements
// Topic: switch selects one branch from multiple cases.
// Args/Input: examples use days, menu options, scores, strings, and value types.
// Usage: use switch when checking one value or several related conditions.
// Key idea: cases do not fall through by default; only the first matching case runs.
package main

import "fmt"

func main() {
	/* Section 1: Basic value switch */
	fmt.Println("== 1. Basic value switch ==")

	/* Basic value switch */
	// Go does not need parentheses around the switch value.
	day := "Sunday"
	switch day {
	case "Saturday":
		fmt.Println("Start of the weekend.")
	case "Sunday":
		fmt.Println("End of the weekend.")
	default:
		fmt.Println("Regular weekday.")
	}

	/* Section 2: Multiple values and empty cases */
	fmt.Println("\n== 2. Multiple values and empty cases ==")

	/* Multiple values in one case */
	// The same case runs when trafficLight is "yellow" or "orange".
	trafficLight := "orange"
	switch trafficLight {
	case "green":
		fmt.Println("Go.")
	case "yellow", "orange":
		fmt.Println("Slow down.")
	case "red":
		fmt.Println("Stop.")
	default:
		fmt.Println("Unknown traffic light color.")
	}

	/* Empty case: match a value, but do nothing */
	notification := "none"
	switch notification {
	case "none":
		// No notification is needed, so this case stays empty.
	case "email":
		fmt.Println("Send an email notification.")
	case "sms":
		fmt.Println("Send a text message.")
	default:
		fmt.Println("Unknown notification type.")
	}
	fmt.Println("Notification check complete.")

	/* Section 3: Switch with numbers */
	fmt.Println("\n== 3. Switch with numbers ==")

	/* Switch with numbers */
	// After one case runs, the switch ends automatically.
	// You do not need break like in some other languages.
	menuOption := 2
	switch menuOption {
	case 1:
		fmt.Println("Create a new file.")
	case 2:
		fmt.Println("Open an existing file.")
	case 3:
		fmt.Println("Save the file.")
	default:
		fmt.Println("Please choose a valid menu option.")
	}

	/* Section 4: Switch with a short statement */
	fmt.Println("\n== 4. Switch with a short statement ==")

	/* Switch with a short statement */
	// userName only exists inside this switch.
	switch userName := "Ana"; userName {
	case "Ana":
		fmt.Println("Welcome back, Ana.")
	case "Ben":
		fmt.Println("Welcome back, Ben.")
	default:
		fmt.Println("Welcome, guest.")
	}
	// userName cannot be used here because it only exists inside the switch.
	// fmt.Println(userName)

	/* Section 5: Expressionless switch */
	fmt.Println("\n== 5. Expressionless switch ==")

	/* Expressionless switch: useful for conditions */
	// The first true case wins.
	// Put the most specific or highest checks first.
	score := 85
	switch {
	case score >= 90:
		fmt.Println("Score grade: A")
	case score >= 80:
		fmt.Println("Score grade: B")
	case score >= 70:
		fmt.Println("Score grade: C")
	case score >= 60:
		fmt.Println("Score grade: D")
	default:
		fmt.Println("Score grade: F")
	}

	/* Section 6: fallthrough */
	fmt.Println("\n== 6. fallthrough ==")

	/* fallthrough: run the next case too */
	// fallthrough is rarely needed, but Go supports it explicitly.
	// It runs the next case without checking that case condition.
	step := 1
	switch step {
	case 1:
		fmt.Println("Step 1: choose a plan.")
		fallthrough
	case 2:
		fmt.Println("Step 2: enter payment details.")
	case 3:
		fmt.Println("Step 3: confirm purchase.")
	default:
		fmt.Println("Unknown step.")
	}

	/* Section 7: Type switch */
	fmt.Println("\n== 7. Type switch ==")

	/* Type switch: choose based on the value's type */
	// Change this value to try the other cases.
	var value any = "Go"
	switch v := value.(type) {
	case int:
		fmt.Printf("Integer value: %d\n", v)
	case string:
		fmt.Printf("String value: %s\n", v)
	case bool:
		fmt.Printf("Boolean value: %t\n", v)
	case nil:
		fmt.Println("Nil value.")
	default:
		fmt.Printf("Unknown type: %T\n", v)
	}

	/* Section 8: Switch inside a loop */
	fmt.Println("\n== 8. Switch inside a loop ==")

	/* For loop with switch and labels */
	commands := []string{"start", "skip", "continue", "stop", "restart"}

CommandLoop:
	for _, command := range commands {
		switch command {
		case "skip":
			fmt.Println("Skipping this command.")
			continue CommandLoop
		case "stop":
			fmt.Println("Stopping command loop.")
			break CommandLoop
		default:
			fmt.Println("Running command:", command)
		}
	}

	/* Section 9: Final switch rules */
	fmt.Println("\n== 9. Final switch rules ==")

	// Use switch when one value has several possible cases.
	// Use expressionless switch when the cases are conditions.
	// Go switches stop automatically unless fallthrough is used.
	fmt.Println("switch keeps related choices readable.")
}
