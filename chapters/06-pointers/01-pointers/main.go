// Pointers
// Topic: pointers store memory addresses and let code read or update original values.
// Args/Input: examples use ints, strings, structs, maps, slices, JSON, nil pointers, and new().
// Usage: compare value parameters and pointer parameters before choosing one.
// Key idea: start with values; choose pointers intentionally for shared mutation, identity, nil, or API needs.
// & (address operator) gets an address, * (indirection operator) reads or updates the value at that address.
package main

import (
	"encoding/json"
	"fmt"
)

/* Helper section 1: Example data types */
type User struct {
	Name string
	Age  int
}

type CartItem struct {
	ID         int
	Quantity   int
	PriceCents int
	Active     bool
}

type Profile struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type APIProfile struct {
	Name     string  `json:"name"`
	Nickname *string `json:"nickname"`
	Age      *int    `json:"age"`
}

type InternalProfile struct {
	Name        string
	Nickname    string
	HasNickname bool
	Age         int
	HasAge      bool
}

/* Helper section 2: Value and pointer parameter helpers */
/* Value parameter: the function receives a copy */
func changeCopy(score int) {
	score = 100
	fmt.Println("Inside changeCopy:", score)
}

/* Pointer parameter: the function receives an address */
func changeOriginal(score *int) {
	if score == nil {
		fmt.Println("Cannot update a nil pointer.")
		return
	}

	*score = 100
}

/* Pointer parameter with strings */
// This demonstrates mutation through a pointer.
// In normal code, returning the new string is usually clearer.
func updateName(name *string) {
	if name == nil {
		return
	}

	*name = "Ana"
}

/* Pointer parameter with float64 */
// This demonstrates mutation through a pointer.
// In normal code, returning the new price is usually clearer.
func applyDiscount(price *float64, percent float64) {
	if price == nil {
		return
	}

	*price = *price - (*price * percent / 100)
}

/* Helper section 3: Value vs pointer guidance */
/* Value parameters vs pointer parameters */
// Value parameter:
// - Receives a copy of the passed value, including any references it contains.
// - Reassigning the parameter does not replace the caller's variable.
// - Referenced data may still be shared, as the map and slice examples below show.
// - Usually best for small structs and read-only data.
//
// Pointer parameter:
// - Receives a copy of an address; Go still passes the pointer by value.
// - Lets the function modify the value at that address or share an object's identity.
// - Can represent a missing value with nil or satisfy an API's mutation contract.
// - May avoid copying a large value; measure when performance matters.
//
// Taking an address does not automatically allocate on the heap.
// The compiler's escape analysis and optimizations determine where storage is needed.
// For return values, T copies its fields; *T can refer to existing or fresh data.
// Document which object is shared, who may change it, and whether nil is valid.

/* Value parameter with a struct: returns a modified copy */
// Choose this for a small result that the caller can edit independently.
// CartItem contains only scalar fields, so copying it needs no nested cloning.
// The compiler may avoid heap allocation; a value return alone does not guarantee it.
func updateCartItemValue(item CartItem, quantity int) CartItem {
	if quantity <= 0 {
		return item
	}

	item.Quantity = quantity
	return item
}

/* Pointer parameter with a struct: modifies the original */
// Use this form when the operation should update the caller's existing item.
// Returning an updated value is another choice when a separate result is wanted.
// This helper creates no new item. Passing &item alone does not force heap storage.
func updateCartItemPointer(item *CartItem, quantity int) {
	if item == nil || quantity <= 0 {
		return
	}

	item.Quantity = quantity
}

/* Pointer to a struct */
func celebrateBirthday(user *User) {
	if user == nil {
		return
	}

	user.Age++ // Same as (*user).Age++, but easier to read.
}

/* When to pass pointers to functions */
// Start with values and returned results for small, independent data.
// Use a pointer when shared mutation, identity, optional data, or an API contract needs it.

/* Helper section 4: JSON and no-value helpers */
/* Pointer passed through interface/any */
// json.Unmarshal requires a non-nil pointer so it can populate the destination.
// Its any parameter lets it accept different destination types; any itself does not require a pointer.
func decodeProfile(jsonText string, destination any) error {
	// A Profile value or a nil *Profile is rejected with an InvalidUnmarshalError.
	return json.Unmarshal([]byte(jsonText), destination)
}

func printAPIProfile(profile APIProfile) {
	fmt.Println("Name:", profile.Name)

	if profile.Nickname == nil {
		fmt.Println("Nickname: no value")
	} else {
		fmt.Printf("Nickname: %q\n", *profile.Nickname)
	}

	if profile.Age == nil {
		fmt.Println("Age: no value")
	} else {
		fmt.Println("Age:", *profile.Age)
	}
}

func printInternalProfile(profile InternalProfile) {
	fmt.Println("Name:", profile.Name)

	if profile.HasNickname {
		fmt.Printf("Nickname: %q\n", profile.Nickname)
	} else {
		fmt.Println("Nickname: no value")
	}

	if profile.HasAge {
		fmt.Println("Age:", profile.Age)
	} else {
		fmt.Println("Age: no value")
	}
}

/* Helper section 5: Map helper functions */
/* Count a visit by updating the caller's shared map data */
func recordVisit(visits map[string]int, page string) {
	if visits == nil {
		return
	}

	visits[page]++
}

/* Return an updated copy when the caller's original map should stay unchanged */
func recordVisitOnCopy(visits map[string]int, page string) map[string]int {
	copiedVisits := make(map[string]int, len(visits)+1)

	for key, value := range visits {
		copiedVisits[key] = value
	}

	copiedVisits[page]++
	return copiedVisits
}

/* Set a specific key to show that map parameters can modify shared map data */
func updateVisitsValue(visits map[string]int) {
	visits["contact"] = 2
	fmt.Println("Visits inside updateVisitsValue:", visits)
}

/* Assigning a new map to a map parameter does not replace the caller's map */
func replaceVisitsValue(visits map[string]int) {
	visits = map[string]int{
		"inside function": 1,
	}
	fmt.Println("Visits inside replaceVisitsValue:", visits)
}

/* Use a pointer to a map only when replacing the map itself */
// Returning a new map is often clearer; this helper demonstrates replacing the caller's variable.
func replaceVisitsPointer(visits *map[string]int) {
	if visits == nil {
		return
	}

	*visits = map[string]int{
		"home":  1,
		"about": 1,
	}
}

/* Helper section 6: Slice helper functions */
/* Slice values already share their backing array */
// The slice value is copied, but both copies can point to the same backing array.
// Changing an element inside the function is visible to the caller.
func updateFirstScore(scores []int) {
	if len(scores) == 0 {
		return
	}

	scores[0] = 100
	fmt.Println("Scores inside updateFirstScore:", scores)
}

/* Copy a slice before changing it when the caller's slice should stay unchanged */
func updateFirstScoreOnCopy(scores []int) []int {
	copiedScores := make([]int, len(scores))
	copy(copiedScores, scores)

	if len(copiedScores) == 0 {
		return copiedScores
	}

	copiedScores[0] = 100
	return copiedScores
}

/* Assigning a new slice to a slice parameter does not replace the caller's slice */
func replaceScoresValue(scores []int) {
	scores = []int{50, 60, 70}
	fmt.Println("Scores inside replaceScoresValue:", scores)
}

/* Use a pointer to a slice only when replacing the slice itself */
// Returning a new slice is often clearer; this helper demonstrates replacing the caller's variable.
func replaceScores(scores *[]int) {
	if scores == nil {
		return
	}

	*scores = []int{95, 90, 85}
}

/* Slice as a buffer */
func writeToBuffer(buffer []byte, message string) int {
	// copy writes into the existing buffer and returns how many bytes were copied.
	return copy(buffer, message)
}

func main() {
	/* Section 1: Pointer basics */
	fmt.Println("== 1. Address and dereference ==")

	age := 21
	// &age gets the memory address of age.
	agePointer := &age

	fmt.Println("Age value:", age)
	fmt.Printf("Age address: %p\n", agePointer)
	// *agePointer reads the value stored at that address.
	fmt.Println("Age through pointer:", *agePointer)

	// *agePointer updates the original age variable.
	*agePointer = 22
	fmt.Println("Age after pointer update:", age)

	/* Section 1.1: Compare and reassign pointers */
	fmt.Println("\n== 1.1. Compare and reassign pointers ==")

	// Pointers can be compared. Same address means same variable.
	sameAgePointer := &age
	anotherAge := age
	anotherAgePointer := &anotherAge

	fmt.Println("agePointer == sameAgePointer:", agePointer == sameAgePointer)
	fmt.Println("agePointer == anotherAgePointer:", agePointer == anotherAgePointer)
	fmt.Println("agePointer is not nil:", agePointer != nil)

	// A pointer variable can point to another variable of the same type.
	agePointer = &anotherAge
	*agePointer = 30
	fmt.Println("Age after pointer reassignment:", age)
	fmt.Println("anotherAge after pointer reassignment:", anotherAge)

	/* Section 2: Nil pointers and new */
	fmt.Println("\n== 2. Nil pointers and new ==")

	// A nil pointer points to nothing.
	// It cannot be dereferenced or updated safely.
	var missingAge *int
	fmt.Println("missingAge is nil:", missingAge == nil)
	changeOriginal(missingAge)

	// new(int) creates an int with zero value and returns its address.
	countPointer := new(int) // new(int) returns *int with value 0.
	*countPointer = 5
	fmt.Println("Count through new pointer:", *countPointer)

	/* Section 3: Value parameters vs pointer parameters */
	fmt.Println("\n== 3. Value parameters vs pointer parameters ==")

	score := 80
	// changeCopy receives a copy, so score does not change.
	changeCopy(score)
	fmt.Println("After changeCopy:", score)

	// changeOriginal receives &score, so it can update score.
	changeOriginal(&score)
	fmt.Println("After changeOriginal:", score)

	/* Section 4: Simple pointer mechanics */
	fmt.Println("\n== 4. Simple pointer mechanics ==")

	// These examples show how pointers work.
	// Prefer returning values for regular business logic.
	name := "Ben"
	updateName(&name)
	fmt.Println("Updated name:", name)

	price := 100.0
	applyDiscount(&price, 15)
	fmt.Println("Discounted price:", price)

	/* Section 5: Structs and pointers */
	fmt.Println("\n== 5. Structs: return a value or update through a pointer ==")

	item := CartItem{ID: 1, Quantity: 2, PriceCents: 1500, Active: true}
	// Returning a changed copy keeps the original value unchanged.
	updatedItem := updateCartItemValue(item, 5)
	fmt.Printf("Original item: %+v\n", item)
	fmt.Printf("Updated copy:  %+v\n", updatedItem)

	// Passing a pointer lets the function update the original struct
	updateCartItemPointer(&item, 10)
	fmt.Printf("Item after pointer update: %+v\n", item)

	user := User{Name: "Ana", Age: 30}
	userPointer := &user
	userPointer.Name = "Ana Gonzalez" // Go lets you access fields without writing (*userPointer).Name.
	celebrateBirthday(&user)
	fmt.Printf("Updated user: %+v\n", user)

	/* Section 6: Pointers through interface/any */
	fmt.Println("\n== 6. Pointer passed through interface/any ==")

	var profile Profile
	// Pass &profile to satisfy json.Unmarshal's destination contract.
	err := decodeProfile(`{"name":"Mia","age":27}`, &profile)
	if err != nil {
		fmt.Println("Decode error:", err)
	} else {
		fmt.Printf("Decoded profile: %+v\n", profile)
	}

	// Passing the struct value is allowed by any, but rejected by json.Unmarshal.
	err = decodeProfile(`{"name":"Other","age":99}`, profile)
	if err != nil {
		fmt.Println("Value destination rejected:", err)
	}
	fmt.Printf("Profile after rejected destination: %+v\n", profile)

	/* Section 7: Zero value vs no value */
	fmt.Println("\n== 7. Zero value vs no value ==")

	// Empty string and 0 are valid zero values.
	// Pointer fields are useful at JSON/API boundaries when null means "no value".
	var apiProfileWithZero APIProfile
	err = decodeProfile(`{"name":"Baby","nickname":"","age":0}`, &apiProfileWithZero)
	if err != nil {
		fmt.Println("Decode error:", err)
	} else {
		fmt.Println("JSON profile with zero values:")
		printAPIProfile(apiProfileWithZero)
	}

	var apiProfileWithNull APIProfile
	// JSON null becomes nil for pointer fields.
	// On a fresh struct, an omitted field is also nil; a pointer alone cannot distinguish the two.
	// On a reused struct, omitted fields keep their previous values, while null clears the pointer.
	err = decodeProfile(`{"name":"Guest","nickname":null,"age":null}`, &apiProfileWithNull)
	if err != nil {
		fmt.Println("Decode error:", err)
	} else {
		fmt.Println("JSON profile with null values:")
		printAPIProfile(apiProfileWithNull)
	}

	// Away from JSON or external protocols, value fields plus bools are often clearer than pointers.
	internalProfile := InternalProfile{
		Name:        "Baby",
		Nickname:    "",
		HasNickname: true,
		Age:         0,
		HasAge:      true,
	}
	fmt.Println("Internal profile:")
	printInternalProfile(internalProfile)

	/* Section 8: Maps and pointers */
	fmt.Println("\n== 8. Maps and pointers ==")

	// Reminder from maps:
	// a map value points to runtime-managed map data.
	// Passing a map copies that value, not the whole map data.
	visits := map[string]int{
		"home": 1,
	}
	fmt.Println("Visits before function update:", visits)

	// Go passes the map value by copy.
	// The copied map value still points to the same map data, so no *map is needed here.
	// Changing a key/value through that copy changes the caller's map data.
	recordVisit(visits, "home")
	recordVisit(visits, "contact")
	fmt.Println("Visits after function update:", visits)

	// Setting a key/value inside the function also changes the caller's map data.
	updateVisitsValue(visits)
	fmt.Println("Visits after key/value update:", visits)

	// Copy the map first when the function should not modify the caller's map.
	visitsCopy := recordVisitOnCopy(visits, "pricing")
	fmt.Println("Visits after copy update:", visits)
	fmt.Println("Copied visits:", visitsCopy)

	// The map parameter is still a copy, so assigning a new map does not replace the caller's map.
	replaceVisitsValue(visits)
	fmt.Println("Visits after value replace attempt:", visits)

	// Use *map only when replacing the map itself.
	replaceVisitsPointer(&visits)
	fmt.Println("Visits after map replace:", visits)

	/* Section 9: Slices and pointers */
	fmt.Println("\n== 9. Slices and pointers ==")

	// Reminder from slices:
	// a slice value has a pointer, len, and cap.
	// Passing a slice copies that value, not the backing array.
	scores := []int{70, 80, 90}
	fmt.Println("Scores before function update:", scores)

	// Go passes the slice value by copy.
	// The copied slice value still points to the same backing array, so no *[]int is needed here.
	// Changing an element through that copy changes the caller's backing array.
	updateFirstScore(scores)
	fmt.Println("Scores after function update:", scores)

	// Copy the slice first when the function should not modify the caller's slice.
	scoresForCopy := []int{70, 80, 90}
	copiedScores := updateFirstScoreOnCopy(scoresForCopy)
	fmt.Println("Scores after copy update:", scoresForCopy)
	fmt.Println("Copied scores:", copiedScores)

	// The slice parameter is still a copy, so assigning a new slice does not replace the caller's slice.
	replaceScoresValue(scores)
	fmt.Println("Scores after value replace attempt:", scores)

	// Use *[]int only when replacing the slice itself.
	replaceScores(&scores)
	fmt.Println("Scores after slice replace:", scores)

	// Passing a slice as a buffer lets the function write into existing memory.
	buffer := make([]byte, 8)
	bytesWritten := writeToBuffer(buffer, "Hello, Go")
	fmt.Println("Buffer bytes:", buffer)
	fmt.Println("Buffer text:", string(buffer[:bytesWritten]))

	/* Section 10: Final pointer rule */
	fmt.Println("\n== 10. Final pointer rule ==")

	// Go does not have pointer arithmetic.
	// You can use pointers safely, but you cannot do agePointer++ like in C.
	// Prefer values first. Use pointers when they represent sharing, mutation, nil, or API needs.
	fmt.Println("Prefer values first; use pointers for sharing, mutation, nil, or API needs.")
}
