// Enums with Constants
// Topic: Go creates enum-like groups using custom types and const blocks.
// Args/Input: iota generates increasing numeric values; string constants store readable labels.
// Usage: group related values like days or roles to make code clearer and safer.
// Key idea: typed const groups make related values clearer and harder to misuse.
package main

import "fmt"

type DayOfWeek int

const (
	Sunday DayOfWeek = iota + 1 // Start at 1 instead of 0
	Monday
	Tuesday
	Wednesday
	Thursday
	Friday
	Saturday
)

type Role string

const (
	Admin    Role = "ADMIN"
	User     Role = "USER"
	Designer Role = "DESIGNER"
)

func main() {
	fmt.Println(Sunday)
	fmt.Println(Friday)
	fmt.Println(User)
}
