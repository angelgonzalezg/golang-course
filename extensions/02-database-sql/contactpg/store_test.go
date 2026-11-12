package contactpg

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// Domain errors must retain SQL/context causes. A successful commit stays
// successful even if the context becomes canceled immediately afterward.
func TestDatabaseErrorPreservesCauses(t *testing.T) {
	err := databaseError(t.Context(), sql.ErrNoRows)
	if !errors.Is(err, contacts.ErrNotFound) || !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing-row causes = %v", err)
	}
	for _, test := range []struct {
		cause *pgconn.PgError
		want  error
	}{
		{&pgconn.PgError{Code: "23505", ConstraintName: "sql_service_contacts_name_key"}, contacts.ErrNameTaken},
		{&pgconn.PgError{Code: "2200H"}, contacts.ErrIDExhausted},
	} {
		err := databaseError(t.Context(), test.cause)
		var cause *pgconn.PgError
		if !errors.Is(err, test.want) || !errors.As(err, &cause) || cause != test.cause {
			t.Fatalf("driver causes = %v", err)
		}
	}
	other := &pgconn.PgError{Code: "23505", ConstraintName: "other_key"}
	if errors.Is(databaseError(t.Context(), other), contacts.ErrNameTaken) {
		t.Fatal("unrelated constraint became a name conflict")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := databaseError(ctx, other); !errors.Is(err, context.Canceled) || !errors.Is(err, other) {
		t.Fatalf("canceled query causes = %v", err)
	}
	if err := databaseError(ctx, nil); err != nil {
		t.Fatalf("successful operation became a cancellation: %v", err)
	}
}
