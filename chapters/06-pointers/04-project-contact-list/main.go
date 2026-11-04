// Project: Contact Book
// Topic: combines structs, slices, maps, pointers, and functions.
// Args/Input: names identify contacts for lookup; stable IDs identify updates and deletions.
// Usage: run from the repository root with go run ./chapters/06-pointers/04-project-contact-list.
// Key idea: mutate one owned book through a pointer and return independent contact values to callers.
package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

/* Helper section 1: Contact data model */
type Contact struct {
	ID    int
	Name  string
	Email string
	Phone string
}

/* Helper section 2: One book owns its storage */
// Use *ContactBook to share the same mutable book; copying a populated book
// would copy its slice and map descriptors, which can still share data.
// A zero-value ContactBook is ready to use; IDs start at 1 within each book.
type ContactBook struct {
	contacts    []Contact
	indexByName map[string]int // Normalized name -> position in contacts.
	lastID      int
}

// NewContactBook returns a pointer to an empty book with independent storage.
func NewContactBook() *ContactBook {
	return &ContactBook{}
}

/* Helper section 3: Add and find contacts */
// addContact rejects nil books, blank or duplicate names, and exhausted IDs.
// Names are trimmed and case-sensitive. Email and phone are optional text fields.
func addContact(book *ContactBook, name, email, phone string) (Contact, bool) {
	name = strings.TrimSpace(name)
	if book == nil || name == "" || book.lastID == math.MaxInt {
		return Contact{}, false
	}
	if _, exists := book.indexByName[name]; exists {
		return Contact{}, false
	}

	contact := Contact{
		ID:    book.lastID + 1,
		Name:  name,
		Email: strings.TrimSpace(email),
		Phone: strings.TrimSpace(phone),
	}
	if book.indexByName == nil {
		book.indexByName = make(map[string]int)
	}
	book.contacts = append(book.contacts, contact)
	book.indexByName[name] = len(book.contacts) - 1
	book.lastID = contact.ID
	return contact, true
}

// findContact returns a snapshot, so changing it cannot bypass the name index.
func findContact(book *ContactBook, name string) (Contact, bool) {
	if book == nil {
		return Contact{}, false
	}
	index, exists := book.indexByName[strings.TrimSpace(name)]
	if !exists {
		return Contact{}, false
	}
	return book.contacts[index], true
}

// contactIndexByID scans a small book; names have the separate map index for fast lookup.
func contactIndexByID(book *ContactBook, id int) (int, bool) {
	if book == nil || id <= 0 {
		return 0, false
	}
	for index, contact := range book.contacts {
		if contact.ID == id {
			return index, true
		}
	}
	return 0, false
}

func findContactByID(book *ContactBook, id int) (Contact, bool) {
	index, ok := contactIndexByID(book, id)
	if !ok {
		return Contact{}, false
	}
	return book.contacts[index], true
}

/* Helper section 4: Updates preserve identity and maintain the index */
// updateContact replaces all editable fields while preserving the contact's ID.
// Validate the name before changing either storage structure.
func updateContact(book *ContactBook, id int, name, email, phone string) (Contact, bool) {
	index, ok := contactIndexByID(book, id)
	name = strings.TrimSpace(name)
	if !ok || name == "" {
		return Contact{}, false
	}
	if otherIndex, exists := book.indexByName[name]; exists && otherIndex != index {
		return Contact{}, false
	}

	old := book.contacts[index]
	updated := Contact{ID: old.ID, Name: name, Email: strings.TrimSpace(email), Phone: strings.TrimSpace(phone)}
	delete(book.indexByName, old.Name)
	book.contacts[index] = updated
	book.indexByName[name] = index
	return updated, true
}

// deleteContact preserves the relative order and IDs of the remaining contacts.
func deleteContact(book *ContactBook, id int) (Contact, bool) {
	index, ok := contactIndexByID(book, id)
	if !ok {
		return Contact{}, false
	}
	removed := book.contacts[index]
	delete(book.indexByName, removed.Name)
	book.contacts = slices.Delete(book.contacts, index, index+1) // Also clears the obsolete tail slot.
	// Entries after the removed position shifted left, so refresh their indexes.
	for next := index; next < len(book.contacts); next++ {
		book.indexByName[book.contacts[next].Name] = next
	}
	return removed, true
}

/* Helper section 5: Return a list snapshot and keep presentation separate */
func listContacts(book *ContactBook) []Contact {
	if book == nil {
		return nil
	}
	// Contact contains only ints and strings, so copying the slice gives callers independent values.
	return slices.Clone(book.contacts)
}

func printContacts(contacts []Contact) {
	if len(contacts) == 0 {
		fmt.Println("No contacts found.")
		return
	}
	for index, contact := range contacts {
		fmt.Printf("%d. ID: %d, Name: %s, Email: %s, Phone: %s\n", index+1, contact.ID, contact.Name, contact.Email, contact.Phone)
	}
}

func main() {
	book := NewContactBook()
	fmt.Println("== 1. Empty book ==")
	printContacts(listContacts(book))

	fmt.Println("\n== 2. Add contacts ==")
	initial := []Contact{
		{Name: "Alice Wonderland", Email: "alice@example.com", Phone: "111-2221"},
		{Name: "Bob The Builder", Email: "bob.the.builder@example.com", Phone: "333-4444"},
		{Name: "Charlie Brown", Email: "charlie.brown@example.com", Phone: "444-1111"},
		{Name: "Sponge Bob Square Pants", Email: "squarepants@example.com", Phone: "100-1212"},
	}
	for _, input := range initial {
		contact, ok := addContact(book, input.Name, input.Email, input.Phone)
		if !ok {
			fmt.Println("Could not add:", input.Name)
			return
		}
		fmt.Printf("Added #%d: %s\n", contact.ID, contact.Name)
	}
	if _, ok := addContact(book, " Alice Wonderland ", "duplicate@example.com", ""); !ok {
		fmt.Println("Duplicate name rejected.")
	}

	fmt.Println("\n== 3. Exact name lookup ==")
	for _, name := range []string{"Bob The Builder", "Bob"} {
		if contact, ok := findContact(book, name); ok {
			fmt.Printf("Found %q as #%d\n", name, contact.ID)
		} else {
			fmt.Printf("No exact match for %q\n", name)
		}
	}

	fmt.Println("\n== 4. Snapshots and updates by ID ==")
	bob, ok := findContact(book, "Bob The Builder")
	if !ok {
		return
	}
	bob.Email = "local-copy@example.com" // Changing this snapshot does not update the book.
	if stored, ok := findContactByID(book, bob.ID); ok {
		fmt.Println("Stored email after changing a snapshot:", stored.Email)
	}
	// Growing the slice does not invalidate a saved ID.
	if added, ok := addContact(book, "Dana Developer", "dana@example.com", "555-0101"); ok {
		fmt.Printf("Added #%d: %s\n", added.ID, added.Name)
	}
	updated, ok := updateContact(book, bob.ID, "Bob Builder", "bob.updated@example.com", bob.Phone)
	if !ok {
		fmt.Println("Could not update Bob.")
		return
	}
	fmt.Printf("Renamed #%d to %s\n", updated.ID, updated.Name)
	_, oldNameExists := findContact(book, "Bob The Builder")
	fmt.Println("Old name still exists:", oldNameExists)
	if _, ok := updateContact(book, bob.ID, "Alice Wonderland", "", ""); !ok {
		fmt.Println("Duplicate rename rejected.")
	}

	fmt.Println("\n== 5. Delete without reusing IDs ==")
	if charlie, ok := findContact(book, "Charlie Brown"); ok {
		if removed, ok := deleteContact(book, charlie.ID); ok {
			fmt.Printf("Deleted #%d: %s\n", removed.ID, removed.Name)
		}
	}
	if added, ok := addContact(book, "Eve Example", "eve@example.com", "555-0102"); ok {
		fmt.Printf("Added #%d: %s\n", added.ID, added.Name)
	}

	fmt.Println("\n== 6. Final insertion order ==")
	printContacts(listContacts(book))
}
