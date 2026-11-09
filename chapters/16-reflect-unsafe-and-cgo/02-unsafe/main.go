// Unsafe
// Topic: memory layout, pointer conversions, bounded views, and ownership.
// Usage: go run ./chapters/16-reflect-unsafe-and-cgo/02-unsafe
// Key idea: start with typed operations; justify every unsafe representation.
package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"unsafe"
)

func showLayout() {
	fmt.Println("== 1. Size, alignment, and field offsets ==")
	var padded paddedRecord
	var grouped groupedRecord
	fmt.Printf("paddedRecord: size=%d alignment=%d offsets=(Active:%d Count:%d Ready:%d)\n",
		unsafe.Sizeof(padded), unsafe.Alignof(padded), unsafe.Offsetof(padded.Active), unsafe.Offsetof(padded.Count), unsafe.Offsetof(padded.Ready))
	fmt.Printf("groupedRecord: size=%d alignment=%d offsets=(Count:%d Active:%d Ready:%d)\n",
		unsafe.Sizeof(grouped), unsafe.Alignof(grouped), unsafe.Offsetof(grouped.Count), unsafe.Offsetof(grouped.Active), unsafe.Offsetof(grouped.Ready))

	values := []uint64{1, 2, 3, 4}
	text := "Go"
	fmt.Println("Slice descriptor bytes:", unsafe.Sizeof(values))
	fmt.Println("Visible uint64 element bytes:", uintptr(len(values))*unsafe.Sizeof(values[0]))
	fmt.Println("String descriptor bytes:", unsafe.Sizeof(text))
	fmt.Println("String content bytes:", len(text))
	// Sizeof excludes referenced storage. Neither measurement above describes
	// allocator overhead or the complete memory retained by an application.
}

func showPointerConversions() {
	fmt.Println("\n== 2. Reinterpret representation, or convert a numeric value ==")
	value := float64(1)
	bits := *(*uint64)(unsafe.Pointer(&value))
	fmt.Printf("Bits of 1.0: %#x\n", bits)
	fmt.Println("Same as math.Float64bits:", bits == math.Float64bits(value))
	fmt.Println("Numeric conversion uint64(1.0):", uint64(value))
	// This documented pattern uses equal-sized, compatible pointer-free storage.
	// Use math.Float64bits when that is the operation your application needs.
	// uintptr holds an integer address. It does not keep an object alive or
	// track pointer relocation; never store it for a later conversion back.
}

func showFieldAddressing() {
	fmt.Println("\n== 3. Address a known field within one struct ==")
	record := paddedRecord{Active: true, Count: 7, Ready: false}
	count := countPointer(&record)
	fmt.Println("Computed pointer matches &record.Count:", count == &record.Count)
	*count = 42
	fmt.Printf("Updated record: %+v\n", record)
	fmt.Println("Nil record returns nil:", countPointer(nil) == nil)
	// Direct access, record.Count = 42, is clearer for this known model.
}

func showElementAddressing() error {
	fmt.Println("\n== 4. Check slice bounds before pointer arithmetic ==")
	values := []uint32{10, 20, 30}
	element, err := uint32At(values, 1)
	if err != nil {
		return err
	}
	fmt.Println("Computed pointer matches &values[1]:", element == &values[1])
	*element = 99
	fmt.Println("Caller slice after update:", values)
	_, err = uint32At(values, len(values))
	fmt.Println("Index at len rejected:", errors.Is(err, ErrIndexOutOfRange))
	return nil
}

func showSliceViews() {
	fmt.Println("\n== 5. Slice views share the original array ==")
	array := [3]uint32{10, 20, 30}
	view := unsafe.Slice(&array[0], len(array))
	view[1] = 77
	fmt.Println("View data starts at &array[0]:", unsafe.SliceData(view) == &array[0])
	fmt.Println("Original array:", array)
	fmt.Println("View length and capacity:", len(view), cap(view))
	// array[:] expresses this view directly. unsafe.Slice needs the caller
	// to establish a real allocation, enough storage, alignment, and lifetime.
	var missing []uint32
	fmt.Println("SliceData(nil slice) is nil:", unsafe.SliceData(missing) == nil)
	fmt.Println("Slice(nil pointer, 0) is nil:", unsafe.Slice((*uint32)(nil), 0) == nil)
}

func showStringOwnership() {
	fmt.Println("\n== 6. A string view needs immutable backing bytes ==")
	ordinary := buildLabel("task", 42)
	privateView := buildLabelUnsafe("task", 42)
	fmt.Println("Ordinary conversion:", ordinary)
	fmt.Println("Private-buffer view:", privateView)
	fmt.Println("Both labels have equal contents:", ordinary == privateView)
	fmt.Println("String(nil pointer, 0) is empty:", unsafe.String(nil, 0) == "")

	fmt.Println("\n== 7. Borrow string bytes for reading, or make mutable bytes ==")
	text := "Go"
	readOnly := unsafe.Slice(unsafe.StringData(text), len(text))
	fmt.Println("Borrowed bytes:", readOnly)
	// The []byte type cannot enforce this read-only contract. Never write through
	// readOnly, pass it to a mutating function, or expose it as mutable storage.
	mutable := []byte(text)
	mutable[0] = 'g'
	fmt.Println("Changed independent bytes:", string(mutable))
	fmt.Println("Original string:", text)
}

func run() error {
	showLayout()
	showPointerConversions()
	showFieldAddressing()
	if err := showElementAddressing(); err != nil {
		return err
	}
	showSliceViews()
	showStringOwnership()
	fmt.Println("\nRule: prefer typed access and standard packages; justify unsafe with a documented representation or measured cost.")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
