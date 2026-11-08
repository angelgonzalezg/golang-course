package labels

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Start with one concrete input and its observable result.
func TestNormalize(t *testing.T) {
	requireLabel(t, " Go ", "go")
}

// Mark the assertion as a helper so failures point to the calling test.
func requireLabel(t *testing.T, input, want string) {
	t.Helper()
	got, err := Normalize(input)
	if err != nil {
		t.Fatalf("Normalize(%q): %v", input, err) // This prerequisite failed; stop this test.
	}
	if got != want {
		t.Errorf("Normalize(%q) = %q, want %q", input, got, want)
	}
}

// A table groups cases with the same setup and assertions; each name is runnable.
func TestNormalizeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "canonical", input: "go", want: "go"},
		{name: "unicode", input: "\u00a0NIÑO 🐹\u00a0", want: "niño 🐹"},
		{name: "internal spaces", input: " Go  Testing ", want: "go  testing"},
		{name: "rune limit", input: strings.Repeat("É", MaxRunes), want: strings.Repeat("é", MaxRunes)},
		{name: "too long", input: strings.Repeat("a", MaxRunes+1), wantErr: true},
		{name: "empty", wantErr: true},
		{name: "whitespace", input: " \t\n", wantErr: true},
		{name: "invalid UTF-8", input: "go\xff", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Each case uses only immutable input and local results; no shared state.
			t.Parallel()
			got, err := Normalize(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidLabel) {
					t.Errorf("error = %v, want ErrInvalidLabel", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("result = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadFile(t *testing.T) {
	for _, input := range []struct {
		name string
		text string
		want string
		err  error
	}{
		{name: "valid", text: " GO\n", want: "go"},
		{name: "invalid", text: " \n", err: ErrInvalidLabel},
	} {
		t.Run(input.name, func(t *testing.T) {
			// TempDir creates isolated storage and removes it after this subtest.
			path := filepath.Join(t.TempDir(), "label.txt")
			if err := os.WriteFile(path, []byte(input.text), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := LoadFile(path)
			if got != input.want || !errors.Is(err, input.err) {
				t.Fatalf("LoadFile = (%q, %v), want (%q, %v)", got, err, input.want, input.err)
			}
		})
	}
	// A guaranteed missing path avoids relying on files outside the test.
	got, err := LoadFile(filepath.Join(t.TempDir(), "missing.txt"))
	if got != "" || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file = (%q, %v), want empty string and fs.ErrNotExist", got, err)
	}
}

func TestLoadFileFixture(t *testing.T) {
	// go test runs in the package directory. testdata holds checked-in fixtures.
	got, err := LoadFile("testdata/label.txt")
	if err != nil || got != "go testing" {
		t.Fatalf("fixture = (%q, %v), want (%q, nil)", got, err, "go testing")
	}
}
