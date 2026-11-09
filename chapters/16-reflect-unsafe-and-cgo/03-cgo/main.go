// Optional cgo lab: run inside this independent module with go run .
// Topic: fixed-width arguments, borrowed Go memory, and owned C allocations.
package main

import (
	"errors"
	"fmt"
	"math"
	"os"

	"example.com/course/cgo-lab/native"
)

func run() error {
	fmt.Println("== 1. Call C through a Go API ==")
	value, err := native.Add(20, 22)
	if err != nil {
		return err
	}
	fmt.Println("20 + 22:", value)
	_, err = native.Add(math.MaxInt32, 1)
	fmt.Println("Overflow rejected:", errors.Is(err, native.ErrOverflow))

	fmt.Println("\n== 2. Borrow Go bytes during a synchronous call ==")
	input := []byte{1, 0, 2, 255}
	fmt.Println("Byte sum:", native.SumBytes(input))
	fmt.Println("Input after C read:", input)
	fmt.Println("Nil input sum:", native.SumBytes(nil))

	fmt.Println("\n== 3. Copy strings across the boundary and free C memory ==")
	greeting, err := native.Greet("niño")
	if err != nil {
		return err
	}
	fmt.Println(greeting)
	_, err = native.Greet("Ana\x00Hidden")
	fmt.Println("Embedded NUL rejected:", errors.Is(err, native.ErrEmbeddedNUL))

	fmt.Println("\n== 4. Mutate an owned C copy, then copy back to Go ==")
	reversed, err := native.ReverseCopy(input)
	if err != nil {
		return err
	}
	fmt.Println("Reversed bytes:", reversed)
	reversed[0] = 99
	fmt.Println("Edited Go result:", reversed)
	fmt.Println("Original input:", input)
	fmt.Println("\nRule: use cgo for a native dependency or system boundary; keep ownership and types explicit.")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
