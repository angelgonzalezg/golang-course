// Package labels supplies small, deterministic operations to practice testing.
package labels

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

const MaxRunes = 40

var ErrInvalidLabel = errors.New("label must be valid UTF-8 with 1–40 runes after trimming")

// Normalize trims surrounding Unicode whitespace and lowercases a label.
// Internal whitespace and punctuation are preserved. Length counts runes,
// which can differ from both byte length and the number of visible characters.
// Failure always returns an empty string and ErrInvalidLabel.
func Normalize(input string) (string, error) {
	if !utf8.ValidString(input) {
		return "", ErrInvalidLabel
	}
	label := strings.ToLower(strings.TrimSpace(input))
	if count := utf8.RuneCountInString(label); count == 0 || count > MaxRunes {
		return "", ErrInvalidLabel
	}
	return label, nil
}

// LoadFile reads one small label file and preserves read or validation causes.
// The caller chooses the path; tests can supply files in t.TempDir().
func LoadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read label %q: %w", path, err)
	}
	label, err := Normalize(string(data))
	if err != nil {
		return "", fmt.Errorf("validate label %q: %w", path, err)
	}
	return label, nil
}
