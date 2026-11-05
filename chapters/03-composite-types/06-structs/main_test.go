package main

import (
	"slices"
	"testing"
	"time"
)

func TestCloneEmployee(t *testing.T) {
	tests := []struct {
		name   string
		skills []string
	}{
		{name: "nil skills"},
		{name: "empty skills", skills: []string{}},
		{name: "populated skills", skills: []string{"Go", "SQL"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := Employee{
				ID: 7, FirstName: "Ana", LastName: "Lopez", Position: "Developer",
				Salary: 100000, IsActive: true,
				JoinedAt: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
				Skills:   tt.skills,
				Address:  Address{City: "Mexico City", Country: "Mexico"},
			}
			cloned := cloneEmployee(original)
			if cloned.ID != original.ID || cloned.FirstName != original.FirstName ||
				cloned.LastName != original.LastName || cloned.Position != original.Position ||
				cloned.Salary != original.Salary || cloned.IsActive != original.IsActive ||
				cloned.JoinedAt != original.JoinedAt || cloned.Address != original.Address {
				t.Fatal("the clone must preserve the employee's other fields")
			}
			if (cloned.Skills == nil) != (original.Skills == nil) || !slices.Equal(cloned.Skills, original.Skills) {
				t.Fatal("the clone must preserve skill values and nilness")
			}
			if len(cloned.Skills) > 0 {
				cloned.Skills[0] = "Rust"
				if original.Skills[0] != "Go" {
					t.Fatal("a cloned skill mutation changed the original employee")
				}
				original.Skills[1] = "Python"
				if cloned.Skills[1] != "SQL" {
					t.Fatal("an original skill mutation changed the cloned employee")
				}
			}
		})
	}
}
