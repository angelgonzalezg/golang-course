package main

import (
	"maps"
	"math"
	"slices"
	"testing"
)

func mustAddContact(t *testing.T, book *ContactBook, name string) Contact {
	t.Helper()
	contact, ok := addContact(book, name, name+"@example.com", "555-0100")
	if !ok {
		t.Fatalf("could not add %q", name)
	}
	return contact
}

func assertBookConsistent(t *testing.T, book *ContactBook) {
	t.Helper()
	if len(book.contacts) != len(book.indexByName) {
		t.Fatalf("contacts=%d index entries=%d", len(book.contacts), len(book.indexByName))
	}
	seenIDs := make(map[int]bool)
	for index, contact := range book.contacts {
		if contact.ID <= 0 || contact.ID > book.lastID || seenIDs[contact.ID] {
			t.Fatalf("invalid or repeated ID: %d", contact.ID)
		}
		seenIDs[contact.ID] = true
		if got, exists := book.indexByName[contact.Name]; !exists || got != index {
			t.Fatalf("name %q indexed at %d (exists=%t), want %d", contact.Name, got, exists, index)
		}
		if got, ok := findContact(book, contact.Name); !ok || got != contact {
			t.Fatalf("name lookup disagrees with stored contact %+v", contact)
		}
		if got, ok := findContactByID(book, contact.ID); !ok || got != contact {
			t.Fatalf("ID lookup disagrees with stored contact %+v", contact)
		}
	}
}

func TestContactBookZeroValueAndIndependentBooks(t *testing.T) {
	var zero ContactBook
	first := NewContactBook()
	if len(listContacts(first)) != 0 || len(listContacts(&zero)) != 0 {
		t.Fatal("new and zero-value books must start empty")
	}
	ana := mustAddContact(t, first, "Ana")
	if ana.ID != 1 {
		t.Fatalf("first contact ID = %d, want 1", ana.ID)
	}
	if _, ok := findContact(&zero, "Ana"); ok {
		t.Fatal("books must not share contacts")
	}
	otherAna := mustAddContact(t, &zero, "Ana")
	if otherAna.ID != 1 {
		t.Fatal("each book must have its own ID counter")
	}
	if _, ok := updateContact(&zero, otherAna.ID, "Ann", "", ""); !ok {
		t.Fatal("could not update the zero-value book")
	}
	if stored, ok := findContact(first, "Ana"); !ok || stored != ana {
		t.Fatal("updating one book changed another book")
	}
	assertBookConsistent(t, first)
	assertBookConsistent(t, &zero)
}

func TestAddContactValidationAndNormalization(t *testing.T) {
	book := NewContactBook()
	got, ok := addContact(book, " \tAna\n", " ana@example.com ", " 555-0100 ")
	want := Contact{ID: 1, Name: "Ana", Email: "ana@example.com", Phone: "555-0100"}
	if !ok || got != want {
		t.Fatalf("normalized contact = (%+v, %t), want %+v and true", got, ok, want)
	}
	for _, name := range []string{"Ana", " Ana ", "", " \t\n"} {
		before, beforeIndex, beforeID := listContacts(book), maps.Clone(book.indexByName), book.lastID
		if contact, ok := addContact(book, name, "other@example.com", ""); ok || contact != (Contact{}) {
			t.Fatalf("invalid or duplicate name %q accepted: %+v", name, contact)
		}
		if !slices.Equal(before, book.contacts) || !maps.Equal(beforeIndex, book.indexByName) || beforeID != book.lastID {
			t.Fatalf("rejected add for %q changed the book", name)
		}
	}
	if stored, ok := findContact(book, "\u00a0Ana\u00a0"); !ok || stored != want {
		t.Fatal("lookups must use the same whitespace normalization as adds")
	}
	if lower, ok := addContact(book, "ana", "", ""); !ok || lower.ID != 2 || lower.Email != "" || lower.Phone != "" {
		t.Fatal("names are case sensitive, and optional fields may be empty")
	}
	if _, ok := findContact(book, "An"); ok {
		t.Fatal("lookup must use a full name rather than a prefix")
	}
	assertBookConsistent(t, book)
}

func TestUpdateContactMaintainsNameIndex(t *testing.T) {
	book := NewContactBook()
	ana := mustAddContact(t, book, "Ana")
	ben := mustAddContact(t, book, "Ben")
	updated, ok := updateContact(book, ana.ID, " Ann ", " ann@example.com ", " 555-0200 ")
	want := Contact{ID: ana.ID, Name: "Ann", Email: "ann@example.com", Phone: "555-0200"}
	if !ok || updated != want {
		t.Fatalf("update = (%+v, %t), want %+v and true", updated, ok, want)
	}
	if _, ok := findContact(book, "Ana"); ok {
		t.Fatal("the old name must be removed from the index")
	}
	if got, ok := findContact(book, "Ann"); !ok || got != want {
		t.Fatal("the new name must resolve to the same contact ID")
	}
	if got, ok := findContactByID(book, ben.ID); !ok || got != ben {
		t.Fatal("renaming one contact changed another contact")
	}

	tests := []struct {
		name string
		id   int
	}{
		{name: " Ben ", id: ana.ID},
		{name: " \t", id: ana.ID},
		{name: "Missing", id: 99},
		{name: "Zero", id: 0},
		{name: "Negative", id: -1},
	}
	for _, tt := range tests {
		before, beforeIndex, beforeID := listContacts(book), maps.Clone(book.indexByName), book.lastID
		if contact, ok := updateContact(book, tt.id, tt.name, "", ""); ok || contact != (Contact{}) {
			t.Fatalf("invalid update accepted: id=%d name=%q", tt.id, tt.name)
		}
		if !slices.Equal(before, book.contacts) || !maps.Equal(beforeIndex, book.indexByName) || beforeID != book.lastID {
			t.Fatal("a rejected update changed the book")
		}
	}
	if cleared, ok := updateContact(book, ana.ID, "Ann", "", ""); !ok || cleared.ID != ana.ID || cleared.Email != "" || cleared.Phone != "" {
		t.Fatal("updating the same name should allow clearing optional fields")
	}
	assertBookConsistent(t, book)
}

func TestDeleteContactRefreshesIndexesAndPreservesIDs(t *testing.T) {
	tests := []struct {
		name    string
		id      int
		wantIDs []int
	}{
		{name: "first", id: 1, wantIDs: []int{2, 3}},
		{name: "middle", id: 2, wantIDs: []int{1, 3}},
		{name: "last", id: 3, wantIDs: []int{1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			book := NewContactBook()
			for _, name := range []string{"Ana", "Ben", "Cara"} {
				mustAddContact(t, book, name)
			}
			storage := book.contacts
			removed, ok := deleteContact(book, tt.id)
			if !ok || removed.ID != tt.id {
				t.Fatalf("deleteContact(%d) = (%+v, %t)", tt.id, removed, ok)
			}
			if _, ok := findContact(book, removed.Name); ok {
				t.Fatal("deleted name is still indexed")
			}
			if _, ok := findContactByID(book, tt.id); ok {
				t.Fatal("deleted ID is still present")
			}
			if storage[len(storage)-1] != (Contact{}) {
				t.Fatal("the obsolete tail slot must not retain contact strings")
			}
			var ids []int
			for _, contact := range listContacts(book) {
				ids = append(ids, contact.ID)
			}
			if !slices.Equal(ids, tt.wantIDs) {
				t.Fatalf("remaining IDs = %v, want %v", ids, tt.wantIDs)
			}
			assertBookConsistent(t, book)

			// The name can be used again, but its old ID must stay retired.
			added := mustAddContact(t, book, removed.Name)
			if added.ID != 4 {
				t.Fatalf("new ID = %d, want 4", added.ID)
			}
			remaining := listContacts(book)[0]
			if _, ok := updateContact(book, remaining.ID, "Updated "+remaining.Name, remaining.Email, remaining.Phone); !ok {
				t.Fatal("updating a remaining contact after deletion failed")
			}
			assertBookConsistent(t, book)
			before, beforeID := listContacts(book), book.lastID
			if _, ok := deleteContact(book, 99); ok || !slices.Equal(before, listContacts(book)) || beforeID != book.lastID {
				t.Fatal("deleting an unknown ID changed the book")
			}
			assertBookConsistent(t, book)
		})
	}
}

func TestDeleteOnlyContactAndReuseBook(t *testing.T) {
	book := NewContactBook()
	ana := mustAddContact(t, book, "Ana")
	if _, ok := deleteContact(book, ana.ID); !ok || len(listContacts(book)) != 0 {
		t.Fatal("deleting the only contact must leave an empty book")
	}
	assertBookConsistent(t, book)
	added := mustAddContact(t, book, "Ana")
	if added.ID != 2 {
		t.Fatal("reusing an emptied book must not reuse a retired ID")
	}
	assertBookConsistent(t, book)
}

func TestContactSnapshotsAndGrowth(t *testing.T) {
	book := NewContactBook()
	book.contacts = make([]Contact, 0, 1) // Force reallocation when the second contact is added.
	ana := mustAddContact(t, book, "Ana")
	saved, ok := findContact(book, "Ana")
	if !ok {
		t.Fatal("contact not found")
	}
	saved.Name, saved.Email = "Local copy", "local@example.com"
	mustAddContact(t, book, "Ben")
	if stored, ok := findContactByID(book, ana.ID); !ok || stored != ana {
		t.Fatal("changing a snapshot or growing the slice changed the stored contact")
	}
	updated, ok := updateContact(book, ana.ID, "Ann", "ann@example.com", ana.Phone)
	if !ok || updated.ID != ana.ID {
		t.Fatal("a saved ID must remain usable after slice growth")
	}
	updated.Name = "Changed returned value"
	if stored, ok := findContact(book, "Ann"); !ok || stored.Name != "Ann" {
		t.Fatal("changing an update result changed the book")
	}
	assertBookConsistent(t, book)
}

func TestListContactsReturnsIndependentSnapshots(t *testing.T) {
	book := NewContactBook()
	ana := mustAddContact(t, book, "Ana")
	ben := mustAddContact(t, book, "Ben")
	editable, saved := listContacts(book), listContacts(book)
	editable[0].Name, editable[0].ID = "Changed locally", -1
	editable[1] = Contact{}
	if !slices.Equal(listContacts(book), []Contact{ana, ben}) {
		t.Fatal("editing a list snapshot changed stored contacts")
	}
	if _, ok := updateContact(book, ana.ID, "Ann", "", ""); !ok {
		t.Fatal("could not update contact")
	}
	if !slices.Equal(saved, []Contact{ana, ben}) {
		t.Fatal("a later book update changed an earlier snapshot")
	}
	assertBookConsistent(t, book)
}

func TestContactBookIDExhaustion(t *testing.T) {
	book := NewContactBook()
	book.lastID = math.MaxInt - 1
	contact := mustAddContact(t, book, "Last")
	if contact.ID != math.MaxInt {
		t.Fatalf("last valid ID = %d, want %d", contact.ID, math.MaxInt)
	}
	before := listContacts(book)
	if _, ok := addContact(book, "Overflow", "", ""); ok || !slices.Equal(before, listContacts(book)) || book.lastID != math.MaxInt {
		t.Fatal("ID exhaustion must fail without wrapping or changing the book")
	}
	assertBookConsistent(t, book)
}

func TestNilContactBook(t *testing.T) {
	if _, ok := addContact(nil, "Ana", "", ""); ok {
		t.Fatal("adding to a nil book must fail")
	}
	if _, ok := findContact(nil, "Ana"); ok {
		t.Fatal("lookup in a nil book must fail")
	}
	if _, ok := findContactByID(nil, 1); ok {
		t.Fatal("ID lookup in a nil book must fail")
	}
	if _, ok := updateContact(nil, 1, "Ana", "", ""); ok {
		t.Fatal("updating a nil book must fail")
	}
	if _, ok := deleteContact(nil, 1); ok {
		t.Fatal("deleting from a nil book must fail")
	}
	if listContacts(nil) != nil {
		t.Fatal("listing a nil book must return nil")
	}
}
