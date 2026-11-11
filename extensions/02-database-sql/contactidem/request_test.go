package contactidem

import (
	"errors"
	"strings"
	"testing"

	"example.com/course/sql-lab/contactsql"
)

// Check identifier boundaries without PostgreSQL. Valid requests reuse the
// original contact rules; invalid caller/key/input data must fail before SQL.
func TestPrepareIdentifiersAndInput(t *testing.T) {
	for _, test := range []struct {
		name, subject, key, contact string
		invalid                     bool
	}{
		{"valid", "user-1", "request_1", " Ana ", false},
		{"maximum IDs", strings.Repeat("s", 64), strings.Repeat("k", 64), "Ana", false},
		{"missing subject", "", "key", "Ana", true},
		{"missing key", "user", "", "Ana", true},
		{"long subject", strings.Repeat("s", 65), "key", "Ana", true},
		{"long key", "user", strings.Repeat("k", 65), "Ana", true},
		{"space", "user", "two words", "Ana", true},
		{"non-ASCII", "usér", "key", "Ana", true},
		{"separator", "user\x00other", "key", "Ana", true},
		{"invalid contact", "user", "key", " ", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, _, err := prepare(test.subject, test.key, contactsql.Input{Name: test.contact})
			if test.invalid {
				if !errors.Is(err, contactsql.ErrInvalidInput) {
					t.Fatalf("prepare = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil || input.Name != "Ana" {
				t.Fatalf("prepare = (%+v, %v)", input, err)
			}
		})
	}
}

// Request identity uses field values, not pointer addresses or normalized names.
// Whitespace and NULL/empty differences must conflict under an already-used key.
func TestFingerprintRetainsOriginalValuesAndPresence(t *testing.T) {
	emptyA, emptyB := "", ""
	inputs := []contactsql.Input{{Name: " Ana "}, {Name: "Ana"}, {Name: "Ana", Email: &emptyA}, {Name: "Ana", Email: &emptyB}}
	hashes := make([][32]byte, len(inputs))
	for i, input := range inputs {
		normalized, hash, err := prepare("user", "key", input)
		if err != nil || normalized.Name != "Ana" {
			t.Fatalf("prepare = (%+v, %v)", normalized, err)
		}
		hashes[i] = hash
	}
	if hashes[0] == hashes[1] || hashes[1] == hashes[2] || hashes[2] != hashes[3] {
		t.Fatal("request identity lost whitespace/presence or used pointer identity")
	}
}
