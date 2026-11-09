// Package native confines cgo to a small boundary with ordinary Go API types.
package native

/*
#cgo CFLAGS: -std=c11 -Wall -Wextra
#include <stdlib.h>
#include "ops.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unsafe"
)

var (
	ErrOverflow         = errors.New("sum outside int32 range")
	ErrEmbeddedNUL      = errors.New("C string input contains a NUL byte")
	ErrNativeAllocation = errors.New("native greeting could not be allocated")
	ErrCopyLength       = errors.New("copy length outside supported C.int range")
)

// Add uses fixed-width C arguments and checks the wider result before narrowing.
func Add(a, b int32) (int32, error) {
	result := int64(C.course_add_int32(C.int32_t(a), C.int32_t(b)))
	if result < math.MinInt32 || result > math.MaxInt32 {
		return 0, fmt.Errorf("%w: %d + %d", ErrOverflow, a, b)
	}
	return int32(result), nil
}

// SumBytes borrows a pointer-free Go backing array for one synchronous call.
// C reads but neither writes nor retains it. The caller must not concurrently
// mutate that array. The result is the byte sum modulo 2^64, not a secure hash.
func SumBytes(data []byte) uint64 {
	if len(data) == 0 {
		return uint64(C.course_sum_bytes(nil, 0))
	}
	return uint64(C.course_sum_bytes((*C.uchar)(unsafe.Pointer(unsafe.SliceData(data))), C.size_t(len(data))))
}

// Greet copies a NUL-free Go string into C memory, then copies the native
// greeting back to Go before releasing both C allocations. Empty names are valid.
// Conversion preserves bytes; C does not interpret or transform Unicode here.
func Greet(name string) (string, error) {
	if strings.IndexByte(name, 0) >= 0 {
		return "", ErrEmbeddedNUL
	}
	input := C.CString(name)
	defer C.free(unsafe.Pointer(input))
	output := C.course_greet(input)
	if output == nil {
		return "", ErrNativeAllocation
	}
	defer C.free(unsafe.Pointer(output))
	return C.GoString(output), nil
}

// validateCopyLength checks the lab's explicit 32-bit length limit without
// allocating a huge buffer. C.GoBytes receives that count through C.int.
func validateCopyLength(length int64) error {
	if length < 0 || length > math.MaxInt32 {
		return ErrCopyLength
	}
	return nil
}

// ReverseCopy returns independent reversed bytes and leaves input unchanged.
// Binary NUL bytes are supported because the C operation takes an explicit
// length. Empty input returns nil. C storage is freed after the result is copied.
func ReverseCopy(data []byte) ([]byte, error) {
	if err := validateCopyLength(int64(len(data))); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	buffer := C.CBytes(data)
	defer C.free(buffer)
	C.course_reverse_bytes((*C.uchar)(buffer), C.size_t(len(data)))
	return C.GoBytes(buffer, C.int(len(data))), nil
}
