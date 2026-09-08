// Maps
// Topic: maps store key/value pairs for fast lookup by key.
// Args/Input: examples use string keys and int values for student grades and config values.
// Usage: add, update, read, check existence, delete, and understand nil vs initialized maps.
// Key idea: the comma-ok idiom tells if a key exists separately from its zero value.
package main

import (
	"fmt"
	"maps"
)

/* Helper section 1: Map parameter helpers */
/* Map input parameter */
// Use a map parameter when the caller has flexible key/value data.
// This is common for config, request metadata, labels, counters, and lookup tables.
func printLabels(labels map[string]string) {
	for key, value := range labels {
		fmt.Printf("%s=%s\n", key, value)
	}
}

/* Map parameter can modify the caller's map */
// The map value is copied, but both copies refer to the same map data.
// Changing a key/value inside the function is visible to the caller.
func setLabel(labels map[string]string, key string, value string) {
	labels[key] = value
}

/* Helper section 2: Map return helper */
/* Map return value */
// Return a map when the keys are discovered at runtime.
// A struct would require the field names to be known when writing the code.
func countWords(words []string) map[string]int {
	counts := map[string]int{}

	for _, word := range words {
		counts[word]++
	}

	return counts
}

func main() {
	/* Section 1: Create a map */
	fmt.Println("== 1. Create a map ==")

	/* Create a map */
	studentGrades := map[string]int{
		"Alice": 90,
		"James": 85,
		"Dan":   60,
	}
	fmt.Printf("initial grades: %+v\n", studentGrades)

	/* Section 2: Nil map vs empty map */
	fmt.Println("\n== 2. Nil map vs empty map ==")

	/* Nil map vs empty map */
	// Use nil when the map may stay empty or be initialized later.
	// Use make or a literal when you need to write values now.
	var nilMap map[string]int       // nil map: zero value, not ready for writes.
	makeMap := make(map[string]int) // empty map: ready for writes.
	literalMap := map[string]int{}  // empty map: ready for writes.

	fmt.Printf("nilMap is nil: %t\n", nilMap == nil)
	fmt.Printf("makeMap is nil: %t\n", makeMap == nil)
	fmt.Printf("literalMap is nil: %t\n", literalMap == nil)

	// Initialize a nil map with make before writing to it.
	nilMap = make(map[string]int)
	nilMap["port"] = 8080

	makeMap["timeout"] = 30
	literalMap["retries"] = 3
	fmt.Printf("nilMap after make: %+v\n", nilMap)
	fmt.Printf("makeMap: %+v\n", makeMap)
	fmt.Printf("literalMap: %+v\n", literalMap)

	/* Section 3: Add, update, read, and delete */
	fmt.Println("\n== 3. Add, update, read, and delete ==")

	/* Update a value */
	studentGrades["Alice"] = 95
	fmt.Printf("after update: %+v\n", studentGrades)

	/* Add a new key/value pair */
	studentGrades["Luke"] = 100
	fmt.Printf("after add: %+v\n", studentGrades)

	/* Read a value and check if the key exists */
	grade, ok := studentGrades["Alice"]
	if ok {
		fmt.Printf("Alice grade: %d\n", grade)
	}

	// Reading a missing key returns the value type's zero value.
	// Use comma-ok when zero could also be a real stored value.
	missingGrade := studentGrades["Bob"]
	fmt.Printf("Bob grade without ok: %d\n", missingGrade)

	/* Check only if a key exists */
	if grade, ok := studentGrades["Bob"]; ok {
		fmt.Printf("Bob grade: %d\n", grade)
	} else {
		fmt.Printf("Bob does not exist\n")
	}

	key := "Dan"
	if grade, ok := studentGrades[key]; ok {
		fmt.Printf("%s grade: %d\n", key, grade)
	} else {
		fmt.Printf("%s does not exist\n", key)
	}

	/* Delete a key/value pair */
	delete(studentGrades, key)
	fmt.Printf("after delete: %+v\n", studentGrades)

	// Deleting a missing key is safe.
	delete(studentGrades, "Missing Student")
	fmt.Printf("after deleting missing key: %+v\n", studentGrades)

	/* Section 4: Map length and range */
	fmt.Println("\n== 4. Map length and range ==")

	/* Map length and range */
	// Map order is not guaranteed.
	fmt.Printf("grades count: %d\n", len(studentGrades))
	for name, grade := range studentGrades {
		fmt.Printf("%s: %d\n", name, grade)
	}

	/* Section 5: Map parameters and return values */
	fmt.Println("\n== 5. Map parameters and return values ==")

	/* Map parameters and return values */
	// A struct is best when field names are known at compile time:
	// type Config struct { Host string; Port string }
	//
	// A map is best when keys are not known until the program runs.
	// Example: labels can come from a file, API request, or user input.
	labels := map[string]string{
		"environment": "development",
		"region":      "us-east",
		"feature":     "payments",
	}
	fmt.Println("labels:")
	printLabels(labels)

	// A function can change key/value pairs in the caller's map.
	setLabel(labels, "feature", "checkout")
	setLabel(labels, "owner", "platform")
	fmt.Println("labels after setLabel:")
	printLabels(labels)

	// The function returns a map because the words become keys at runtime.
	wordCounts := countWords([]string{"go", "map", "go", "slice", "map", "go"})
	fmt.Printf("word counts: %+v\n", wordCounts)

	/* Section 6: Map assignment shares data */
	fmt.Println("\n== 6. Map assignment shares data ==")

	/* Map assignment shares data */
	originalGrades := map[string]int{"Alice": 90, "James": 85, "Curtis": 70}
	fmt.Printf("original before shared change: %+v\n", originalGrades)
	sharedGrades := originalGrades
	sharedGrades["Alice"] = 100
	fmt.Printf("original after shared change:  %+v\n", originalGrades)
	fmt.Printf("shared after shared assign:    %+v\n", sharedGrades)

	/* Section 7: Clone a map */
	fmt.Println("\n== 7. Clone a map ==")

	/* Clone a map to avoid sharing data */
	clonedGrades := maps.Clone(originalGrades) // maps package go 1.21+
	clonedGrades["Alice"] = 70
	fmt.Printf("original after clone change: %+v\n", originalGrades)
	fmt.Printf("clone after change:          %+v\n", clonedGrades)

	/* Older option: make a map, then copy each key/value */
	// make(map[key_type]val_type, cap)
	manualClone := make(map[string]int, len(originalGrades))
	for name, grade := range originalGrades {
		manualClone[name] = grade
	}
	manualClone["Alice"] = 80
	fmt.Printf("original after manual clone change: %+v\n", originalGrades)
	fmt.Printf("manual clone after change:          %+v\n", manualClone)

	/* Section 8: Delete and clear */
	fmt.Println("\n== 8. Delete and clear ==")

	/* Delete one map entry */
	delete(clonedGrades, "Alice")
	fmt.Printf("clonedGrades after deleting Alice: %+v\n", clonedGrades)

	/* Clear all map entries */
	clear(clonedGrades)
	fmt.Printf("clonedGrades after clear: %+v\n", clonedGrades)

	/* Section 9: Compare maps */
	fmt.Println("\n== 9. Compare maps ==")

	/* Compare maps */
	// maps.Equal checks if both maps have the same keys and values.
	firstMap := map[string]int{"Alice": 90, "James": 85}
	secondMap := map[string]int{"James": 85, "Alice": 90}
	thirdMap := map[string]int{"Alice": 90, "James": 70}
	fmt.Printf("first equals second: %t\n", maps.Equal(firstMap, secondMap))
	fmt.Printf("first equals third:  %t\n", maps.Equal(firstMap, thirdMap))

	/* Section 10: Use a map as a set */
	fmt.Println("\n== 10. Use a map as a set ==")

	/* Use a map as a set */
	// The key is the item. true means the item is in the set.
	// bool is simple to read; a missing key returns false.
	visitedCitySet1 := map[string]bool{}
	cities := []string{"Paris", "Berlin", "Tokyo"}
	for _, city := range cities {
		visitedCitySet1[city] = true
	}

	visitedCitySet1["Lima"] = true

	fmt.Printf("Visited Kyoto?: %+v\n", visitedCitySet1["Kyoto"])

	delete(visitedCitySet1, "Berlin")
	fmt.Printf("visited cities: %+v\n", visitedCitySet1)

	/* Section 11: Use struct{} for larger sets */
	fmt.Println("\n== 11. Use struct{} for larger sets ==")

	/* Use a map as a set with struct{} */
	// struct{} stores no value data, so it is common for large sets.
	// empty struct{} uses zero bytes, while bool uses 1 byte.
	visitedCitySet2 := map[string]struct{}{}
	for _, city := range cities {
		visitedCitySet2[city] = struct{}{}
	}

	visitedCitySet2["Lima"] = struct{}{}

	// With struct{} values, use comma-ok to check membership.
	if _, ok := visitedCitySet2["Kyoto"]; ok {
		fmt.Printf("Kyoto was visited\n")
	} else {
		fmt.Printf("Kyoto was not visited\n")
	}

	delete(visitedCitySet2, "Berlin")
	fmt.Printf("visited cities set: %+v\n", visitedCitySet2)

	/* Section 12: Map keys must be comparable */
	fmt.Println("\n== 12. Map keys must be comparable ==")

	// Strings, numbers, booleans, and arrays can be map keys.
	// Slices, maps, and functions cannot be map keys.
	coordinates := map[[2]int]string{
		{0, 0}: "origin",
		{1, 2}: "point A",
	}
	fmt.Printf("coordinates: %+v\n", coordinates)

	/* Section 13: Final map rules */
	fmt.Println("\n== 13. Final map rules ==")

	// Use maps for key/value lookup when keys are known at runtime.
	// Use comma-ok when a missing key and a zero value must be different.
	// Clone a map when the caller should not share changes.
	fmt.Println("Maps are great for fast lookup by key.")
}
