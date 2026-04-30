// Stringer Interface
// Topic: types can control their printed text by implementing String() string.
// Args/Input: String receives no args and returns the custom string representation.
// Usage: fmt.Println automatically calls String when a value implements fmt.Stringer.
// Key idea: implementing String() customizes how fmt displays your type.
package main

import (
	"fmt"
)

type Person interface {
	String() string // Same method required by fmt.Stringer
}

type BusinessPerson struct {
	ID   int
	Name string
}

func (e BusinessPerson) String() string {
	// Custom format for BusinessPerson
	return fmt.Sprintf("Person[ID:%d, Name:%s]", e.ID, e.Name)
}

type ID int

func (idx ID) String() string {
	// Custom format for ID
	return fmt.Sprintf("ID: %d", idx)
}

func main() {
	jane := BusinessPerson{
		ID:   1,
		Name: "Jane",
	}

	fmt.Println(jane) // Uses BusinessPerson.String()

	myId := ID(30)
	fmt.Println(myId) // Uses ID.String()

}
