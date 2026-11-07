# Contact lookup: values and pointers

Practice the same lookup using two return types before adding a contact book's
IDs, map index, and validation. Read
[structs](../../../03-composite-types/06-structs/main.go),
[multiple return values](../../../05-functions/02-multiple-return-values/main.go),
and [pointer basics](../../01-pointers/main.go) first.

## Run and compare

Run from the repository root:

```sh
go run ./chapters/06-pointers/04-project-contact-list/01-pointer-lookup
go test ./chapters/06-pointers/04-project-contact-list/01-pointer-lookup
```

Both functions receive `[]Contact`, a slice of struct values, and scan for the
first exact, case-sensitive name match. They return results without printing.
The caller in `main` chooses what to change and display.

| Function | Result | Choose it when |
| --- | --- | --- |
| `findContactValue` | `(Contact, bool)`: a copied contact and a found flag. | The caller needs an independent result, such as data for display or local editing. |
| `findContactPointer` | `*Contact`: the address of the stored element, or `nil`. | The caller is allowed to change that element directly while the slice's structure stays stable. |

Check `ok` before using a value result and check for `nil` before dereferencing
a pointer result. Missing names, empty slices, and nil slices are valid inputs.
The value lookup returns `(Contact{}, false)` when it finds no match.

The output makes the difference visible:

```text
== 1. Edit an independent value ==
Edited value: local-copy@example.com
Stored email: bob@example.com

== 2. Edit the stored element through a pointer ==
Pointer refers to contacts[1]: true
Stored email: bob.updated@example.com

== 3. Handle a missing contact ==
Value lookup succeeded: false
Pointer lookup returned nil: true
```

## Trace the stored element and the copy

`Contact` contains only strings. Copying it gives independently editable fields;
changing the returned value's `Email` leaves the stored field unchanged. If the
struct later contains a slice or map, copying it also copies references to that
data, so nested contents may still be shared.

Passing the slice copies its descriptor, which still refers to the caller's
backing array. `&contacts[index]` points to an element in that array. The pointer
function ranges over indexes so it can address the stored contact. In a
`for _, contact := range contacts` loop, `contact` is a copy; returning `&contact`
would point to that copy. Such a pointer is safe to use, but updating its fields
would not update the slice element.

`contact.Email` works on both a value and a pointer to a struct. For a pointer,
Go treats the field access as `(*contact).Email`. Both representations allow
mutation; the difference here is which object changes. The slice itself still
contains values (`[]Contact`), even when lookup returns `*Contact`.

## Keep the pointer's role clear

The caller owns this small slice and keeps its structure stable while using a
returned pointer. Finish those edits before appending, reordering, or deleting
elements. Appending may allocate a new backing array: a saved pointer remains
valid, but refers to the old array rather than the copied element in the new
one. Reordering or deletion may put a different contact in a saved position.

Choose the required behavior first. Returning a pointer does not by itself
guarantee a heap allocation, a speedup, or lower memory use. A pointer can keep
the backing array reachable; measure copying, allocation, and GC costs when
performance matters. The
[pointer performance](../../02-pointer-performance/README.md) and
[GC](../../03-garbage-collector/README.md) lessons explore those costs separately.

## Try it, then extend it

1. Change the query to `"Alice"`, `"bob"`, and a missing name. Predict `ok` and
   `nil` before running the program.
2. Change `Name` through each result type. Check whether a lookup using the old
   or new name succeeds. This exercise has no separate name index to maintain.
3. Add a second contact with the same name and confirm that the first match is
   returned. Decide whether your application should allow duplicate names.

Continue with the [complete contact book](../README.md). It adds stable IDs,
unique normalized names, and a map index. Its functions return snapshots and
update the book through controlled operations so renames also refresh that
index. Both designs use pointers deliberately, with different editing contracts.
