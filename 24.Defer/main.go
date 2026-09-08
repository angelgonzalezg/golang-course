// Defer
// Topic: defer schedules a function call to run when the surrounding function returns.
// Args/Input: examples use print statements, early returns, variables, and cleanup.
// Usage: use defer for cleanup tasks like closing files, unlocking locks, or logging completion.
// Key idea: deferred calls run after the function finishes, in last-in-first-out (LIFO) order.
package main

import "fmt"

/* Helper section 1: Defer behavior helpers */
/* Basic defer */
func basicDefer() {
	fmt.Println("basicDefer: start")
	defer fmt.Println("basicDefer: deferred")
	fmt.Println("basicDefer: end")
}

/* Multiple defers run in reverse order */
func lifoDefer() {
	fmt.Println("lifoDefer: start")
	defer fmt.Println("lifoDefer: first defer")
	defer fmt.Println("lifoDefer: second defer")
	defer fmt.Println("lifoDefer: third defer")
	fmt.Println("lifoDefer: end")
}

/* Defer still runs before an early return */
func earlyReturn(allowed bool) {
	defer fmt.Println("earlyReturn: cleanup")

	if !allowed {
		fmt.Println("earlyReturn: not allowed")
		return
	}

	fmt.Println("earlyReturn: allowed")
}

/* Deferred arguments are evaluated immediately */
func deferArguments() {
	name := "Ana"
	defer fmt.Println("deferArguments:", name)

	name = "Ben"
	fmt.Println("current name:", name)
}

/* Deferred anonymous function reads the latest variable value */
func deferClosure() {
	name := "Ana"
	defer func() {
		fmt.Println("deferClosure:", name)
	}()

	name = "Ben"
	fmt.Println("current name:", name)
}

/* Helper section 2: Cleanup helpers */
/* Cleanup example */
func openResource(name string) {
	fmt.Println("Open resource:", name)
}

func closeResource(name string) {
	fmt.Println("Close resource:", name)
}

func useResource(name string) {
	openResource(name)
	defer closeResource(name)

	fmt.Println("Use resource:", name)
}

/* Helper section 3: Advanced defer examples */
/* Defer can update named return values */
func addAuditSuffix(message string) (result string) {
	result = message
	defer func() {
		result += " [done]"
	}()

	return result
}

/* Defer inside a loop runs when the function returns, not each loop turn */
func deferInLoop() {
	for index := 1; index <= 3; index++ {
		defer fmt.Println("deferInLoop:", index)
	}

	fmt.Println("deferInLoop: loop finished")
}

func main() {
	/* Section 1: Basic defer */
	fmt.Println("== 1. Basic defer ==")

	basicDefer()

	/* Section 2: Multiple defers */
	fmt.Println("\n== 2. Multiple defers ==")

	lifoDefer()

	/* Section 3: Defer with early return */
	fmt.Println("\n== 3. Defer with early return ==")

	earlyReturn(false)

	/* Section 4: Deferred arguments */
	fmt.Println("\n== 4. Deferred arguments ==")

	deferArguments()

	/* Section 5: Deferred closures */
	fmt.Println("\n== 5. Deferred closures ==")

	deferClosure()

	/* Section 6: Cleanup with defer */
	fmt.Println("\n== 6. Cleanup with defer ==")

	useResource("report.txt")

	/* Section 7: Defer and named return values */
	fmt.Println("\n== 7. Defer and named return values ==")

	fmt.Println(addAuditSuffix("save report"))

	/* Section 8: Defer inside a loop */
	fmt.Println("\n== 8. Defer inside a loop ==")

	// Avoid many defers in long loops; they wait until the function returns.
	deferInLoop()

	/* Section 9: Final defer rules */
	fmt.Println("\n== 9. Final defer rules ==")

	// Put defer right after successful resource creation.
	// Deferred calls run in reverse order.
	// Use defer for cleanup, not for normal control flow.
	fmt.Println("defer keeps cleanup close to the work that needs it.")
}
