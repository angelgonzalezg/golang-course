package main

import (
	"errors"
	"math"
	"slices"
	"testing"
)

func TestCountPointer(t *testing.T) {
	if count := countPointer(nil); count != nil {
		t.Fatalf("nil record returned %p", count)
	}
	record := paddedRecord{Active: true, Count: 7, Ready: true}
	count := countPointer(&record)
	if count != &record.Count {
		t.Fatalf("computed field pointer=%p, want %p", count, &record.Count)
	}
	*count = 99
	if want := (paddedRecord{Active: true, Count: 99, Ready: true}); record != want {
		t.Fatalf("record=%+v, want %+v", record, want)
	}
}

func TestUint32AtUsesTheCallerElement(t *testing.T) {
	values := []uint32{10, 20, 30, 40}
	view := values[1:3] // Start within the backing array and restrict the visible length.
	for index := range view {
		pointer, err := uint32At(view, index)
		if err != nil || pointer != &view[index] {
			t.Fatalf("index=%d: pointer=%p error=%v, want %p", index, pointer, err, &view[index])
		}
		*pointer += 1
	}
	if want := []uint32{10, 21, 31, 40}; !slices.Equal(values, want) {
		t.Fatalf("caller values=%v, want %v", values, want)
	}
}

func TestUint32AtRejectsOutsideLength(t *testing.T) {
	for _, tt := range []struct {
		name   string
		values []uint32
		index  int
	}{
		{name: "nil slice"},
		{name: "empty slice", values: []uint32{}},
		{name: "negative index", values: []uint32{1}, index: -1},
		{name: "index at length", values: []uint32{1, 2}, index: 2},
		{name: "spare capacity", values: make([]uint32, 1, 8), index: 1},
		{name: "empty with capacity", values: make([]uint32, 0, 8)},
		{name: "large index", values: []uint32{1}, index: math.MaxInt},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := slices.Clone(tt.values)
			pointer, err := uint32At(tt.values, tt.index)
			if pointer != nil || !errors.Is(err, ErrIndexOutOfRange) {
				t.Fatalf("pointer=%p error=%v, want nil and ErrIndexOutOfRange", pointer, err)
			}
			if !slices.Equal(tt.values, before) {
				t.Fatalf("rejected lookup changed values: %v", tt.values)
			}
		})
	}
}
