// Maps
// Topic: maps store key/value pairs for fast lookup by key.
// Args/Input: examples use string keys and int values for student grades and config values.
// Usage: add, update, read, check existence, delete, and understand nil vs initialized maps.
// Key idea: the comma-ok idiom tells if a key exists separately from its zero value.
package main

import "fmt"

func main() {

	studentGrades := map[string]int{ // Map literal
		"Alice": 90,
		"James": 85,
		"Dan":   60,
	}

	fmt.Printf("%+v\n", studentGrades)
	studentGrades["Alice"] = 95 // Update existing value
	fmt.Printf("%+v\n", studentGrades)

	/* Add a new key/value to map */
	studentGrades["Luke"] = 100
	fmt.Printf("%+v\n", studentGrades)

	/* Validate if a key exists */
	val, ok := studentGrades["Alice"]
	if ok {
		fmt.Printf("Alice: %+v\n", val)
	}

	/* Cleaner key existence check */
	if _, ok := studentGrades["Bob"]; ok {
		fmt.Printf("Bob: %+v\n", studentGrades)
	} else {
		fmt.Printf("Bob(key) does not exists!\n")
	}

	key := "Dan"
	if _, ok := studentGrades[key]; ok {
		fmt.Printf("%s: %+v\n", key, studentGrades)
	} else {
		fmt.Printf("%s(key) does not exists!\n", key)
	}

	/* Delete a key/value from map */
	delete(studentGrades, key)
	fmt.Printf("%+v\n", studentGrades)

	var config1 map[string]int      // is NIL by default
	config2 := make(map[string]int) // is not NIL because memory is initialized with make
	config3 := map[string]int{}     // is not NIL because memory is initialized with ':='

	fmt.Printf("%+v %T\n", config1, config1)

	if config1 == nil {
		fmt.Printf("Config is nil\n")
	} else {
		fmt.Printf("%+v\n", config1)
	}

	if config2 == nil {
		fmt.Printf("Config2 is nil\n")
	} else {
		fmt.Printf("%+v\n", config2)
	}

	if config3 == nil {
		fmt.Printf("Config3 is nil\n")
	} else {
		fmt.Printf("%+v\n", config3)
	}

}
