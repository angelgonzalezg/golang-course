// Constants
// Topic: constants are fixed values that cannot change while the program runs.
// Args/Input: this example does not receive input; it prints constant values.
// Usage: use const for stable values like app names, limits, statuses, and formulas.
// Key idea: constants are known at compile time and cannot be reassigned.
package main

import "fmt"

/* Helper section 1: Package-level constants */
// Package-level constants can be used by every function in this file.
const appName = "GolangCourse"
const defaultPort int = 8080

/* Helper section 2: Grouped constants */
const (
	statusPending  = "pending"
	statusPaid     = "paid"
	statusCanceled = "canceled"
)

/* Helper section 3: iota constants */
const (
	roleGuest = iota
	roleMember
	roleAdmin
)

/* Helper section 4: Bit flag constants */
const (
	readPermission = 1 << iota
	writePermission
	executePermission
)

func main() {
	/* Section 1: Basic constants */
	fmt.Println("== 1. Basic constants ==")

	const greeting = "Hello, constants!"
	fmt.Println(greeting)

	// greeting = "new value" // This would not compile.

	/* Section 2: Package-level constants */
	fmt.Println("\n== 2. Package-level constants ==")

	fmt.Println("app name:", appName)
	fmt.Println("default port:", defaultPort)

	/* Section 3: Local constants */
	fmt.Println("\n== 3. Local constants ==")

	const maxLoginAttempts = 3
	const timeoutSeconds = 30

	fmt.Println("max login attempts:", maxLoginAttempts)
	fmt.Println("timeout seconds:", timeoutSeconds)

	/* Section 4: Typed constants */
	fmt.Println("\n== 4. Typed constants ==")

	const typedYear int = 2026
	const typedRate float64 = 0.16

	fmt.Printf("typed year: %v, type: %T\n", typedYear, typedYear)
	fmt.Printf("typed rate: %v, type: %T\n", typedRate, typedRate)

	/* Section 5: Untyped constants */
	fmt.Println("\n== 5. Untyped constants ==")

	// Untyped constants can adapt when assigned to compatible types.
	const workers = 5

	var workerCount int = workers
	var workerAverage float64 = workers

	fmt.Printf("worker count: %v, type: %T\n", workerCount, workerCount)
	fmt.Printf("worker average: %v, type: %T\n", workerAverage, workerAverage)

	/* Section 6: Constant expressions */
	fmt.Println("\n== 6. Constant expressions ==")

	const secondsPerMinute = 60
	const minutesPerHour = 60
	const secondsPerHour = secondsPerMinute * minutesPerHour

	fmt.Println("seconds per hour:", secondsPerHour)

	const subtotal = 100.0
	const taxRate = 0.16
	const total = subtotal + subtotal*taxRate

	fmt.Println("total with tax:", total)

	/* Section 7: Grouped constants */
	fmt.Println("\n== 7. Grouped constants ==")

	fmt.Println(statusPending)
	fmt.Println(statusPaid)
	fmt.Println(statusCanceled)

	/* Section 8: iota for simple counters */
	fmt.Println("\n== 8. iota for simple counters ==")

	// iota starts at 0 and increases by 1 in a const group.
	fmt.Println("guest:", roleGuest)
	fmt.Println("member:", roleMember)
	fmt.Println("admin:", roleAdmin)

	/* Section 9: iota for bit flags */
	fmt.Println("\n== 9. iota for bit flags ==")

	permissions := readPermission | writePermission

	fmt.Println("can read:", permissions&readPermission != 0)
	fmt.Println("can write:", permissions&writePermission != 0)
	fmt.Println("can execute:", permissions&executePermission != 0)

	/* Section 10: const vs var */
	fmt.Println("\n== 10. const vs var ==")

	// Constants must be known at compile time.
	// Use var when the value is calculated while the program runs.
	currentUsers := 12
	fmt.Println("current users:", currentUsers)

	// const currentUsers = getCurrentUsers() // This would not compile.

	/* Section 11: Final constant rules */
	fmt.Println("\n== 11. Final constant rules ==")

	// Use constants for names, limits, fixed messages, and known formulas.
	// Use variables when the value needs to change.
	fmt.Println("Constants make stable values clear and safe.")
}
