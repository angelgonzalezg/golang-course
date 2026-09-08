// Enums in Go
// Topic: Go does not have an enum keyword, but const groups can create enum-like values.
// Args/Input: examples use order status, priority, days, and file permissions.
// Usage: use custom types plus const when a value must come from a small fixed set.
// Key idea: typed constants make related values clearer and harder to mix by mistake.
package main

import "fmt"

/* Helper section 1: Numeric enum types and constants */
type OrderStatus int

const (
	StatusUnknown OrderStatus = iota // Zero value for "not set yet".
	StatusPending
	StatusPaid
	StatusShipped
	StatusCanceled
)

/* Helper section 2: String enum types and constants */
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
)

/* Helper section 3: Other enum-style examples */
type DayOfWeek int

const (
	Sunday DayOfWeek = iota + 1 // Start at 1 instead of 0.
	Monday
	Tuesday
	Wednesday
	Thursday
	Friday
	Saturday
)

type FilePermission uint8

const (
	PermissionRead FilePermission = 1 << iota
	PermissionWrite
	PermissionExecute
)

/* Helper section 4: Enum helper functions */
func (status OrderStatus) String() string {
	switch status {
	case StatusUnknown:
		return "unknown"
	case StatusPending:
		return "pending"
	case StatusPaid:
		return "paid"
	case StatusShipped:
		return "shipped"
	case StatusCanceled:
		return "canceled"
	default:
		return "invalid status"
	}
}

func isValidOrderStatus(status OrderStatus) bool {
	switch status {
	case StatusUnknown, StatusPending, StatusPaid, StatusShipped, StatusCanceled:
		return true
	default:
		return false
	}
}

func nextStatus(status OrderStatus) OrderStatus {
	switch status {
	case StatusPending:
		return StatusPaid
	case StatusPaid:
		return StatusShipped
	default:
		return status
	}
}

func parsePriority(text string) (Priority, bool) {
	priority := Priority(text)

	switch priority {
	case PriorityLow, PriorityMedium, PriorityHigh:
		return priority, true
	default:
		return "", false
	}
}

func hasPermission(allPermissions, permission FilePermission) bool {
	return allPermissions&permission != 0
}

func main() {
	/* Section 1: Go-style enum with iota */
	fmt.Println("== 1. Go-style enum with iota ==")

	status := StatusPending
	fmt.Println(status)
	fmt.Printf("number: %d, type: %T\n", status, status)

	/* Section 2: Zero value as unknown */
	fmt.Println("\n== 2. Zero value as unknown ==")

	var emptyStatus OrderStatus
	fmt.Println(emptyStatus)

	/* Section 3: Use enum values in conditions */
	fmt.Println("\n== 3. Use enum values in conditions ==")

	if status == StatusPending {
		fmt.Println("order is waiting for payment")
	}

	/* Section 4: Use switch with enum values */
	fmt.Println("\n== 4. Use switch with enum values ==")

	status = nextStatus(status)
	fmt.Println("next status:", status)

	status = nextStatus(status)
	fmt.Println("next status:", status)

	/* Section 5: Validate enum values */
	fmt.Println("\n== 5. Validate enum values ==")

	fmt.Println("valid status:", isValidOrderStatus(StatusPaid))
	fmt.Println("valid status:", isValidOrderStatus(OrderStatus(99)))

	/* Section 6: String enum values */
	fmt.Println("\n== 6. String enum values ==")

	priority := PriorityHigh
	fmt.Println(priority)
	fmt.Printf("value: %v, type: %T\n", priority, priority)

	/* Section 7: Parse string into enum */
	fmt.Println("\n== 7. Parse string into enum ==")

	parsedPriority, ok := parsePriority("medium")
	if ok {
		fmt.Println("parsed priority:", parsedPriority)
	}

	parsedPriority, ok = parsePriority("urgent")
	if !ok {
		fmt.Println("urgent is not a valid priority")
	}

	/* Section 8: Start iota at another number */
	fmt.Println("\n== 8. Start iota at another number ==")

	fmt.Println("Sunday:", Sunday)
	fmt.Println("Friday:", Friday)

	/* Section 9: Bit flag enums */
	fmt.Println("\n== 9. Bit flag enums ==")

	permissions := PermissionRead | PermissionWrite

	fmt.Println("can read:", hasPermission(permissions, PermissionRead))
	fmt.Println("can write:", hasPermission(permissions, PermissionWrite))
	fmt.Println("can execute:", hasPermission(permissions, PermissionExecute))

	/* Section 10: Enum limits in Go */
	fmt.Println("\n== 10. Enum limits in Go ==")

	// Go allows converting a number to the enum type.
	// Validate values when they come from user input, files, or APIs.
	invalidStatus := OrderStatus(99)
	fmt.Printf("text: %v, number: %d\n", invalidStatus, invalidStatus)

	/* Section 11: Final enum rules */
	fmt.Println("\n== 11. Final enum rules ==")

	// Use numeric enums with iota when order or compact values matter.
	// Use string enums when readable stored values matter.
	// Keep related enum values in one const group.
	fmt.Println("Enums in Go are custom typed constants.")
}
