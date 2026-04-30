// Pointers
// Topic: pointers store memory addresses and allow changing original values.
// Args/Input: modifyValue receives a copy; modifyPointer receives an *int address.
// Usage: use & to get an address and * to dereference/read or update the value.
// Key idea: passing a pointer lets code modify the original value instead of a copy.
package main

import "fmt"

func modifyValue(val int) {
	val = val * 10 // Modifies only the local copy
	fmt.Printf("modifyValue: %+v\n", val)
}

func modifyPointer(val *int) {
	if val == nil { // Check before dereferencing
		fmt.Printf("val is nil!\n")
		return
	}
	*val = *val * 10 // Dereference and modify original value
	fmt.Printf("modifyPointer: %+v\n", val)
}

func main() {
	age := 10
	agePrt := &age // & gets the address of age
	fmt.Printf("age: %d\n", &age)
	fmt.Printf("age: %d\n", agePrt)

	num := 10
	modifyValue(num) // Pass by value
	fmt.Println(num)

	modifyPointer(&num) // Pass address
	fmt.Println(num)

	grade := 50
	gradePtr := &grade // Pointer to grade

	fmt.Printf("gradePtr grade: %+v\n", gradePtr)
	fmt.Printf("gradePtr: %+v\n", *(&gradePtr))
}
