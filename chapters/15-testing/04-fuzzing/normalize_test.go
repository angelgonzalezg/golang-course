package properties_test

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"golangcourse/chapters/15-testing/01-unit-tests"
)

func FuzzNormalize(f *testing.F) {
	for _, seed := range []string{"", " Go ", "NIÑO 🐹", " \t", "go  testing", strings.Repeat("É", labels.MaxRunes), strings.Repeat("a", labels.MaxRunes+1)} {
		f.Add(seed)
	}
	// testdata/fuzz/FuzzNormalize also supplies a checked-in invalid-UTF-8 seed.
	f.Fuzz(func(t *testing.T, input string) {
		got, err := labels.Normalize(input)
		if !utf8.ValidString(input) && !errors.Is(err, labels.ErrInvalidLabel) {
			t.Fatalf("invalid UTF-8 accepted: input=%q result=%q error=%v", input, got, err)
		}
		if err != nil {
			if !errors.Is(err, labels.ErrInvalidLabel) || got != "" {
				t.Fatalf("failure=(%q, %v), want empty result and ErrInvalidLabel", got, err)
			}
			return
		}
		if !utf8.ValidString(got) || utf8.RuneCountInString(got) < 1 || utf8.RuneCountInString(got) > labels.MaxRunes {
			t.Fatalf("successful result violates the UTF-8 or length contract: %q", got)
		}
		if strings.TrimSpace(got) != got || strings.ToLower(got) != got {
			t.Fatalf("successful result is not canonical: %q", got)
		}
		// Idempotence: normalizing an already normalized label changes nothing.
		if again, err := labels.Normalize(got); err != nil || again != got {
			t.Fatalf("second normalization=(%q, %v), want (%q, nil)", again, err, got)
		}
		// Surrounding whitespace must not change a successful label's meaning.
		if padded, err := labels.Normalize(" \t" + input + "\n"); err != nil || padded != got {
			t.Fatalf("padded normalization=(%q, %v), want (%q, nil)", padded, err, got)
		}
	})
}
