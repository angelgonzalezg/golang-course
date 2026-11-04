# Contact book project

Practice structs, slices, maps, pointers, and functions with `(value, ok)` returns.
Read the composite types, function, and pointer lessons first.

## Run and test

Run these commands from the repository root:

```sh
go run ./chapters/06-pointers/04-project-contact-list
go test ./chapters/06-pointers/04-project-contact-list
```

## Ownership and snapshots

A `ContactBook` owns an ordered slice, a name-to-position index, and its ID
counter. Both `NewContactBook()` and a zero-value `ContactBook` create an empty
usable book. Each book has independent storage and its own IDs.

Pass `*ContactBook` to operations that use the same book. Copying a populated
book value would copy descriptors that can still refer to shared slice and map
data. The functions centralize changes to that storage.

Lookups return `(Contact, bool)`, and `listContacts` returns a copied slice.
These are snapshots: changing a returned contact or list does not update the
book. The `Contact` fields are ints and immutable strings, so copying these
values provides independent editable fields.

Use a saved ID when updating or deleting. An ID still identifies the same
stored contact after the underlying slice grows or other contacts are removed.

## Operations and rules

| Operation | Result |
| --- | --- |
| `addContact(book, name, email, phone)` | Adds a contact and assigns its ID. |
| `findContact(book, name)` | Returns a snapshot for an exact normalized name. |
| `findContactByID(book, id)` | Returns a snapshot for a stable ID. |
| `updateContact(book, id, name, email, phone)` | Replaces editable fields while keeping the ID and list position. |
| `deleteContact(book, id)` | Returns the removed contact and preserves the relative order of the others. |
| `listContacts(book)` | Returns contacts in insertion order as a snapshot. |

- Names are trimmed, case sensitive, nonblank, and unique within this exercise.
  `" Ana "` and `"Ana"` identify the same contact; `"ana"` is a different name.
- Email and phone are stored as trimmed text. Empty values are allowed, and
  an update can clear these fields.
- IDs start at 1, increase after successful additions, and are never reused.
  Adding fails if the ID counter reaches the maximum `int`.
- A rejected addition or update leaves the book unchanged. A duplicate rename
  is rejected before either the slice or the name index is modified.
- Deletion refreshes the indexes of shifted contacts and clears the obsolete
  tail slot through `slices.Delete`.
- Check `ok` before using returned contact values. A nil book yields `false`
  from contact operations and `nil` from `listContacts`.

Name lookup uses the map index. ID lookup and deletion scan or update the
small ordered slice, so they take linear time in the number of contacts.

## Example behavior

The program demonstrates an empty book, adding contacts, rejecting a duplicate,
successful and unsuccessful name lookups, snapshot isolation, renaming, and
deletion. Its output includes:

```text
Found "Bob The Builder" as #2
No exact match for "Bob"
Stored email after changing a snapshot: bob.the.builder@example.com
Renamed #2 to Bob Builder
Old name still exists: false
Duplicate rename rejected.
Deleted #3: Charlie Brown
Added #6: Eve Example
```

## Further practice

After the [methods lesson](../../07-types-methods-and-interfaces/02-methods/main.go),
rewrite the ordinary functions as methods on `*ContactBook`. After the
[errors lesson](../../09-errors/01-error-handling/main.go), distinguish invalid
input, duplicate names, missing IDs, and exhausted IDs with explicit errors.
Later exercises can add JSON persistence and synchronization for shared use.
