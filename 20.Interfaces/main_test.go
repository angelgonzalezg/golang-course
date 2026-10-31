package main

import (
	"errors"
	"strings"
	"testing"
)

func TestWriteDescription(t *testing.T) {
	tests := []struct {
		name  string
		value Describer
		want  string
	}{
		{name: "employee", value: Employee{FullName: "Jane Doe", Position: "Developer"}, want: "Jane Doe, Developer\n"},
		{name: "customer", value: Customer{FullName: "Ana Lopez", Plan: "premium"}, want: "Ana Lopez, premium customer\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder
			if err := writeDescription(&output, tt.value); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != tt.want {
				t.Errorf("description = %q, want %q", got, tt.want)
			}
		})
	}
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestWriteDescriptionReturnsWriteError(t *testing.T) {
	wantErr := errors.New("destination is unavailable")
	err := writeDescription(failingWriter{err: wantErr}, Employee{FullName: "Jane Doe"})
	if !errors.Is(err, wantErr) {
		t.Errorf("writeDescription error = %v, want %v", err, wantErr)
	}
}
