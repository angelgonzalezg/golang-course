// Methods
// Topic: methods are functions attached to a named type by using a receiver.
// Args/Input: examples use Salary, Employee, and Department types.
// Usage: use methods when behavior belongs naturally to a type.
// Key idea: methods let values and pointers expose behavior with dot syntax.
package main

import "fmt"

/* Helper section 1: Types used by method examples */
// Salary is a named type, so Go allows us to attach methods to it.
// You cannot attach methods directly to built-in types like int or string.
// The receiver's base type must be defined in the same package as the method.
type Salary int

// An alias is another name for the same type; SalaryAlias has Salary's methods too.
type SalaryAlias = Salary

/* Method on a non-struct type */
// The receiver is the (s Salary) part.
func (s Salary) Monthly() Salary {
	// Inside the method, s is the Salary value.
	return s / 12
}

type Employee struct {
	ID        int
	FirstName string
	LastName  string
	Position  string
	Salary    Salary
	Active    bool
}

/* Helper section 2: Constructor-style helper */
/* Constructor-style function */
// Go does not have constructors, but New... functions are common.
func NewEmployee(id int, firstName, lastName, position string, salary Salary) Employee {
	return Employee{
		ID:        id,
		FirstName: firstName,
		LastName:  lastName,
		Position:  position,
		Salary:    salary,
		Active:    true,
	}
}

/* Helper section 3: Employee methods */
// This lesson mixes receiver styles to compare their behavior.
// For an application type with mutable state, prefer a consistent receiver style.
/* Value receiver: reads data */
// A value receiver is suitable for a small value that does not need shared mutation.
func (e Employee) FullName() string {
	// e is a copy of the Employee value.
	return e.FirstName + " " + e.LastName
}

/* Value receiver: formats data */
func (e Employee) Summary() string {
	status := "inactive"
	if e.Active {
		status = "active"
	}

	return fmt.Sprintf("#%d %s - %s (%s)", e.ID, e.FullName(), e.Position, status)
}

/* Value receiver: returns a changed copy */
// This does not update the original employee.
func (e Employee) WithPosition(position string) Employee {
	if position == "" {
		return e
	}

	// This changes only the receiver copy.
	e.Position = position
	return e
}

/* Pointer receiver: changes the original value */
// Use a pointer receiver when the method should update the struct.
func (e *Employee) Rename(firstName, lastName string) *Employee {
	if e == nil {
		return e
	}

	// e points to the original Employee.
	e.FirstName = firstName
	e.LastName = lastName
	return e
}

/* Pointer receiver: changes one field */
func (e *Employee) SetPosition(position string) *Employee {
	if e == nil || position == "" {
		return e
	}

	e.Position = position
	// Returning e makes method chaining possible.
	return e
}

/* Pointer receiver: changes a numeric field */
func (e *Employee) GiveRaise(amount Salary) *Employee {
	if e == nil || amount <= 0 {
		return e
	}

	e.Salary += amount
	return e
}

/* Pointer receiver: changes a boolean field */
func (e *Employee) Deactivate() {
	if e == nil {
		return
	}

	e.Active = false
}

/* Pointer receiver: changes a boolean field */
func (e *Employee) Reactivate() {
	if e == nil {
		return
	}

	e.Active = true
}

/* Pointer receiver: safely reads a possibly nil employee */
func (e *Employee) IsActive() bool {
	if e == nil {
		return false
	}

	return e.Active
}

/* Helper section 4: Embedded type method example */
type Department struct {
	Name string

	// Embedded field: Department gets Employee fields and methods promoted.
	// Department remains a distinct type; embedding provides composition.
	Employee
}

/* Method on a type with an embedded field */
func (d Department) Overview() string {
	return fmt.Sprintf("%s department led by %s", d.Name, d.FullName())
}

func main() {
	/* Section 1: Receiver basics */
	fmt.Println("== 1. Receiver basics ==")

	employee := NewEmployee(1, "Jane", "Doe", "Developer", 120000)

	// FullName is a method because it has a receiver.
	fmt.Println("Full name:", employee.FullName())

	// This is a regular function call.
	anotherEmployee := NewEmployee(2, "Ana", "Lopez", "Designer", 96000)
	fmt.Println("Another employee:", anotherEmployee.FullName())

	/* Section 2: Methods on non-struct types */
	fmt.Println("\n== 2. Methods on non-struct types ==")

	// Salary is a named int type, so it can have methods.
	fmt.Println("Yearly salary:", employee.Salary)
	fmt.Println("Monthly salary:", employee.Salary.Monthly())

	// A defined type is distinct from int, so conversion is explicit.
	var rawSalary int = int(employee.Salary)
	fmt.Printf("Converted salary: %d (%T)\n", rawSalary, rawSalary)
	// var rawSalary int = employee.Salary // Does not compile: Salary and int are distinct.

	// An alias preserves type identity and the existing method set.
	var aliasSalary SalaryAlias = employee.Salary
	fmt.Printf("Alias salary: %d (%T), monthly: %d\n", aliasSalary, aliasSalary, aliasSalary.Monthly())

	/* Section 3: Value receiver methods */
	fmt.Println("\n== 3. Value receiver methods ==")

	// Value receivers are good for reading or formatting data.
	fmt.Println("Summary:", employee.Summary())

	// A value receiver receives a copy.
	copiedEmployee := employee.WithPosition("Team Lead")
	fmt.Println("Original position:", employee.Position)
	fmt.Println("Copied position:", copiedEmployee.Position)

	/* Section 4: Pointer receiver methods */
	fmt.Println("\n== 4. Pointer receiver methods ==")

	// Pointer receivers can update the original value.
	employee.Rename("Jane", "Smith")
	employee.SetPosition("Senior Developer")
	employee.GiveRaise(10000)
	fmt.Println("After pointer updates:", employee.Summary())
	fmt.Println("New monthly salary:", employee.Salary.Monthly())

	/* Section 5: Automatic address and dereference */
	fmt.Println("\n== 5. Automatic address and dereference ==")

	// A pointer can call value receiver methods.
	employeePointer := &employee
	fmt.Println("Full name from pointer:", employeePointer.FullName())

	// An addressable value can call pointer receiver methods.
	employee.SetPosition("Staff Developer")
	fmt.Println("After automatic address:", employee.Position)

	// This would not compile because a temporary value is not addressable:
	// NewEmployee(3, "Temporary", "User", "Intern", 50000).SetPosition("Developer")

	/* Section 6: Method chaining */
	fmt.Println("\n== 6. Method chaining ==")

	// These methods return *Employee, so the next method can be called right away.
	employee.SetPosition("Principal Developer").GiveRaise(15000)
	fmt.Println("After chaining:", employee.Summary())

	/* Section 7: Nil pointer receivers */
	fmt.Println("\n== 7. Nil pointer receivers ==")

	// A pointer receiver can handle nil if the method checks for nil.
	var missingEmployee *Employee
	missingEmployee.Deactivate()
	missingEmployee.GiveRaise(1000)
	fmt.Println("Missing employee active:", missingEmployee.IsActive())

	/* Section 8: Method values */
	fmt.Println("\n== 8. Method values ==")

	// A method value stores the receiver and can be called later.
	raiseLater := employee.GiveRaise
	// raiseLater already knows which employee to update.
	raiseLater(5000)
	fmt.Println("After method value:", employee.Salary)

	// A value-receiver method value saves a copy; a pointer-receiver one saves the pointer.
	fullNameLater := employee.FullName
	renameLater := employee.Rename
	renameLater("Jane", "Taylor")
	fmt.Println("Saved value receiver name:", fullNameLater()) // Jane Smith.
	fmt.Println("Current employee name:", employee.FullName()) // Jane Taylor.

	/* Section 9: Method expressions */
	fmt.Println("\n== 9. Method expressions ==")

	// A method expression receives the receiver as its first argument.
	fullName := Employee.FullName
	// fullName needs an Employee argument.
	fmt.Println("From value receiver expression:", fullName(employee))

	giveRaise := (*Employee).GiveRaise
	// giveRaise needs a *Employee argument.
	giveRaise(&employee, 1000)
	fmt.Println("After pointer receiver expression:", employee.Salary)

	/* Section 10: Embedded types and promoted methods */
	fmt.Println("\n== 10. Embedded types and promoted methods ==")

	department := Department{
		Name:     "Engineering",
		Employee: employee,
	}

	// Department stores its own Employee value.
	// Department can call Employee methods because Employee is embedded.
	// Department is not assignable to Employee; access department.Employee when that type is needed.
	fmt.Println("Department leader:", department.FullName())
	fmt.Println("Department overview:", department.Overview())

	// Pointer receiver methods are promoted too when the value is addressable.
	department.GiveRaise(3000)
	fmt.Println("Department leader salary:", department.Salary)
	fmt.Println("Original employee salary:", employee.Salary)

	/* Section 11: Final method rule */
	fmt.Println("\n== 11. Final method rule ==")

	// Use methods when behavior belongs to a type.
	// Examples: employee.FullName(), salary.Monthly(), account.Deposit().
	// Methods make code read like the value is doing the action.
	// Use value receivers for read-only behavior and small immutable-style updates.
	// Use pointer receivers when the method must update the original value.
	// If one method needs a pointer receiver, it is common to use pointer receivers consistently.
	fmt.Println("Methods attach behavior to types; choose value or pointer receivers intentionally.")
}
