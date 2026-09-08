// For Loops
// Topic: Go uses only the for keyword for loop logic.
// Args/Input: examples use counters, totals, slices, strings, maps, and nested loops.
// Usage: for can work as a counter loop, while-style loop, infinite loop, and range loop.
// Key idea: Go has one loop keyword, but it covers several loop patterns.
package main

import "fmt"

func main() {
	/* Section 1: Counter loops */
	fmt.Println("== 1. Counter loops ==")

	/* Classic C-style for: init; condition; post */
	for i := 1; i <= 5; i++ {
		fmt.Println("Count:", i)
	}
	// i cannot be used here because it only exists inside the for loop.
	// fmt.Println(i)

	/* Count backwards */
	for countdown := 3; countdown >= 1; countdown-- {
		fmt.Println("Countdown:", countdown)
	}
	fmt.Println("Go!")

	/* Add numbers with a loop */
	sum := 0
	for i := 1; i <= 5; i++ {
		sum += i
	}
	fmt.Println("Sum from 1 to 5:", sum)

	/* Section 2: Loop over string bytes */
	fmt.Println("\n== 2. Loop over string bytes ==")

	/* Loop over a string as bytes */
	// byte is useful for raw data, but one visible character can use many bytes.
	word1 := "Hello ☀️" // sun emoji uses 4 bytes
	for index := 0; index < len(word1); index++ {
		letter := word1[index]
		fmt.Printf("word[%d] byte = %d (%c)\n", index, letter, letter)
	}

	/* Section 3: While-style and infinite loops */
	fmt.Println("\n== 3. While-style and infinite loops ==")

	/* While-style loop: only a condition */
	// Go does not have a while keyword. Use for instead.
	energy := 3
	for energy > 0 {
		fmt.Println("Running, energy left:", energy)
		energy--
	}

	/* Infinite loop: use break to exit */
	counter := 0
	for {
		if counter >= 3 {
			break
		}
		fmt.Println("Infinite loop counter:", counter)
		counter++
	}

	/* Section 4: continue and break */
	fmt.Println("\n== 4. continue and break ==")

	/* continue: skip the rest of this loop turn */
	for number := 1; number <= 5; number++ {
		if number == 3 {
			continue
		}
		fmt.Println("Not skipped:", number)
	}

	/* break: stop the loop early */
	for number := 1; number <= 5; number++ {
		if number == 4 {
			break
		}
		fmt.Println("Before break:", number)
	}

	/* Section 5: Range over slices */
	fmt.Println("\n== 5. Range over slices ==")

	/* Range over a slice: get index and value */
	numbers := []int{10, 20, 30}
	for index, value := range numbers {
		fmt.Printf("numbers[%d] = %d\n", index, value)
	}

	/* Ignore the index with _ */
	for _, value := range numbers {
		fmt.Println("Slice value:", value)
	}

	/* Use only the index when you need positions */
	names := []string{"Ana", "Ben", "Cara"}
	for index := range names {
		fmt.Printf("Name %d is %s\n", index, names[index])
	}

	/* Update slice values by using the index */
	scores := []int{70, 80, 90}
	for index := range scores {
		scores[index] += 5
	}
	fmt.Println("Updated scores:", scores)

	/* Section 6: Range over strings and maps */
	fmt.Println("\n== 6. Range over strings and maps ==")

	/* Range over a string: value is a rune */
	word2 := "Go 🦫"
	for index, letter := range word2 {
		fmt.Printf("word[%d] = %c\n", index, letter)
	}

	/* Range over a map: order is not guaranteed */
	stock := map[string]int{
		"apples":  5,
		"bananas": 2,
		"oranges": 4,
	}
	for fruit, quantity := range stock {
		fmt.Printf("%s: %d\n", fruit, quantity)
	}

	/* Section 7: Nested loops */
	fmt.Println("\n== 7. Nested loops ==")

	/* Nested loops: one loop inside another */
	for row := 1; row <= 3; row++ {
		for column := 1; column <= 3; column++ {
			fmt.Printf("(%d,%d) ", row, column)
		}
		fmt.Println()
	}

	/* Section 8: Loop over a fixed number of attempts */
	fmt.Println("\n== 8. Loop over a fixed number of attempts ==")

	/* Loop over a fixed number of attempts */
	password := "go123"
	guess := "go123"
	for attempt := 1; attempt <= 3; attempt++ {
		if guess == password {
			fmt.Println("Login successful.")
			break
		}
		fmt.Println("Wrong password.")
	}

	/* Section 9: Labeled loops */
	fmt.Println("\n== 9. Labeled loops ==")

	/* Labeled loop: break out of an outer loop */
	targetRow := 2
	targetColumn := 3

SearchLoop:
	for row := 1; row <= 3; row++ {
		for column := 1; column <= 3; column++ {
			fmt.Printf("Checking (%d,%d)\n", row, column)
			if row == targetRow && column == targetColumn {
				fmt.Println("Target found.")
				break SearchLoop
			}
		}
	}

	/* Labeled loop: continue the outer loop */
	blockedRow := 2
	blockedColumn := 2

SeatLoop:
	for row := 1; row <= 3; row++ {
		for column := 1; column <= 3; column++ {
			if row == blockedRow && column == blockedColumn {
				fmt.Printf("Skipping the rest of row %d\n", row)
				continue SeatLoop
			}
			fmt.Printf("Seat (%d,%d) is available\n", row, column)
		}
	}

	/* Section 10: Final for loop rules */
	fmt.Println("\n== 10. Final for loop rules ==")

	// Go uses for for every loop style.
	// Use range for collections, strings, and maps.
	// Use labels only when nested loops need a clear exit or skip.
	fmt.Println("for is Go's only loop keyword, but it covers many patterns.")
}
