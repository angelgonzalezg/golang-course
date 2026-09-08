// Interfaces
// Topic: interfaces describe behavior by listing method signatures.
// Args/Input: examples use employees, customers, accounts, notifications, and any.
// Usage: write functions that depend on behavior instead of concrete types.
// Key idea: implementation is implicit; no "implements" keyword is needed in Go.
package main

import "fmt"

/* Helper section 1: Interface definitions */
type Namer interface {
	// Any type with this method satisfies Namer.
	Name() string
}

type Describer interface {
	// Any type with this method satisfies Describer.
	Description() string
}

/* Interface embedding: combines smaller interfaces */
type Profile interface {
	Namer
	Describer
}

/* Helper section 2: Concrete types */
type Employee struct {
	ID       int
	FullName string
	Position string
}

type Customer struct {
	ID       int
	FullName string
	Plan     string
}

type Robot struct {
	Serial string
	Model  string
}

/* Helper section 3: Methods that satisfy interfaces */
/* Employee methods */
// These methods make Employee satisfy Namer and Describer.
func (e Employee) Name() string {
	return e.FullName
}

func (e Employee) Description() string {
	return fmt.Sprintf("%s, %s", e.FullName, e.Position)
}

/* Customer methods */
// These methods make Customer satisfy Namer and Describer.
func (c Customer) Name() string {
	return c.FullName
}

func (c Customer) Description() string {
	return fmt.Sprintf("%s, %s customer", c.FullName, c.Plan)
}

/* Robot method */
// Robot satisfies Namer only because it has Name(), but not Description().
func (r Robot) Name() string {
	return r.Serial
}

type Account struct {
	Owner        string
	BalanceCents int
}

type Depositer interface {
	Deposit(amountCents int)
}

/* Pointer receiver method */
// Only *Account satisfies Depositer because Deposit has a pointer receiver.
func (a *Account) Deposit(amountCents int) {
	if a == nil || amountCents <= 0 {
		return
	}

	a.BalanceCents += amountCents
}

type Notifier interface {
	Notify()
}

type EmailNotifier struct {
	Address string
}

func (e *EmailNotifier) Notify() {
	if e == nil {
		fmt.Println("No email notifier configured.")
		return
	}

	fmt.Println("Sending email to:", e.Address)
}

/* Helper section 4: Interface helper functions */
func printName(value Namer) {
	// The function depends on behavior, not the concrete type.
	fmt.Println("Name:", value.Name())
}

func printProfile(value Profile) {
	fmt.Println("Profile:", value.Description())
}

func addDeposit(value Depositer, amountCents int) {
	// This function only needs something that can Deposit.
	value.Deposit(amountCents)
}

func printUnknown(value any) {
	// any is an alias for interface{}.
	fmt.Printf("value: %v, concrete type: %T\n", value, value)
}

func describeUnknown(value any) {
	// A type switch checks the concrete type inside the interface.
	switch concreteValue := value.(type) {
	case string:
		fmt.Println("string length:", len(concreteValue))
	case int:
		fmt.Println("int doubled:", concreteValue*2)
	case Employee:
		fmt.Println("employee from type switch:", concreteValue.Name())
	default:
		fmt.Printf("unhandled type: %T\n", concreteValue)
	}
}

func main() {
	/* Section 1: Interface basics */
	fmt.Println("== 1. Interface basics ==")

	employee := Employee{ID: 1, FullName: "Jane Doe", Position: "Developer"}
	customer := Customer{ID: 10, FullName: "Ana Lopez", Plan: "premium"}

	// Employee and Customer both satisfy Namer because both have Name() string.
	printName(employee)
	printName(customer)

	/* Section 2: Implicit implementation */
	fmt.Println("\n== 2. Implicit implementation ==")

	// There is no "implements" keyword in Go.
	// If a type has the required methods, it satisfies the interface.
	robot := Robot{Serial: "R-1000", Model: "delivery"}
	printName(robot)

	/* Section 3: Interface values */
	fmt.Println("\n== 3. Interface values ==")

	var namedValue Namer
	namedValue = employee
	fmt.Printf("namedValue holds type %T with name %q\n", namedValue, namedValue.Name())

	namedValue = customer
	fmt.Printf("namedValue now holds type %T with name %q\n", namedValue, namedValue.Name())

	/* Section 4: Small interfaces */
	fmt.Println("\n== 4. Small interfaces ==")

	// Small interfaces are easier to satisfy and reuse.
	names := []Namer{employee, customer, robot}
	for _, value := range names {
		fmt.Println("listed name:", value.Name())
	}

	/* Section 5: Interface embedding */
	fmt.Println("\n== 5. Interface embedding ==")

	// Profile requires both Name() and Description().
	printProfile(employee)
	printProfile(customer)

	// Robot has Name(), but not Description(), so this would not compile:
	// printProfile(robot)

	/* Section 6: Pointer receivers and interfaces */
	fmt.Println("\n== 6. Pointer receivers and interfaces ==")

	account := Account{Owner: "Jane Doe", BalanceCents: 5000}

	// Deposit has a pointer receiver, so pass &account.
	addDeposit(&account, 2500)
	fmt.Println("Account after deposit:", account)

	// This would not compile because Account does not satisfy Depositer:
	// addDeposit(account, 2500)

	/* Section 7: Empty interface and any */
	fmt.Println("\n== 7. Empty interface and any ==")

	// any can hold a value of any type.
	printUnknown("hello")
	printUnknown(42)
	printUnknown(employee)

	/* Section 8: Type assertions */
	fmt.Println("\n== 8. Type assertions ==")

	var unknown any = "Go"

	// value.(string) asks: is the concrete value a string?
	text, ok := unknown.(string)
	if ok {
		fmt.Println("type assertion succeeded:", text)
	}

	// The ok value prevents a panic when the type is different.
	number, ok := unknown.(int)
	if !ok {
		fmt.Println("type assertion failed; value is not an int:", number)
	}

	/* Section 9: Type switches */
	fmt.Println("\n== 9. Type switches ==")

	describeUnknown("interfaces")
	describeUnknown(21)
	describeUnknown(employee)
	describeUnknown(3.14)

	/* Section 10: Nil interfaces */
	fmt.Println("\n== 10. Nil interfaces ==")

	var notifier Notifier
	fmt.Println("notifier is nil:", notifier == nil)

	var emailNotifier *EmailNotifier
	notifier = emailNotifier

	// Now the interface has a concrete type, even though the concrete value is nil.
	fmt.Println("notifier with nil concrete value is nil:", notifier == nil)
	notifier.Notify()

	// Assign a real EmailNotifier value to satisfy and use the interface.
	notifier = &EmailNotifier{Address: "jane@example.com"}
	fmt.Println("notifier with real concrete value is nil:", notifier == nil)
	notifier.Notify()

	/* Section 11: Prefer interfaces at boundaries */
	fmt.Println("\n== 11. Prefer interfaces at boundaries ==")

	// Common Go style:
	// - accept interfaces when a function only needs behavior
	// - return concrete types when creating values
	fmt.Println("printName accepts an interface because it only needs Name().")
	fmt.Println("Employee and Customer stay concrete types.")

	/* Section 12: Final interface rule */
	fmt.Println("\n== 12. Final interface rule ==")

	// Use interfaces when different concrete types should be used the same way.
	// Examples: anything with Name(), anything that can Deposit(), anything that can Notify().
	// Interfaces let functions depend on behavior instead of exact struct types.
	// Keep interfaces small.
	// Define interfaces near the code that uses the behavior.
	// Use 'any' only when the type is truly unknown.
	fmt.Println("Interfaces describe behavior; concrete types store data.")
}
