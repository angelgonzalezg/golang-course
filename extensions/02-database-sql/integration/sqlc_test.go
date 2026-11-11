//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"example.com/course/sql-lab/contactsql"
	"example.com/course/sql-lab/internal/sqlcdemo"
	"example.com/course/sql-lab/internal/sqlcgen"
	"github.com/jackc/pgx/v5/pgconn"
)

// The tests consume only the operations they check. Production stores do not
// need to declare this interface; Go checks their method sets at the factories.
type contactStore interface {
	Create(context.Context, contactsql.Input) (contactsql.Contact, error)
	Get(context.Context, int64) (contactsql.Contact, error)
	List(context.Context, int) ([]contactsql.Contact, error)
	UpdateEmail(context.Context, int64, *string) (contactsql.Contact, error)
	Delete(context.Context, int64) error
}

type storeFactory func(*sql.DB) contactStore

// Each serial contract case owns an empty stage-1 table in a disposable database.
// This helper never truncates the learner's Compose database.
func resetFundamentalsTable(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), "TRUNCATE sql_lab_contacts RESTART IDENTITY"); err != nil {
		t.Fatal(err)
	}
}

// Invalid inputs must fail before writing rows. Missing-row and duplicate-name
// errors must retain the same sentinels AND underlying causes in both stores.
func testStoreErrors(t *testing.T, db *sql.DB, _ string, newStore storeFactory) {
	store, ctx := newStore(db), t.Context()
	for _, input := range []contactsql.Input{{Name: " "}, {Name: strings.Repeat("x", 81)}} {
		if _, err := store.Create(ctx, input); !errors.Is(err, contactsql.ErrInvalidInput) {
			t.Fatalf("invalid creation = %v", err)
		}
	}
	for _, id := range []int64{0, -1} {
		if _, err := store.Get(ctx, id); !errors.Is(err, contactsql.ErrInvalidInput) {
			t.Fatalf("invalid get ID = %v", err)
		}
		if _, err := store.UpdateEmail(ctx, id, nil); !errors.Is(err, contactsql.ErrInvalidInput) {
			t.Fatalf("invalid update ID = %v", err)
		}
		if err := store.Delete(ctx, id); !errors.Is(err, contactsql.ErrInvalidInput) {
			t.Fatalf("invalid delete ID = %v", err)
		}
	}
	for _, limit := range []int{0, 101} {
		if _, err := store.List(ctx, limit); !errors.Is(err, contactsql.ErrInvalidInput) {
			t.Fatalf("invalid limit = %v", err)
		}
	}
	badEmail := strings.Repeat("é", 255)
	if _, err := store.Create(ctx, contactsql.Input{Name: "Ana", Email: &badEmail}); !errors.Is(err, contactsql.ErrInvalidInput) {
		t.Fatalf("invalid creation email = %v", err)
	}
	if _, err := store.UpdateEmail(ctx, 1, &badEmail); !errors.Is(err, contactsql.ErrInvalidInput) {
		t.Fatalf("invalid update email = %v", err)
	}
	page, err := store.List(ctx, 100)
	if err != nil || len(page) != 0 {
		t.Fatalf("invalid inputs wrote rows: (%v, %v)", page, err)
	}
	_, err = store.Get(ctx, 1)
	if !errors.Is(err, contactsql.ErrNotFound) || !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing row lost its causes: %v", err)
	}
	if _, err := store.Create(ctx, contactsql.Input{Name: "Ana"}); err != nil {
		t.Fatal(err)
	}
	_, err = store.Create(ctx, contactsql.Input{Name: "Ana"})
	var cause *pgconn.PgError
	if !errors.Is(err, contactsql.ErrNameTaken) || !errors.As(err, &cause) || cause.Code != "23505" || cause.ConstraintName != "sql_lab_contacts_name_key" {
		t.Fatalf("duplicate name lost its causes: %v", err)
	}
}

// Editing a returned optional value must not affect another result or a stored
// row. Check create, list, and update results rather than comparing pointer addresses.
func testStoreResultOwnership(t *testing.T, db *sql.DB, _ string, newStore storeFactory) {
	store, ctx := newStore(db), t.Context()
	email := "original@example.com"
	first, err := store.Create(ctx, contactsql.Input{Name: "Ana", Email: &email})
	if err != nil || first.Email == nil {
		t.Fatalf("first create = (%+v, %v)", first, err)
	}
	if _, err := store.Create(ctx, contactsql.Input{Name: "Bea", Email: &email}); err != nil {
		t.Fatal(err)
	}
	*first.Email = "caller changed create result"
	page, err := store.List(ctx, 2)
	if err != nil || len(page) != 2 || page[0].Email == nil || page[1].Email == nil || *page[0].Email != email || *page[1].Email != email {
		t.Fatalf("stored emails = (%v, %v)", page, err)
	}
	*page[0].Email = "caller changed list result"
	if *page[1].Email != email {
		t.Fatal("list entries share mutable email storage")
	}
	updated, err := store.UpdateEmail(ctx, first.ID, &email)
	if err != nil || updated.Email == nil {
		t.Fatalf("update = (%+v, %v)", updated, err)
	}
	*updated.Email = "caller changed update result"
	stored, err := store.Get(ctx, first.ID)
	if err != nil || stored.Email == nil || *stored.Email != email || email != "original@example.com" {
		t.Fatalf("caller edits reached database or input: (%+v, %v)", stored, err)
	}
}

// Bind generated queries to explicit caller-owned transactions. Rollback must
// remove the uncommitted row; a separate Commit must make its row visible.
func testSQLCTransactions(t *testing.T, db *sql.DB, _ string) {
	queries := sqlcgen.New(db)
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[commit], func(t *testing.T) {
			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			bound := queries.WithTx(tx)
			created, err := bound.CreateContact(t.Context(), sqlcgen.CreateContactParams{Name: "Transactional"})
			if err != nil {
				t.Fatal(err)
			}
			if got, err := bound.GetContact(t.Context(), created.ID); err != nil || got.ID != created.ID {
				t.Fatalf("transaction cannot read its own row: (%+v, %v)", got, err)
			}
			// A pool query must not see this uncommitted insert. This also catches
			// accidentally using the original Queries instead of WithTx's result.
			if _, err := queries.GetContact(t.Context(), created.ID); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("uncommitted insert is visible outside tx: %v", err)
			}
			if commit {
				err = tx.Commit()
			} else {
				err = tx.Rollback()
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := contactsql.NewStore(db).Get(t.Context(), created.ID)
			if commit {
				if err != nil || got.Name != "Transactional" {
					t.Fatalf("committed row = (%+v, %v)", got, err)
				}
			} else if !errors.Is(err, contactsql.ErrNotFound) {
				t.Fatalf("rolled-back row survived: %v", err)
			}
		})
	}
}

// Run the comparison beside an unrelated row. Its cross-store reads and
// rollback must succeed, and finite cleanup must preserve the unrelated row.
func testSQLCDemo(t *testing.T, db *sql.DB, dsn string) {
	store := contactsql.NewStore(db)
	retained, err := store.Create(t.Context(), contactsql.Input{Name: "Keep this row"})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := sqlcdemo.Run(t.Context(), dsn, &output); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"same PostgreSQL row", "SQL NULL", "rollback", "ErrNameTaken/ErrNotFound"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("comparison output missing %q", text)
		}
	}
	page, err := store.List(t.Context(), 100)
	if err != nil || len(page) != 1 || page[0].ID != retained.ID || page[0].Name != retained.Name {
		t.Fatalf("comparison changed unrelated data: (%v, %v)", page, err)
	}
}
