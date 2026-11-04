// Values
// Topic: values are the actual pieces of data a Go program works with.
// Args/Input: this example does not receive input; it prints values directly.
// Usage: use fmt.Println and fmt.Printf to inspect values and their types.
// Key idea: every value has a type, and that type controls what you can do with it.
package main

import "fmt"

func main() {
	/* Section 1: String values */
	fmt.Println("== 1. String values ==")

	fmt.Println("Hello, Go!")
	fmt.Println("Go" + "lang") // + joins strings.
	fmt.Println("line one\nline two")
	fmt.Println(`raw strings keep \n as text`)
	fmt.Printf("quoted string: %q\n", "Hello, Go!")

	/* Section 2: Integer values */
	fmt.Println("\n== 2. Integer values ==")

	fmt.Println(42)
	fmt.Println(-10)
	fmt.Println(10 + 5)
	fmt.Println(10 - 5)
	fmt.Println(10 * 5)
	fmt.Println(10 / 3) // Integer division drops the decimal part.
	fmt.Println(10 % 3) // Remainder after division.

	/* Section 3: Floating-point values */
	fmt.Println("\n== 3. Floating-point values ==")

	fmt.Println(3.14)
	fmt.Println(10.0 / 3.0)
	fmt.Println(2.5 * 4)

	/* Section 4: Boolean values */
	fmt.Println("\n== 4. Boolean values ==")

	fmt.Println(true)
	fmt.Println(false)
	fmt.Println(5 > 3)
	fmt.Println(5 == 3)
	fmt.Println(5 > 3 && 2 < 4) // && means and.
	fmt.Println(5 > 3 || 2 > 4) // || means or.
	fmt.Println(!true)          // ! flips a boolean.

	/* Section 5: Rune and byte values */
	fmt.Println("\n== 5. Rune and byte values ==")

	// A rune literal represents one Unicode code point; rune is an alias for int32.
	// One visible character can contain more than one code point.
	fmt.Printf("rune value: %c, number: %v, type: %T\n", 'A', 'A', 'A')

	// byte is an alias for uint8 and is common when working with raw data.
	fmt.Printf("byte value: %c, number: %v, type: %T\n", byte('A'), byte('A'), byte('A'))

	/* Section 6: Complex number values */
	fmt.Println("\n== 6. Complex number values ==")

	// Complex numbers are built into Go, but most programs do not need them.
	fmt.Println(2 + 3i)
	fmt.Printf("complex type: %T\n", 2+3i)

	/* Section 7: Inspect values with fmt */
	fmt.Println("\n== 7. Inspect values with fmt ==")

	fmt.Printf("value: %v, type: %T\n", "hello", "hello")
	fmt.Printf("value: %v, type: %T\n", 42, 42)
	fmt.Printf("value: %v, type: %T\n", 3.14, 3.14)
	fmt.Printf("value: %v, type: %T\n", true, true)

	/* Section 8: Convert values explicitly */
	fmt.Println("\n== 8. Convert values explicitly ==")

	// Go does not automatically mix integer and float types.
	fmt.Println(float64(10) / 4)
	almostFour := 3.99
	fmt.Println(int(almostFour)) // Converting float to int drops the decimal part.
	fmt.Println(string('A'))

	/* Section 9: Collection values preview */
	fmt.Println("\n== 9. Collection values preview ==")

	// These topics are covered later, but they are values too.
	fmt.Println([]int{1, 2, 3})
	fmt.Println(map[string]int{"go": 2009})

	/* Section 10: nil means no value for some types */
	fmt.Println("\n== 10. nil means no value for some types ==")

	var numbers []int
	var scores map[string]int
	var callback func()
	var anything any

	fmt.Println(numbers == nil)
	fmt.Println(scores == nil)
	fmt.Println(callback == nil)
	fmt.Println(anything == nil)

	/* Section 11: Final value rule */
	fmt.Println("\n== 11. Final value rule ==")

	// Values can be printed, compared, calculated, stored, and passed around.
	fmt.Println("A Go program is built by working with typed values.")
}
