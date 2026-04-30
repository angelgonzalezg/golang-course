// Interfaces
// Topic: interfaces define behavior by listing method signatures, not fields.
// Args/Input: displayPerson receives a Person, meaning any type with GetName() string.
// Usage: different structs can be used the same way when they satisfy the interface.
// Key idea: implementation is implicit; no "implements" keyword is needed in Go.
package main

import (
	"fmt"
)

type Person interface {
	GetName() string // Required method to be a Person
}

/* Concrete structs with different types but similar behavior */
type Employee struct {
	ID   int
	Name string
}

type BusinessPerson struct {
	ID   int
	Name string
}

/* These methods make Employee and BusinessPerson satisfy Person (simulates Polymorphism) */
func (e Employee) GetName() string {
	return e.Name
}

func (b BusinessPerson) GetName() string {
	return b.Name
}

func displayPerson(p Person) {
	// The function depends on behavior, not the concrete struct type.
	fmt.Println(p.GetName())
}

func main() {
	joe := Employee{
		ID:   1,
		Name: "Joe",
	}

	jane := BusinessPerson{
		ID:   1,
		Name: "Jane",
	}

	displayPerson(joe)
	displayPerson(jane)

}
