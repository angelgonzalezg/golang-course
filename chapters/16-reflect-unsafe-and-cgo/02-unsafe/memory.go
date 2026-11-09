package main

import (
	"errors"
	"fmt"
	"unsafe"
)

// These records contain the same kinds of fields in different orders.
// Padding and alignment depend on the target; observe them instead of assuming
// a fixed struct size or using the in-memory layout as a wire format.
type paddedRecord struct {
	Active bool
	Count  uint64
	Ready  bool
}

type groupedRecord struct {
	Count  uint64
	Active bool
	Ready  bool
}

// countPointer demonstrates field addressing within one Go-owned struct.
// Offsetof locates the declared uint64 field, with its proper alignment.
// In ordinary code, return &record.Count after the same nil check.
func countPointer(record *paddedRecord) *uint64 {
	if record == nil {
		return nil
	}
	return (*uint64)(unsafe.Add(unsafe.Pointer(record), unsafe.Offsetof(record.Count)))
}

var ErrIndexOutOfRange = errors.New("index outside slice length")

// uint32At returns an alias to an existing element, or nil and an error.
// Validate against len, even when the backing array has additional capacity.
// The checked index and uint32 stride keep the result inside the same Go-owned
// array and aligned for uint32. The caller must provide a valid Go slice.
// Prefer &values[index] after the bounds check in ordinary application code.
func uint32At(values []uint32, index int) (*uint32, error) {
	if index < 0 || index >= len(values) {
		return nil, fmt.Errorf("%w: index=%d len=%d", ErrIndexOutOfRange, index, len(values))
	}
	base := unsafe.Pointer(&values[0])
	offset := uintptr(index) * unsafe.Sizeof(values[0])
	return (*uint32)(unsafe.Add(base, offset)), nil
}
