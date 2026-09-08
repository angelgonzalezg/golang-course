// Panic and Recovery
// Topic: panic stops normal execution; recover can catch it from a deferred function.
// Args/Input: examples use small functions that intentionally panic for learning.
// Usage: prefer errors for expected problems; use panic for programmer mistakes.
// Key idea: recover is useful at boundaries where you do not want one panic to stop everything.
package main

import (
	"errors"
	"fmt"
	"sync"
)

/* Helper section 1: Error value used when recovering */
var ErrRecoveredPanic = errors.New("recovered panic")

/* Helper section 2: Panic examples */
func requirePositive(number int) {
	if number <= 0 {
		panic("number must be positive")
	}

	fmt.Println("positive number:", number)
}

func panicWithCleanup() {
	defer fmt.Println("cleanup still runs before the panic leaves the function")

	fmt.Println("starting work")
	panic("something broke")
}

func recoverPanic() {
	defer func() {
		recoveredValue := recover()
		if recoveredValue != nil {
			fmt.Println("recovered value:", recoveredValue)
		}
	}()

	panic("panic handled by recoverPanic")
}

func recoverOutsideDeferDoesNothing() {
	recoveredValue := recover()
	fmt.Println("recover outside defer:", recoveredValue)
}

func panicValueType() {
	defer func() {
		recoveredValue := recover()

		switch value := recoveredValue.(type) {
		case nil:
			fmt.Println("no panic happened")
		case string:
			fmt.Println("panic was a string:", value)
		case error:
			fmt.Println("panic was an error:", value)
		default:
			fmt.Println("panic had another type:", value)
		}
	}()

	panic(errors.New("database connection lost"))
}

/* Helper section 3: Recover boundary helpers */
func riskyDivide(a, b int) int {
	if b == 0 {
		panic("division by zero")
	}

	return a / b
}

func safeDivide(a, b int) (result int, err error) {
	defer func() {
		recoveredValue := recover()
		if recoveredValue != nil {
			result = 0
			err = fmt.Errorf("%w: %v", ErrRecoveredPanic, recoveredValue)
		}
	}()

	result = riskyDivide(a, b)
	return result, nil
}

func recoverAndRepanic() {
	defer func() {
		recoveredValue := recover()
		if recoveredValue != nil {
			fmt.Println("logging before re-panic:", recoveredValue)
			panic(recoveredValue)
		}
	}()

	panic("this panic is not handled here")
}

func protectBoundary(name string, work func()) {
	defer func() {
		recoveredValue := recover()
		if recoveredValue != nil {
			fmt.Println(name, "recovered:", recoveredValue)
		}
	}()

	work()
}

func worker(id int, wg *sync.WaitGroup) {
	defer wg.Done()

	defer func() {
		recoveredValue := recover()
		if recoveredValue != nil {
			fmt.Println("worker recovered:", recoveredValue)
		}
	}()

	if id == 2 {
		panic("worker 2 failed")
	}

	fmt.Println("worker finished:", id)
}

func main() {
	/* Section 1: Use errors for expected problems */
	fmt.Println("== 1. Use errors for expected problems ==")

	fmt.Println("Bad input, missing files, and failed requests should usually return error.")

	/* Section 2: Panic stops normal execution */
	fmt.Println("\n== 2. Panic stops normal execution ==")

	protectBoundary("requirePositive", func() {
		requirePositive(10)
		requirePositive(-1)
		fmt.Println("this line is skipped after panic")
	})

	/* Section 3: Deferred cleanup still runs */
	fmt.Println("\n== 3. Deferred cleanup still runs ==")

	protectBoundary("panicWithCleanup", panicWithCleanup)

	/* Section 4: Recover must be called from defer */
	fmt.Println("\n== 4. Recover must be called from defer ==")

	recoverOutsideDeferDoesNothing()
	recoverPanic()

	/* Section 5: Recover returns the panic value */
	fmt.Println("\n== 5. Recover returns the panic value ==")

	panicValueType()

	/* Section 6: Convert panic to error at a boundary */
	fmt.Println("\n== 6. Convert panic to error at a boundary ==")

	value, err := safeDivide(10, 2)
	if err != nil {
		fmt.Println("safe divide error:", err)
	} else {
		fmt.Println("safe divide result:", value)
	}

	value, err = safeDivide(10, 0)
	if err != nil {
		fmt.Println("safe divide error:", err)
	} else {
		fmt.Println("safe divide result:", value)
	}

	/* Section 7: Re-panic when you cannot handle it */
	fmt.Println("\n== 7. Re-panic when you cannot handle it ==")

	protectBoundary("outer boundary", recoverAndRepanic)

	/* Section 8: Recover inside each goroutine */
	fmt.Println("\n== 8. Recover inside each goroutine ==")

	// Each goroutine needs its own recover.
	// The order of goroutine output can change each run.
	var wg sync.WaitGroup
	for id := 1; id <= 3; id++ {
		wg.Add(1)
		go worker(id, &wg)
	}
	wg.Wait()

	/* Section 9: Final panic and recover rules */
	fmt.Println("\n== 9. Final panic and recover rules ==")

	// Panic is for programmer mistakes or impossible states.
	// Recover is usually placed at application or goroutine boundaries.
	// Do not use panic as normal control flow.
	fmt.Println("Prefer error first; use panic and recover sparingly.")
}
