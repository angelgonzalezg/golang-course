// An external test package sees the exported API, just like another consumer.
package labels_test

import (
	"fmt"

	"golangcourse/chapters/15-testing/01-unit-tests"
)

// Example output is checked by go test and also serves as runnable documentation.
func ExampleNormalize() {
	label, err := labels.Normalize(" Go Testing ")
	fmt.Println(label)
	fmt.Println(err)
	// Output:
	// go testing
	// <nil>
}
