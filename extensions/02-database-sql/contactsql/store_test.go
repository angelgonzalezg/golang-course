package contactsql

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Try valid boundaries and rejected text without PostgreSQL. Normalization must
// trim names, count runes correctly, and reject blank, malformed, or NUL-containing input.
func TestInputBoundaries(t *testing.T) {
	for _, test := range []struct {
		name     string
		in       Input
		wantName string
		invalid  bool
	}{
		{name: "trim Unicode whitespace", in: Input{Name: "\u00a0Ana \t"}, wantName: "Ana"},
		{name: "rune limit", in: Input{Name: strings.Repeat("ñ", 80)}, wantName: strings.Repeat("ñ", 80)},
		{name: "blank", in: Input{Name: " \t"}, invalid: true},
		{name: "too long", in: Input{Name: strings.Repeat("a", 81)}, invalid: true},
		{name: "invalid UTF-8", in: Input{Name: "a\xff"}, invalid: true},
		{name: "NUL", in: Input{Name: "A\x00B"}, invalid: true},
		{name: "email limit", in: Input{Name: "Ana", Email: stringPointer(strings.Repeat("é", 254))}, wantName: "Ana"},
		{name: "long email", in: Input{Name: "Ana", Email: stringPointer(strings.Repeat("a", 255))}, invalid: true},
		{name: "email NUL", in: Input{Name: "Ana", Email: stringPointer("a\x00b")}, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalize(test.in)
			if test.invalid {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil || got.Name != test.wantName {
				t.Fatalf("normalize = (%+v, %v)", got, err)
			}
		})
	}
}

// Wrap missing-row, unique-name, and canceled-query errors. Domain sentinels
// must remain inspectable alongside the original SQL/driver causes.
func TestQueryErrorPreservesCauses(t *testing.T) {
	err := QueryError(t.Context(), sql.ErrNoRows)
	if !errors.Is(err, ErrNotFound) || !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing row error = %v", err)
	}
	duplicate := &pgconn.PgError{Code: "23505", ConstraintName: "sql_lab_contacts_name_key"}
	err = QueryError(t.Context(), duplicate)
	var cause *pgconn.PgError
	if !errors.Is(err, ErrNameTaken) || !errors.As(err, &cause) || cause != duplicate {
		t.Fatalf("duplicate error = %v", err)
	}
	other := &pgconn.PgError{Code: "23505", ConstraintName: "another_key"}
	if errors.Is(QueryError(t.Context(), other), ErrNameTaken) {
		t.Fatal("unrelated unique violation classified as a name conflict")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = QueryError(ctx, duplicate)
	if !errors.Is(err, context.Canceled) || !errors.As(err, &cause) {
		t.Fatalf("canceled query error = %v", err)
	}
}

// Pass a blank DSN. Open must reject it without returning a pool or trying to
// use an implicit/default database connection.
func TestOpenNeedsConnectionString(t *testing.T) {
	db, err := Open(t.Context(), " \t")
	if err == nil || db != nil {
		t.Fatalf("Open blank = (%v, %v)", db, err)
	}
}

func stringPointer(value string) *string { return &value }
