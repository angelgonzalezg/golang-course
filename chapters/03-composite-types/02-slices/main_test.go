package main

import (
	"slices"
	"testing"
)

func TestCloneRowsPreservesValuesAndNilness(t *testing.T) {
	tests := []struct {
		name string
		rows [][]int
	}{
		{name: "nil outer slice"},
		{name: "empty outer slice", rows: [][]int{}},
		{name: "nil and empty rows", rows: [][]int{nil, {}, {1, 2}, {3}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cloneRows(tt.rows)
			if (got == nil) != (tt.rows == nil) || len(got) != len(tt.rows) {
				t.Fatalf("outer slice changed: got %v, want %v with matching nilness", got, tt.rows)
			}
			for index, row := range tt.rows {
				if (got[index] == nil) != (row == nil) || !slices.Equal(got[index], row) {
					t.Errorf("row %d changed: got %v, want %v with matching nilness", index, got[index], row)
				}
			}
		})
	}
}

func TestCloneRowsHasIndependentStorage(t *testing.T) {
	shared := []int{1, 2}
	original := [][]int{shared, shared}
	cloned := cloneRows(original)

	cloned[0][0] = 99
	if original[0][0] != 1 || original[1][0] != 1 || cloned[1][0] != 1 {
		t.Fatal("each cloned row must be independent of the source and of other cloned rows")
	}
	original[0][1] = 77
	if cloned[0][1] != 2 || cloned[1][1] != 2 {
		t.Fatal("a source mutation changed a cloned row")
	}
	cloned[1] = []int{3, 4}
	if !slices.Equal(original[1], []int{1, 77}) {
		t.Fatal("replacing a cloned row changed the source outer slice")
	}
}
