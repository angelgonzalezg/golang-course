// Structs
// Topic: structs group related fields into one custom data type.
// Args/Input: Employee stores identity, job info, salary, status, and creation time.
// Usage: create structs with literals or constructor functions, then read/update fields.
// Key idea: structs model real entities by grouping related data fields.
package main

import (
	"fmt"
	"time"
)

type Employee struct {
	ID        int
	FirstName string
	LastName  string
	Position  string
	Salary    int
	IsActive  bool
	JoinedAt  time.Time
}

// NewEmployee creates an Employee and sets JoinedAt automatically.
func NewEmployee(id int, firstName, lastName, position string, isActive bool) Employee {
	return Employee{
		ID:        id,
		FirstName: firstName,
		LastName:  lastName,
		Position:  position,
		IsActive:  isActive,
		JoinedAt:  time.Now(),
	}
}

func main() {
	// Struct literal with named fields.
	jane := Employee{
		ID:        1,
		FirstName: "Jane",
		LastName:  "Doe",
		Position:  "Night",
		Salary:    100,
		IsActive:  true,
		JoinedAt:  time.Now(),
	}

	fmt.Println(jane)
	fmt.Printf("%v\n", jane)
	fmt.Printf("%+v\n", jane) // Prints field names with values
	fmt.Println(jane.ID)
	fmt.Println(jane.FirstName)
	fmt.Println(jane.LastName)
	fmt.Println(jane.Position)
	fmt.Println(jane.Salary)
	fmt.Println(jane.IsActive)
	fmt.Println(jane.JoinedAt)

	joe := NewEmployee(1, "Joe", "Doe", "Night", true) // Constructor-style creation
	fmt.Println(joe.FirstName)
	fmt.Println(joe.LastName)
	fmt.Println(joe.Position)

	joe.Salary = 100000
	fmt.Println(joe.Salary)
	fmt.Printf("%+v\n", joe)

	joePtr := &joe // Pointer to the same struct value
	fmt.Println(joePtr.FirstName)
	fmt.Println(joePtr.LastName)
	fmt.Println(joePtr.Position)

	joePtr.IsActive = false // Updates original joe
	joePtr.LastName = "Adam"
	fmt.Printf("%+v\n", joe)

}
