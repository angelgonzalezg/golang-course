// Practice: Contact lookup with values and pointers
// Prerequisites: structs, slices, loops, multiple return values, and pointer basics.
// Usage: go run ./chapters/06-pointers/04-project-contact-list/01-pointer-lookup
// Key idea: choose an independent result or access to the caller's stored element.
package main

import "fmt"

type Contact struct {
	Name  string
	Email string
}

// findContactValue returns the first exact match as an independent value.
// Choose this when callers should edit a result without changing stored data.
// Contact has only string fields; a struct with slices or maps may still share data.
func findContactValue(contacts []Contact, name string) (Contact, bool) {
	for _, contact := range contacts {
		if contact.Name == name {
			return contact, true
		}
	}
	return Contact{}, false
}

// findContactPointer returns the first exact match, or nil if it is absent.
// Choose this when callers should modify the actual element in a stable slice.
// Use the pointer before appending, reordering, or deleting slice elements.
func findContactPointer(contacts []Contact, name string) *Contact {
	for index := range contacts {
		if contacts[index].Name == name {
			// Address the stored element. &contact in a value-range loop would
			// point to a copy, whose fields could be changed independently.
			return &contacts[index]
		}
	}
	return nil
}

func main() {
	contacts := []Contact{
		{Name: "Alice", Email: "alice@example.com"},
		{Name: "Bob", Email: "bob@example.com"},
	}

	fmt.Println("== 1. Edit an independent value ==")
	if contact, ok := findContactValue(contacts, "Bob"); ok {
		contact.Email = "local-copy@example.com"
		fmt.Println("Edited value:", contact.Email)
		fmt.Println("Stored email:", contacts[1].Email)
	}

	fmt.Println("\n== 2. Edit the stored element through a pointer ==")
	if contact := findContactPointer(contacts, "Bob"); contact != nil {
		// Go lets contact.Email stand for (*contact).Email here.
		contact.Email = "bob.updated@example.com"
		fmt.Println("Pointer refers to contacts[1]:", contact == &contacts[1])
		fmt.Println("Stored email:", contacts[1].Email)
	}

	fmt.Println("\n== 3. Handle a missing contact ==")
	_, ok := findContactValue(contacts, "Charlie")
	fmt.Println("Value lookup succeeded:", ok)
	fmt.Println("Pointer lookup returned nil:", findContactPointer(contacts, "Charlie") == nil)
}
