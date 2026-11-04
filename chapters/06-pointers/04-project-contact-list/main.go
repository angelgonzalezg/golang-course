// Project: Contact List
// Topic: combines structs, slices, maps, pointers, and functions.
// Args/Input: contacts are added with name, email, and phone; search uses name.
// Usage: store contacts in a slice and keep a map index for quick lookup by name.
// Key idea: a slice stores ordered data while a map provides fast name-based indexing.
package main

import "fmt"

/* Helper section 1: Contact data model */
type Contact struct {
	ID    int
	Name  string
	Email string
	Phone string
}

/* Helper section 2: Contact storage */
var contactList []Contact
var contactIndexByName map[string]int // Name -> index in contactList
var nextID int = 1

/* Helper section 3: Contact list helpers */
func init() {
	// Initialize storage before main runs.
	contactList = make([]Contact, 0)
	contactIndexByName = make(map[string]int, 0)
}

func addcontact(name, email, phone string) {
	if _, exists := contactIndexByName[name]; exists { // Prevent duplicates
		fmt.Printf("Contact already exists: %v\n", name)
		return
	}

	newContact := Contact{
		ID:    nextID,
		Name:  name,
		Email: email,
		Phone: phone,
	}
	nextID += 1 // Prepare ID for next contact
	contactList = append(contactList, newContact)
	contactIndexByName[name] = len(contactList) - 1 // Save index for fast search
	fmt.Printf("Contact added: %v\n", name)
}

func findContact(name string) *Contact {
	index, exists := contactIndexByName[name]
	if exists {
		return &contactList[index] // Return pointer to existing contact
	}
	return nil
}

func listContacts() {
	fmt.Println("--- Listing Contacts ---")
	if len(contactList) == 0 {
		fmt.Println("No contacts found.")
		return
	}

	for index, contact := range contactList {
		fmt.Printf("%d. ID: %d, Name: %s, Email: %s, Phone: %s\n", index+1, contact.ID, contact.Name, contact.Email, contact.Phone)
	}

	fmt.Println("")
}

func main() {
	listContacts()

	addcontact("Alice Wonderland", "alice@example.com", "111-2221")
	addcontact("Bob The Builder", "bob.the.builder@example.com", "333-4444")
	addcontact("Charlie Brown", "charlie.brown@example.com", "444-1111")
	addcontact("Sponge Bob Square Pants", "squarepants@example.com", "100-1212")

	// Attempt to add duplicate
	addcontact("Alice Wonderland", "alice@example.com", "111-2221")

	listContacts()

	bob := findContact("Bob")
	// bob := findContact("Bob The Builder")
	if bob == nil {
		fmt.Println("No Bob contact found!")
	} else {
		fmt.Println("Bob contact found,", bob.Name)
	}
}
