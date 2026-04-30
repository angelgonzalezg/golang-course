// Methods
// Topic: methods attach behavior to structs using receivers.
// Args/Input: value receivers read copies; pointer receivers can update the original struct.
// Usage: use methods to read or modify struct data in an object-like style.
// Key idea: receiver type controls whether a method reads a copy or modifies the original.
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

/* Methods of struct - Getters & Setters */
// FullName uses a value receiver, so it reads a copy of Employee.
func (e Employee) FullName() string {
	return e.FirstName + " " + e.LastName
}

// Deactivate uses a pointer receiver, so it modifies the original Employee.
func (e *Employee) Deactivate() {
	e.IsActive = false
}

func (e *Employee) SetJoinedAt(t time.Time) {
	e.JoinedAt = t
}

/* Constructor-style function */
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
	jane := Employee{
		ID:        1,
		FirstName: "Jane",
		LastName:  "Doe",
		Position:  "Night",
		Salary:    100,
		IsActive:  true,
		JoinedAt:  time.Now(),
	}

	// fmt.Println(jane.FullName())
	fmt.Printf("%+v\n", jane)
	jane.Deactivate() // Pointer receiver updates jane
	fmt.Printf("%+v\n", jane)
	jane.SetJoinedAt(time.Now().Add(10 * time.Second)) // Update time field
	fmt.Printf("%+v\n", jane)

	// Panic when method is called from a nil struct pointer
	joe := &Employee{}
	joe = nil
	joe.FullName()
}
