package main

import (
	"strconv"
	"unsafe"
)

// buildLabel uses an ordinary conversion with independent string semantics.
func buildLabel(prefix string, number int) string {
	buffer := append([]byte(prefix), ':')
	buffer = strconv.AppendInt(buffer, int64(number), 10)
	return string(buffer)
}

// buildLabelUnsafe shares a freshly built private buffer with its result.
// That buffer has no external mutable aliases and is never changed or reused
// after String. The string keeps the backing allocation reachable.
// A buffer borrowed from a caller or pool would require a different ownership
// contract. A view can also retain spare capacity; measure before choosing it.
func buildLabelUnsafe(prefix string, number int) string {
	buffer := append([]byte(prefix), ':')
	buffer = strconv.AppendInt(buffer, int64(number), 10)
	// The separator and digits ensure a nonempty buffer. String consumes its
	// current length, not capacity, and performs no final byte copy.
	return unsafe.String(unsafe.SliceData(buffer), len(buffer))
}
