// Structs
// Topic: structs group related fields into one custom data type.
// Args/Input: examples use employees, addresses, JSON fields, and badges.
// Usage: create structs, read/update fields, copy values, use pointers, nest/embed structs, and use tags.
// Key idea: structs model real things by keeping related data together.
package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

/* Helper section 1: Struct type definitions */
type Address struct {
	City    string
	Country string
}

type Employee struct {
	ID        int
	FirstName string
	LastName  string
	Position  string
	Salary    int
	IsActive  bool
	JoinedAt  time.Time
	Skills    []string // A slice field demonstrates shared data after a struct assignment.
	// Nested struct: Employee has an Address field.
	Address Address
}

type Manager struct {
	// Embedding promotes fields and methods; Manager remains a distinct type.
	Employee
	TeamSize int
}

type APIEmployee struct {
	ID        int    `json:"id"`              // Rename to "id".
	FirstName string `json:"first_name"`      // Rename to "first_name".
	LastName  string `json:"last_name"`       // Rename to "last_name".
	Email     string `json:"email,omitempty"` // Skip when empty.
	Password  string `json:"-"`               // Always omit this field.
}

type Badge struct {
	ID     int
	Level  string
	Active bool
}

/* Helper section 2: Constructor-style helper */
// NewEmployee creates an Employee and sets JoinedAt automatically.
func NewEmployee(id int, firstName, lastName, position string, salary int) Employee {
	return Employee{
		ID:        id,
		FirstName: firstName,
		LastName:  lastName,
		Position:  position,
		Salary:    salary,
		IsActive:  true,
		JoinedAt:  time.Now(),
	}
}

// cloneEmployee copies the Employee value and duplicates its Skills slice.
func cloneEmployee(employee Employee) Employee {
	employee.Skills = slices.Clone(employee.Skills)
	return employee
}

func main() {
	/* Section 1: Zero value */
	fmt.Println("== 1. Zero value ==")

	/* Zero value */
	// Each field starts with the zero value of its type.
	var emptyEmployee Employee
	fmt.Printf("empty employee: %+v\n", emptyEmployee)

	/* Section 2: Struct literal */
	fmt.Println("\n== 2. Struct literal ==")

	/* Struct literal */
	// Named fields are clearer and do not depend on field order.
	jane := Employee{
		ID:        1,
		FirstName: "Jane",
		LastName:  "Doe",
		Position:  "Developer",
		Salary:    100000,
		IsActive:  true,
		JoinedAt:  time.Now(),
		Skills:    []string{"Go", "SQL"},
		Address: Address{
			City:    "Mexico City",
			Country: "Mexico",
		},
	}
	fmt.Printf("jane: %+v\n", jane)

	/* Section 3: Read and update fields */
	fmt.Println("\n== 3. Read and update fields ==")

	/* Read and update fields */
	fmt.Printf("first name: %s\n", jane.FirstName)
	jane.Position = "Senior Developer"
	jane.Salary = 120000
	fmt.Printf("jane after update: %+v\n", jane)

	/* Section 4: Constructor-style function */
	fmt.Println("\n== 4. Constructor-style function ==")

	/* Constructor-style function */
	// Go does not have constructors, but functions like this are common.
	joe := NewEmployee(2, "Joe", "Doe", "Designer", 90000)
	fmt.Printf("joe: %+v\n", joe)

	/* Section 5: Struct assignment copies values */
	fmt.Println("\n== 5. Struct assignment copies values ==")

	/* Struct assignment copies values */
	// Assignment copies each field; reference-containing fields can still share data.
	janeCopy := jane
	janeCopy.FirstName = "Janet"
	fmt.Printf("original jane: %+v\n", jane)
	fmt.Printf("jane copy:     %+v\n", janeCopy)
	janeCopy.Skills[0] = "Rust"
	fmt.Printf("original skills after copied slice change: %v\n", jane.Skills)

	// Duplicate the slice when this field should have independent element storage.
	independentJane := cloneEmployee(jane)
	independentJane.Skills[0] = "Python"
	fmt.Printf("original skills after independent change: %v\n", jane.Skills)
	fmt.Printf("independent employee skills: %v\n", independentJane.Skills)

	/* Section 6: Pointer to a struct */
	fmt.Println("\n== 6. Pointer to a struct ==")

	/* Pointer to a struct */
	// Use a pointer when you want to modify the original struct.
	joePtr := &joe
	joePtr.IsActive = false
	joePtr.Position = "Lead Designer"
	fmt.Printf("joe after pointer update: %+v\n", joe)

	/* Section 7: Nested structs */
	fmt.Println("\n== 7. Nested structs ==")

	/* Nested structs */
	// Nested means "has a". An Employee has an Address.
	// Access nested fields through the field name: jane.Address.City.
	fmt.Printf("jane city: %s\n", jane.Address.City)
	jane.Address.City = "Guadalajara"
	fmt.Printf("jane after address update: %+v\n", jane)

	/* Section 8: Embedded structs */
	fmt.Println("\n== 8. Embedded structs ==")

	/* Embedded structs */
	// Embedded means "includes". A Manager includes Employee fields.
	// Promoted fields can be accessed directly: manager.FirstName.
	manager := Manager{
		Employee: joe,
		TeamSize: 5,
	}
	fmt.Printf("manager name: %s\n", manager.FirstName)
	fmt.Printf("manager team size: %d\n", manager.TeamSize)

	/* Section 9: Anonymous struct */
	fmt.Println("\n== 9. Anonymous struct ==")

	/* Anonymous struct */
	// Use it when you need a small struct only in this place.
	// Common for temporary data shapes, test cases, or temporary JSON shapes.
	tmpReport := struct {
		Title string
		Count int
	}{
		Title: "Active employees",
		Count: 2,
	}
	fmt.Printf("report: %+v\n", tmpReport)

	/* Section 10: Struct tags */
	fmt.Println("\n== 10. Struct tags ==")

	/* Struct tags */
	// Tags add metadata to fields.
	// JSON tags can rename fields, skip empty fields, or ignore fields.
	apiEmployee := APIEmployee{
		ID:        1,
		FirstName: "Jane",
		LastName:  "Doe",
		Password:  "secret",
	}
	jsonData, err := json.Marshal(apiEmployee)
	if err != nil {
		fmt.Printf("json error: %v\n", err)
		return
	}
	fmt.Printf("api employee json: %s\n", jsonData)

	apiEmployee.Email = "jane@example.com"
	jsonData, err = json.Marshal(apiEmployee)
	if err != nil {
		fmt.Printf("json error: %v\n", err)
		return
	}
	fmt.Printf("api employee with email json: %s\n", jsonData)

	/* Section 11: Compare structs */
	fmt.Println("\n== 11. Compare structs ==")

	/* Compare structs */
	// Structs can be compared when all fields are comparable.
	// Employee is not comparable because its Skills field is a slice.
	badge1 := Badge{ID: 1, Level: "gold", Active: true}
	badge2 := Badge{ID: 1, Level: "gold", Active: true}
	fmt.Printf("badges are equal: %t\n", badge1 == badge2)

	/* Section 12: Final struct rules */
	fmt.Println("\n== 12. Final struct rules ==")

	// Use structs when fields are known when you write the code.
	// Use named fields in literals so the code stays clear when fields change.
	// Choose pointers deliberately for shared mutation, optional values, or API needs.
	fmt.Println("Structs keep related data together in one custom type.")
}
