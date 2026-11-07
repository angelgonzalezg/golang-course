package main

import "testing"

func TestLookupResultOwnership(t *testing.T) {
	contacts := []Contact{
		{Name: "Alice", Email: "alice@example.com"},
		{Name: "Bob", Email: "bob@example.com"},
	}
	value, ok := findContactValue(contacts, "Bob")
	if !ok || value != contacts[1] {
		t.Fatalf("value lookup = (%+v, %t), want (%+v, true)", value, ok, contacts[1])
	}
	value.Email = "local-copy@example.com"
	if contacts[1].Email != "bob@example.com" {
		t.Fatal("editing the returned value changed the stored contact")
	}

	pointer := findContactPointer(contacts, "Bob")
	if pointer != &contacts[1] {
		t.Fatalf("pointer lookup = %p, want stored element %p", pointer, &contacts[1])
	}
	pointer.Email = "bob.updated@example.com"
	if contacts[1].Email != "bob.updated@example.com" {
		t.Fatal("editing through the pointer did not change the stored contact")
	}
	if value.Email != "local-copy@example.com" || contacts[0].Email != "alice@example.com" {
		t.Fatal("editing through the pointer changed the independent copy or another contact")
	}
}

func TestMissingContact(t *testing.T) {
	tests := []struct {
		name     string
		contacts []Contact
		query    string
	}{
		{name: "nil slice", query: "Bob"},
		{name: "empty slice", contacts: []Contact{}, query: "Bob"},
		{name: "absent name", contacts: []Contact{{Name: "Alice"}}, query: "Bob"},
		{name: "case differs", contacts: []Contact{{Name: "Bob"}}, query: "bob"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if value, ok := findContactValue(tt.contacts, tt.query); ok || value != (Contact{}) {
				t.Fatalf("missing value lookup = (%+v, %t), want zero value and false", value, ok)
			}
			if pointer := findContactPointer(tt.contacts, tt.query); pointer != nil {
				t.Fatalf("missing pointer lookup = %p, want nil", pointer)
			}
		})
	}
}

func TestLookupReturnsFirstMatch(t *testing.T) {
	contacts := []Contact{
		{Name: "Bob", Email: "first@example.com"},
		{Name: "Bob", Email: "second@example.com"},
	}
	if value, ok := findContactValue(contacts, "Bob"); !ok || value != contacts[0] {
		t.Fatalf("value lookup = (%+v, %t), want the first match", value, ok)
	}
	if pointer := findContactPointer(contacts, "Bob"); pointer != &contacts[0] {
		t.Fatalf("pointer lookup = %p, want first stored element %p", pointer, &contacts[0])
	}
}
