package main

import "testing"

func TestSplitName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantFirst string
		wantLast  string
	}{
		{name: "empty input"},
		{name: "whitespace only", input: " \t\n "},
		{name: "one word", input: "Ada", wantFirst: "Ada"},
		{name: "two words", input: "Angel Gonzalez", wantFirst: "Angel", wantLast: "Gonzalez"},
		{name: "multiple remaining words", input: "Ana Garcia Lopez", wantFirst: "Ana", wantLast: "Garcia Lopez"},
		{name: "normalize whitespace", input: "  Ana\tGarcia \n Lopez  ", wantFirst: "Ana", wantLast: "Garcia Lopez"},
		{name: "unicode whitespace", input: "José\u00a0García", wantFirst: "José", wantLast: "García"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first, last := splitName(tt.input)
			if first != tt.wantFirst || last != tt.wantLast {
				t.Errorf("splitName(%q) = (%q, %q), want (%q, %q)", tt.input, first, last, tt.wantFirst, tt.wantLast)
			}
		})
	}
}

func TestMinMax(t *testing.T) {
	tests := []struct {
		name    string
		numbers []int
		wantMin int
		wantMax int
		wantOK  bool
	}{
		{name: "nil slice"},
		{name: "empty slice", numbers: []int{}},
		{name: "single zero", numbers: []int{0}, wantOK: true},
		{name: "unordered values", numbers: []int{8, 3, 10, 5}, wantMin: 3, wantMax: 10, wantOK: true},
		{name: "negative values", numbers: []int{-8, -3, -10}, wantMin: -10, wantMax: -3, wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			min, max, ok := minMax(tt.numbers)
			if min != tt.wantMin || max != tt.wantMax || ok != tt.wantOK {
				t.Errorf("minMax(%v) = (%d, %d, %t), want (%d, %d, %t)", tt.numbers, min, max, ok, tt.wantMin, tt.wantMax, tt.wantOK)
			}
		})
	}
}
