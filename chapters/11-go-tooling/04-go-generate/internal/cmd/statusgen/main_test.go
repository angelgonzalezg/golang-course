package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseNames(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		want  []string
	}{
		{"input order", "Running\nQueued\nComplete\n", []string{"Running", "Queued", "Complete"}},
		{"comments and whitespace", "# status list\n\n Queued \r\n\tRunning\n", []string{"Queued", "Running"}},
		{"Unicode identifier", "Listo\nÉxito\n", []string{"Listo", "Éxito"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNames([]byte(tt.input))
			if err != nil || !slices.Equal(got, tt.want) {
				t.Fatalf("parseNames = (%v, %v), want (%v, nil)", got, err, tt.want)
			}
		})
	}
}

func TestParseNamesRejectsInvalidInput(t *testing.T) {
	for _, tt := range []struct{ name, input, context string }{
		{"empty input", "", "at least one"},
		{"comments only", "# nothing here\n \n", "at least one"},
		{"duplicate", "Queued\nQueued\n", "line 2"},
		{"invalid punctuation", "Queued\nIn-Progress\n", "line 2"},
		{"leading digit", "1Queued\n", "line 1"},
		{"keyword", "for\n", "line 1"},
		{"blank identifier", "_\n", "line 1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNames([]byte(tt.input))
			if err == nil || got != nil || !strings.Contains(err.Error(), tt.context) {
				t.Fatalf("parseNames = (%v, %v), want no partial names and error containing %q", got, err, tt.context)
			}
		})
	}
}

func TestInvalidInputPreservesGeneratedFile(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "statuses.txt")
	output := filepath.Join(directory, "status_names_gen.go")
	original := []byte("previous generated contents\n")
	if err := os.WriteFile(input, []byte("Queued\nQueued\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generateFile(input, output); err == nil {
		t.Fatal("duplicate status names were accepted")
	}
	got, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("invalid input changed previous output: (%q, %v)", got, err)
	}
}

func TestMissingInputDoesNotCreateOutput(t *testing.T) {
	directory := t.TempDir()
	output := filepath.Join(directory, "status_names_gen.go")
	if err := generateFile(filepath.Join(directory, "missing.txt"), output); err == nil {
		t.Fatal("missing input was accepted")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("unexpected output file or stat error: %v", err)
	}
}
