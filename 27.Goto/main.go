// Goto
// Topic: goto jumps to a labeled statement in the same function.
// Args/Input: example simulates setup steps that need one shared cleanup path.
// Usage: goto is rare in Go; prefer defer, return, break, and continue most of the time.
// Key idea: goto can be useful when several failure points should jump to one cleanup block.
package main

import "fmt"

/* Helper section 1: Goto examples */
func jumpForward() {
	fmt.Println("before goto")
	goto done

	fmt.Println("this line is skipped")

done:
	fmt.Println("after label")
}

func retryWithGoto() {
	attempt := 0

retry:
	attempt++
	fmt.Println("attempt:", attempt)

	if attempt < 3 {
		goto retry
	}

	fmt.Println("retry finished")
}

/* Helper section 2: Shared cleanup example */
func sharedCleanup() {
	/* Best simple use case: one shared cleanup section */
	// This avoids repeating the same cleanup code after every possible failure.
	connected := false
	fileOpen := false
	recordLocked := false
	workDone := false

	fmt.Println("Starting work.")

	connected = true
	fmt.Println("Connected to service.")

	fileOpen = true
	fmt.Println("Opened file.")

	recordLocked = true
	fmt.Println("Locked record.")

	userHasPermission := false
	if !userHasPermission {
		fmt.Println("Permission denied.")
		goto cleanup
	}

	fmt.Println("Doing important work.")
	workDone = true

cleanup:
	// This cleanup runs whether the work succeeds or fails.
	if recordLocked {
		fmt.Println("Unlocked record.")
	}
	if fileOpen {
		fmt.Println("Closed file.")
	}
	if connected {
		fmt.Println("Disconnected from service.")
	}

	if workDone {
		fmt.Println("Work finished successfully.")
	} else {
		fmt.Println("Work did not finish.")
	}
}

func main() {
	/* Section 1: Basic goto */
	fmt.Println("== 1. Basic goto ==")

	// goto jumps to a label in the same function.
	jumpForward()

	/* Section 2: Goto can jump backward */
	fmt.Println("\n== 2. Goto can jump backward ==")

	// This works, but a for loop is usually clearer for retries.
	retryWithGoto()

	/* Section 3: Shared cleanup path */
	fmt.Println("\n== 3. Shared cleanup path ==")

	sharedCleanup()

	/* Section 4: Goto rules */
	fmt.Println("\n== 4. Goto rules ==")

	// goto cannot jump to a label in another function.
	// goto should not jump over variable declarations that would enter scope.
	// Prefer defer for cleanup when cleanup is simple and tied to one resource.
	fmt.Println("goto is legal, but it should stay rare and obvious.")

	/* Section 5: Final goto rule */
	fmt.Println("\n== 5. Final goto rule ==")

	// Prefer return, break, continue, and defer.
	// Use goto only when it makes a rare control-flow case simpler.
	fmt.Println("Use goto only when the jump makes the code easier to understand.")
}
