package main

import "testing"

func TestStackPopClearsRemovedReference(t *testing.T) {
	first, second := 10, 20
	var stack Stack[*int]
	stack.Push(&first)
	stack.Push(&second)
	// Keep a view of the old storage so the test can inspect the removed slot directly.
	// This avoids relying on GC timing or finalizers.
	storage := stack.items

	got, ok := stack.Pop()
	if !ok || got != &second {
		t.Fatalf("Pop() = (%v, %t), want the second pointer and true", got, ok)
	}
	if storage[1] != nil {
		t.Fatal("the removed slot still retains the popped pointer")
	}
	if stack.Len() != 1 || storage[0] != &first {
		t.Fatal("popping must preserve the remaining item")
	}
}

func TestStackLIFOAndReuse(t *testing.T) {
	var stack Stack[int]
	if value, ok := stack.Pop(); value != 0 || ok {
		t.Fatalf("empty Pop() = (%d, %t), want (0, false)", value, ok)
	}
	stack.Push(10)
	stack.Push(20)
	for index, want := range []int{20, 10} {
		value, ok := stack.Pop()
		if !ok || value != want || stack.Len() != 1-index {
			t.Fatalf("Pop() = (%d, %t), len=%d; want %d, true, len=%d", value, ok, stack.Len(), want, 1-index)
		}
	}
	if value, ok := stack.Pop(); value != 0 || ok {
		t.Fatalf("drained Pop() = (%d, %t), want (0, false)", value, ok)
	}
	stack.Push(30)
	if value, ok := stack.Pop(); value != 30 || !ok {
		t.Fatalf("reused Pop() = (%d, %t), want (30, true)", value, ok)
	}
}

func TestStackNilElementIsPresent(t *testing.T) {
	var stack Stack[*int]
	stack.Push(nil)
	if value, ok := stack.Pop(); value != nil || !ok {
		t.Fatalf("stored nil: Pop() = (%v, %t), want (nil, true)", value, ok)
	}
	if value, ok := stack.Pop(); value != nil || ok {
		t.Fatalf("empty stack: Pop() = (%v, %t), want (nil, false)", value, ok)
	}
}

func TestStackPopClearsString(t *testing.T) {
	var stack Stack[string]
	stack.Push("temporary data")
	storage := stack.items
	if value, ok := stack.Pop(); value != "temporary data" || !ok {
		t.Fatalf("Pop() = (%q, %t)", value, ok)
	}
	if storage[0] != "" {
		t.Fatal("the removed string must be zeroed in the backing array")
	}
}

func TestFirstEmptyAndZeroValues(t *testing.T) {
	tests := []struct {
		name   string
		items  []int
		want   int
		wantOK bool
	}{
		{name: "nil slice"},
		{name: "empty slice", items: []int{}},
		{name: "stored zero", items: []int{0}, wantOK: true},
		{name: "multiple values", items: []int{7, 9}, want: 7, wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if value, ok := first(tt.items); value != tt.want || ok != tt.wantOK {
				t.Errorf("first(%v) = (%d, %t), want (%d, %t)", tt.items, value, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestSmallestWithOrderedTypes(t *testing.T) {
	if got := smallest(uint8(8), uint8(3), uint8(5)); got != 3 {
		t.Errorf("uint8 minimum = %d, want 3", got)
	}
	type Reading float32
	if got := smallest(Reading(2.5), Reading(1.5)); got != Reading(1.5) {
		t.Errorf("defined float32 minimum = %v, want 1.5", got)
	}
	if got := smallest("Mia", "Ana", "Zoe"); got != "Ana" {
		t.Errorf("string minimum = %q, want Ana", got)
	}
	if got := smallest(UserID(42)); got != UserID(42) {
		t.Errorf("single value minimum = %d, want 42", got)
	}
	values := []int{8, 3, 10, 2}
	if got := smallest(values[0], values[1:]...); got != 2 {
		t.Errorf("slice minimum = %d, want 2", got)
	}
}
