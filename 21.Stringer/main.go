// Stringer Interface
// Topic: types can control their printed text by implementing String() string.
// Args/Input: examples use IDs, money, order statuses, structs, pointer receivers, and slices.
// Usage: fmt.Println and fmt.Printf use String when a value implements fmt.Stringer.
// Key idea: Stringer is for human-friendly text, not for JSON or permanent data formats.
package main

import (
	"fmt"
	"strings"
)

/* Helper section 1: Named basic types with Stringer */
type ID int

/* Stringer on a named int type */
func (id ID) String() string {
	// Convert to int to avoid calling ID.String again by accident.
	return fmt.Sprintf("ID-%03d", int(id))
}

type MoneyCents int

/* Stringer for formatted money */
func (m MoneyCents) String() string {
	dollars := int(m) / 100
	cents := int(m) % 100
	return fmt.Sprintf("$%d.%02d", dollars, cents)
}

/* Helper section 2: Enum-like Stringer type */
type OrderStatus int

const (
	StatusUnknown OrderStatus = iota
	StatusPending
	StatusPaid
	StatusShipped
	StatusCanceled
)

/* Stringer on an enum-like type */
func (status OrderStatus) String() string {
	switch status {
	case StatusPending:
		return "pending"
	case StatusPaid:
		return "paid"
	case StatusShipped:
		return "shipped"
	case StatusCanceled:
		return "canceled"
	default:
		return "unknown"
	}
}

/* Helper section 3: Struct and collection Stringers */
type BusinessPerson struct {
	ID   ID
	Name string
}

/* Stringer on a struct */
func (person BusinessPerson) String() string {
	return fmt.Sprintf("%s (%s)", person.Name, person.ID)
}

type Product struct {
	SKU   string
	Name  string
	Price MoneyCents
}

/* Pointer receiver Stringer */
func (product *Product) String() string {
	if product == nil {
		return "<nil product>"
	}

	return fmt.Sprintf("%s - %s", product.Name, product.Price)
}

type Team []BusinessPerson

/* Stringer on a slice type */
func (team Team) String() string {
	names := make([]string, 0, len(team))
	for _, person := range team {
		names = append(names, person.Name)
	}

	return strings.Join(names, ", ")
}

/* Helper section 4: Function that accepts fmt.Stringer */
func printStringer(value fmt.Stringer) {
	// This function accepts any value with String() string.
	fmt.Println("Stringer value:", value.String())
}

func main() {
	/* Section 1: What fmt.Stringer requires */
	fmt.Println("== 1. What fmt.Stringer requires ==")

	// fmt.Stringer is an interface from the fmt package:
	// type Stringer interface { String() string }
	var value fmt.Stringer = ID(7)
	fmt.Println(value)

	/* Section 2: Stringer on a named basic type */
	fmt.Println("\n== 2. Stringer on a named basic type ==")

	myID := ID(30)
	fmt.Println(myID)
	fmt.Printf("with %%v: %v\n", myID)
	fmt.Printf("with %%s: %s\n", myID)

	/* Section 3: Stringer for formatted values */
	fmt.Println("\n== 3. Stringer for formatted values ==")

	price := MoneyCents(1299)
	fmt.Println("price:", price)

	/* Section 4: Stringer on enum-like values */
	fmt.Println("\n== 4. Stringer on enum-like values ==")

	status := StatusPaid
	fmt.Println("status:", status)
	fmt.Println("unknown status:", OrderStatus(99))

	/* Section 5: Stringer on structs */
	fmt.Println("\n== 5. Stringer on structs ==")

	jane := BusinessPerson{
		ID:   1,
		Name: "Jane Doe",
	}
	fmt.Println(jane)

	/* Section 6: Passing Stringer values to functions */
	fmt.Println("\n== 6. Passing Stringer values to functions ==")

	printStringer(myID)
	printStringer(price)
	printStringer(jane)

	/* Section 7: Pointer receiver Stringer */
	fmt.Println("\n== 7. Pointer receiver Stringer ==")

	product := Product{
		SKU:   "P-100",
		Name:  "Go Notebook",
		Price: 2599,
	}

	// Product has String on *Product, so pass a pointer.
	fmt.Println(&product)

	var missingProduct *Product
	fmt.Println(missingProduct)

	/* Section 8: Stringer on collection types */
	fmt.Println("\n== 8. Stringer on collection types ==")

	team := Team{
		{ID: 1, Name: "Jane"},
		{ID: 2, Name: "Ana"},
		{ID: 3, Name: "Ben"},
	}
	fmt.Println("team:", team)

	/* Section 9: Compile-time interface check */
	fmt.Println("\n== 9. Compile-time interface check ==")

	// These lines fail to compile if the type stops satisfying fmt.Stringer.
	var _ fmt.Stringer = ID(0)
	var _ fmt.Stringer = MoneyCents(0)
	var _ fmt.Stringer = BusinessPerson{}
	var _ fmt.Stringer = (*Product)(nil)
	fmt.Println("Stringer checks compiled successfully.")

	/* Section 10: Avoid recursion inside String */
	fmt.Println("\n== 10. Avoid recursion inside String ==")

	// Inside ID.String, use int(id), not fmt.Sprintf("%v", id).
	// Formatting id as ID inside ID.String would call ID.String again.
	fmt.Println("safe ID string:", myID.String())

	/* Section 11: Final Stringer rules */
	fmt.Println("\n== 11. Final Stringer rules ==")

	// Use Stringer for readable logs, debugging, and terminal output.
	// Do not use Stringer as your JSON, database, or API format.
	// Keep String short, safe, and free of surprising side effects.
	fmt.Println("Stringer customizes how your type is printed.")
}
