// Generics
// Topic: generics let functions and types work with different concrete types safely.
// Args/Input: examples use ints, strings, custom numeric types, slices, maps, and stacks.
// Usage: use type parameters when the same logic should work for more than one type.
// Key idea: a type parameter is a placeholder type chosen when the function or type is used.
package main

import (
	"fmt"
	"maps"
	"slices"
)

/* Helper section 1: Generic function helpers */
type UserID int

/* any means T can be any type */
// [T any] introduces a type parameter named T.
func printValue[T any](value T) {
	fmt.Printf("Value: %v, Type: %T\n", value, value)
}

/* Return the first item from any slice type */
func first[T any](items []T) (T, bool) {
	if len(items) == 0 {
		// zero is the default value for whatever T is.
		var zero T
		return zero, false
	}

	return items[0], true
}

/* comparable means values can be compared with == or != */
// Slices, maps, and functions are not comparable, so they cannot be used here.
func contains[T comparable](items []T, target T) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}

	return false
}

/* Map keys must be comparable */
func keys[K comparable, V any](values map[K]V) []K {
	result := make([]K, 0, len(values))

	for key := range values {
		result = append(result, key)
	}

	return result
}

/* Helper section 2: Generic constraints */
/* Number limits T to types that support + */
// The | symbol means "or" in a type constraint.
// The ~ symbol allows named types whose underlying type matches.
type Number interface {
	~int | ~int64 | ~float64
}

func sum[T Number](numbers ...T) T {
	// total has the same type as the numbers passed in.
	var total T
	for _, number := range numbers {
		total += number
	}

	return total
}

/* Ordered limits T to values that can use < */
type Ordered interface {
	~int | ~int64 | ~float64 | ~string
}

func min[T Ordered](firstValue T, otherValues ...T) T {
	smallest := firstValue

	for _, value := range otherValues {
		if value < smallest {
			smallest = value
		}
	}

	return smallest
}

/* Transform a slice without caring about the element type */
func mapSlice[T any, R any](items []T, transform func(T) R) []R {
	result := make([]R, 0, len(items))

	for _, item := range items {
		result = append(result, transform(item))
	}

	return result
}

/* Keep only the items that pass a test */
func filter[T any](items []T, keep func(T) bool) []T {
	result := make([]T, 0, len(items))

	for _, item := range items {
		if keep(item) {
			result = append(result, item)
		}
	}

	return result
}

/* Combine every item into a single value */
// R is the accumulator type; it can differ from the item type T.
func reduce[T any, R any](items []T, initial R, combine func(R, T) R) R {
	accumulator := initial

	for _, item := range items {
		accumulator = combine(accumulator, item)
	}

	return accumulator
}

/* Helper section 3: Generic struct and methods */
type Pair[K comparable, V any] struct {
	Key   K
	Value V
}

/* Generic type with methods */
type Stack[T any] struct {
	items []T
}

func (s *Stack[T]) Push(value T) {
	s.items = append(s.items, value)
}

func (s *Stack[T]) Pop() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}

	lastIndex := len(s.items) - 1
	value := s.items[lastIndex]
	s.items = s.items[:lastIndex]
	return value, true
}

func (s Stack[T]) Len() int {
	return len(s.items)
}

/* Helper section 4: Constraints can require a method, not just a type */
// Stringer only accepts types that know how to describe themselves.
type Stringer interface {
	String() string
}

type Coin struct {
	Cents int
}

/* String makes Coin satisfy the Stringer constraint */
func (c Coin) String() string {
	return fmt.Sprintf("%d cents", c.Cents)
}

/* describe works with any type that has a String method */
func describe[T Stringer](value T) string {
	return "It says: " + value.String()
}

func main() {
	/* Section 1: Generic function basics */
	fmt.Println("== 1. Generic function basics ==")

	// Go infers T from the argument type.
	printValue(10)
	printValue("Go")
	printValue(true)

	/* Section 2: Generic functions with slices */
	fmt.Println("\n== 2. Generic functions with slices ==")

	if value, ok := first([]string{"Ana", "Ben"}); ok {
		fmt.Println("First name:", value)
	}

	if value, ok := first([]int{100, 200}); ok {
		fmt.Println("First number:", value)
	}

	// Use [int] when Go cannot infer T from the arguments.
	emptyValue, ok := first[int]([]int{})
	fmt.Println("Empty first value:", emptyValue)
	fmt.Println("Empty first ok:", ok)

	/* Section 3: comparable constraints */
	fmt.Println("\n== 3. comparable constraints ==")

	// comparable allows == and !=.
	fmt.Println("Contains Ben:", contains([]string{"Ana", "Ben"}, "Ben"))
	fmt.Println("Contains 9:", contains([]int{1, 2, 3}, 9))

	// This would not compile because []int is not comparable:
	// contains([][]int{{1}, {2}}, []int{1})

	/* Section 4: Generic map helpers */
	fmt.Println("\n== 4. Generic map helpers ==")

	ages := map[string]int{"Ana": 31, "Ben": 28}
	fmt.Println("Age map keys:", keys(ages))

	scores := map[int]string{1: "first", 2: "second"}
	fmt.Println("Score map keys:", keys(scores))

	/* Section 5: Type constraints with unions */
	fmt.Println("\n== 5. Type constraints with unions ==")

	fmt.Println("Int sum:", sum(1, 2, 3))
	fmt.Println("Float sum:", sum(1.5, 2.5, 3.5))

	// UserID is a named type whose underlying type is int.
	var firstID UserID = 10
	var secondID UserID = 20
	fmt.Println("UserID sum:", sum(firstID, secondID))

	/* Section 6: Ordered constraints */
	fmt.Println("\n== 6. Ordered constraints ==")

	fmt.Println("Smallest number:", min(8, 3, 10, 2))
	fmt.Println("Smallest name:", min("Mia", "Ana", "Zoe"))

	/* Section 7: Map, filter, and reduce */
	fmt.Println("\n== 7. Map, filter, and reduce ==")

	names := []string{"go", "generics", "type"}
	lengths := mapSlice(names, func(value string) int {
		return len(value)
	})
	fmt.Println("Name lengths:", lengths)

	numbers := []int{1, 2, 3, 4, 5, 6}
	labels := mapSlice(numbers, func(value int) string {
		return fmt.Sprintf("number-%d", value)
	})
	fmt.Println("Number labels:", labels)

	// filter keeps only the items where the function returns true.
	evenNumbers := filter(numbers, func(value int) bool {
		return value%2 == 0
	})
	fmt.Println("Even numbers:", evenNumbers)

	// reduce folds every item into a single accumulator value.
	total := reduce(numbers, 0, func(accumulator int, value int) int {
		return accumulator + value
	})
	fmt.Println("Sum via reduce:", total)

	// The accumulator type can differ from the item type.
	joined := reduce(names, "", func(accumulator string, value string) string {
		if accumulator == "" {
			return value
		}

		return accumulator + ", " + value
	})
	fmt.Println("Joined names via reduce:", joined)

	/* Section 8: Generic structs */
	fmt.Println("\n== 8. Generic structs ==")

	namePair := Pair[string, int]{Key: "Ana", Value: 31}
	idPair := Pair[int, string]{Key: 1, Value: "first"}
	fmt.Printf("Name pair: %+v\n", namePair)
	fmt.Printf("ID pair:   %+v\n", idPair)

	/* Section 9: Generic types with methods */
	fmt.Println("\n== 9. Generic types with methods ==")

	var stringStack Stack[string]
	stringStack.Push("first")
	stringStack.Push("second")
	fmt.Println("String stack length:", stringStack.Len())

	if value, ok := stringStack.Pop(); ok {
		fmt.Println("Popped string:", value)
	}

	var intStack Stack[int]
	intStack.Push(10)
	intStack.Push(20)
	fmt.Println("Int stack length:", intStack.Len())

	if value, ok := intStack.Pop(); ok {
		fmt.Println("Popped int:", value)
	}

	/* Section 10: Type inference and explicit type arguments */
	fmt.Println("\n== 10. Type inference and explicit type arguments ==")

	// Go can usually infer type arguments from regular arguments.
	fmt.Println("Inferred sum:", sum(4, 5, 6))

	// Explicit type arguments are useful when inference is not enough.
	emptyStack := Stack[float64]{}
	fmt.Println("Empty float stack length:", emptyStack.Len())

	/* Section 11: Constraints with methods */
	fmt.Println("\n== 11. Constraints with methods ==")

	// A constraint is not limited to a list of types like Number or Ordered.
	// It can also require a method, the same way a normal interface does.
	coin := Coin{Cents: 25}
	fmt.Println(describe(coin))

	/* Section 12: Generics already in the standard library */
	fmt.Println("\n== 12. Generics already in the standard library ==")

	// Go's standard library already ships generic helpers like the ones
	// we wrote by hand above (contains, keys). Prefer these when they fit.
	letters := []string{"c", "a", "b"}
	fmt.Println("slices.Contains 'a':", slices.Contains(letters, "a"))

	slices.Sort(letters)
	fmt.Println("slices.Sort result:", letters)

	stdlibKeys := slices.Sorted(maps.Keys(ages))
	fmt.Println("Sorted keys via maps.Keys + slices.Sorted:", stdlibKeys)

	/* Section 13: When not to use generics */
	fmt.Println("\n== 13. When not to use generics ==")

	// Do not use generics just to avoid writing a concrete type.
	// If the code only works with Employee, use Employee.
	// If the code needs behavior, an interface may be clearer.
	fmt.Println("Use generics when the same logic works across multiple types.")

	/* Section 14: Final generic rule */
	fmt.Println("\n== 14. Final generic rule ==")

	// Start concrete. Add type parameters when duplication appears.
	// Keep constraints as small as possible.
	fmt.Println("Generics remove repeated type-safe logic; constraints say what T must support.")
}
