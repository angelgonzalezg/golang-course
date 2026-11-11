//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/course/sql-lab/contactidem"
	"example.com/course/sql-lab/contactsql"
	"example.com/course/sql-lab/contacttx"
	"example.com/course/sql-lab/internal/idempotencydemo"
	"github.com/golang-migrate/migrate/v4"
	"github.com/jackc/pgx/v5/pgconn"
)

// Run the durable-create contracts on a private PostgreSQL server. Each named
// case starts without this lesson's schema/history; earlier-stage tables are independent.
func TestIdempotency(t *testing.T) {
	db, dsn := newTestDatabase(t)
	for _, test := range []struct {
		name string
		run  func(*testing.T, *sql.DB, string)
	}{
		{"independent repeatable migrations", testIdempotencyMigrations},
		{"create replay and result ownership", testCreationReplay},
		{"payload binding and caller scope", testCreationKeyScope},
		{"validation and pre-canceled requests", testCreationValidation},
		{"business error leaves key available", testCreationNameConflict},
		{"result failure rolls back contact", testCreationResultFailure},
		{"concurrent retries across pools", testConcurrentCreationReplays},
		{"concurrent different payloads", testConcurrentCreationConflict},
		{"historical replay after edit and delete", testHistoricalCreationReplay},
		{"canceled duplicate preserves original", testCanceledCreationWaiter},
		{"cancellation after contact insert", testCanceledCreationWrite},
		{"lost reply and separate processes", testCreationProcessRestart},
		{"finite demo preserves other callers", testIdempotencyDemo},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetIdempotencyTables(t, db)
			test.run(t, db, dsn)
		})
	}
}

// Reset only the stage-3 tables inside this suite's disposable test database.
func resetIdempotencyTables(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), `DROP TABLE IF EXISTS
		sql_idem_create_requests, sql_idem_contacts, sql_idem_schema_migrations`); err != nil {
		t.Fatal(err)
	}
}

// Prepare a fresh schema and return a store that borrows this test's pool.
func idempotencyStore(t *testing.T, db *sql.DB, dsn string) *contactidem.Store {
	t.Helper()
	if version, err := contactidem.Migrate(dsn); err != nil || version != contactidem.LatestVersion {
		t.Fatalf("migrate idempotency = (%d, %v)", version, err)
	}
	return contactidem.NewStore(db)
}

// Count committed business rows and complete request records independently.
// Checking only the returned error/result could miss an unintended side effect.
func assertIdempotencyCounts(t *testing.T, db *sql.DB, wantContacts, wantRequests int) {
	t.Helper()
	var contacts, requests int
	if err := db.QueryRowContext(t.Context(), `SELECT
		(SELECT count(*) FROM sql_idem_contacts),
		(SELECT count(*) FROM sql_idem_create_requests)`).Scan(&contacts, &requests); err != nil {
		t.Fatal(err)
	}
	if contacts != wantContacts || requests != wantRequests {
		t.Fatalf("committed counts = (%d, %d), want (%d, %d)", contacts, requests, wantContacts, wantRequests)
	}
}

// Compare snapshot contents, including NULL versus empty email, not pointer addresses.
func assertCreationContact(t *testing.T, got, want contactsql.Contact) {
	t.Helper()
	if got.ID != want.ID || got.Name != want.Name || (got.Email == nil) != (want.Email == nil) {
		t.Fatalf("contact = %+v, want %+v", got, want)
	}
	if got.Email != nil && *got.Email != *want.Email {
		t.Fatalf("email = %q, want %q", *got.Email, *want.Email)
	}
}

// Stage 3 can migrate repeatedly without changing stage 2's schema version.
// Dirty/future metadata must stop migration instead of being silently overwritten.
func testIdempotencyMigrations(t *testing.T, db *sql.DB, dsn string) {
	if _, err := contacttx.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	idempotencyStore(t, db, dsn)
	idempotencyStore(t, db, dsn)
	if version, err := contacttx.Migrate(dsn); err != nil || version != 2 {
		t.Fatalf("stage-2 schema = (%d, %v)", version, err)
	}
	for _, dirty := range []bool{true, false} {
		t.Run(map[bool]string{true: "dirty", false: "future version"}[dirty], func(t *testing.T) {
			resetIdempotencyTables(t, db)
			idempotencyStore(t, db, dsn)
			version := 99
			if dirty {
				version = 1
			}
			if _, err := db.ExecContext(t.Context(), "UPDATE sql_idem_schema_migrations SET version = $1, dirty = $2", version, dirty); err != nil {
				t.Fatal(err)
			}
			_, err := contactidem.Migrate(dsn)
			var dirtyError migrate.ErrDirty
			if err == nil || dirty && !errors.As(err, &dirtyError) {
				t.Fatalf("unreviewed schema state was accepted: %v", err)
			}
			var gotVersion int
			var gotDirty bool
			if err := db.QueryRowContext(t.Context(), "SELECT version, dirty FROM sql_idem_schema_migrations").Scan(&gotVersion, &gotDirty); err != nil {
				t.Fatal(err)
			}
			if gotVersion != version || gotDirty != dirty {
				t.Fatal("migration rewrote unreviewed metadata")
			}
		})
	}
}

// Repeat one valid creation and edit the caller's returned snapshot. One contact
// and one request remain, and later replays must retain their own email storage.
func testCreationReplay(t *testing.T, db *sql.DB, dsn string) {
	store := idempotencyStore(t, db, dsn)
	email := "ana@example.com"
	input := contactsql.Input{Name: " Ana ", Email: &email}
	first, err := store.CreateOnce(t.Context(), "user", "key", input)
	if err != nil || first.Replayed || first.Contact.ID <= 0 || first.Contact.Name != "Ana" || first.Contact.Email == nil || *first.Contact.Email != email {
		t.Fatalf("first creation = (%+v, %v)", first, err)
	}
	again, err := store.CreateOnce(t.Context(), "user", "key", input)
	if err != nil || !again.Replayed {
		t.Fatalf("replay = (%+v, %v)", again, err)
	}
	assertCreationContact(t, again.Contact, first.Contact)
	*again.Contact.Email = "caller edit"
	later, err := store.CreateOnce(t.Context(), "user", "key", input)
	if err != nil || !later.Replayed {
		t.Fatalf("later replay = (%+v, %v)", later, err)
	}
	assertCreationContact(t, later.Contact, first.Contact)
	assertIdempotencyCounts(t, db, 1, 1)
}

// The original raw fields bind a key, including whitespace and NULL/empty data.
// Another subject has a separate key namespace but still follows name uniqueness.
func testCreationKeyScope(t *testing.T, db *sql.DB, dsn string) {
	store := idempotencyStore(t, db, dsn)
	if _, err := store.CreateOnce(t.Context(), "user-A", "same-key", contactsql.Input{Name: " Ana "}); err != nil {
		t.Fatal(err)
	}
	empty := ""
	for _, input := range []contactsql.Input{{Name: "Ana"}, {Name: " Ana ", Email: &empty}, {Name: "Bea"}} {
		if _, err := store.CreateOnce(t.Context(), "user-A", "same-key", input); !errors.Is(err, contactidem.ErrKeyReuse) {
			t.Fatalf("changed request = %v", err)
		}
	}
	other, err := store.CreateOnce(t.Context(), "user-B", "same-key", contactsql.Input{Name: "Bea"})
	if err != nil || other.Replayed {
		t.Fatalf("another subject = (%+v, %v)", other, err)
	}
	assertIdempotencyCounts(t, db, 2, 2)
}

// Invalid or already-canceled requests must reach neither the business insert
// nor the saved-result insert. Validation uses the same contact rules as stage 1.
func testCreationValidation(t *testing.T, db *sql.DB, dsn string) {
	store := idempotencyStore(t, db, dsn)
	for _, test := range []struct{ subject, key, name string }{{"", "key", "Ana"}, {"user", "bad key", "Ana"}, {"user", "key", " "}} {
		if _, err := store.CreateOnce(t.Context(), test.subject, test.key, contactsql.Input{Name: test.name}); !errors.Is(err, contactsql.ErrInvalidInput) {
			t.Fatalf("invalid request = %v", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.CreateOnce(ctx, "user", "key", contactsql.Input{Name: "Ana"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled request = %v", err)
	}
	assertIdempotencyCounts(t, db, 0, 0)
}

// A failed business insert rolls back without binding its key. A corrected
// request can reuse that key because no successful result was ever committed.
func testCreationNameConflict(t *testing.T, db *sql.DB, dsn string) {
	store := idempotencyStore(t, db, dsn)
	if _, err := store.CreateOnce(t.Context(), "user", "first", contactsql.Input{Name: "Ana"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOnce(t.Context(), "user", "retryable", contactsql.Input{Name: "Ana"}); !errors.Is(err, contactsql.ErrNameTaken) {
		t.Fatalf("name conflict = %v", err)
	}
	assertIdempotencyCounts(t, db, 1, 1)
	got, err := store.CreateOnce(t.Context(), "user", "retryable", contactsql.Input{Name: "Bea"})
	if err != nil || got.Replayed {
		t.Fatalf("corrected request = (%+v, %v)", got, err)
	}
	assertIdempotencyCounts(t, db, 2, 2)
}

// Force the result INSERT to fail after the contact INSERT. Both writes must
// disappear, and removing the test constraint must let the same key create fresh.
func testCreationResultFailure(t *testing.T, db *sql.DB, dsn string) {
	store := idempotencyStore(t, db, dsn)
	if _, err := db.ExecContext(t.Context(), "ALTER TABLE sql_idem_create_requests ADD CONSTRAINT test_reject_saved_result CHECK (false)"); err != nil {
		t.Fatal(err)
	}
	_, err := store.CreateOnce(t.Context(), "user", "key", contactsql.Input{Name: "Ana"})
	var cause *pgconn.PgError
	if !errors.As(err, &cause) || cause.Code != "23514" || cause.ConstraintName != "test_reject_saved_result" {
		t.Fatalf("forced result failure = %v", err)
	}
	assertIdempotencyCounts(t, db, 0, 0)
	if _, err := db.ExecContext(t.Context(), "ALTER TABLE sql_idem_create_requests DROP CONSTRAINT test_reject_saved_result"); err != nil {
		t.Fatal(err)
	}
	if got, err := store.CreateOnce(t.Context(), "user", "key", contactsql.Input{Name: "Ana"}); err != nil || got.Replayed {
		t.Fatalf("retry after rollback = (%+v, %v)", got, err)
	}
	assertIdempotencyCounts(t, db, 1, 1)
}

type creationOutcome struct {
	result contactidem.Result
	err    error
}

// Twelve callers use stores backed by two independent pools. Exactly one
// creates; all other calls must replay that same saved contact.
func testConcurrentCreationReplays(t *testing.T, db *sql.DB, dsn string) {
	first := idempotencyStore(t, db, dsn)
	secondPool, err := contactsql.Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer secondPool.Close()
	stores := []*contactidem.Store{first, contactidem.NewStore(secondPool)}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	// Buffer all outcomes so workers can finish before the caller drains them.
	outcomes := make(chan creationOutcome, 12)
	var workers sync.WaitGroup
	for i := range 12 {
		workers.Go(func() {
			<-start
			result, err := stores[i%2].CreateOnce(ctx, "user", "key", contactsql.Input{Name: "Ana"})
			outcomes <- creationOutcome{result, err}
		})
	}
	close(start)
	workers.Wait()
	close(outcomes)
	fresh, replayed := 0, 0
	var id int64
	for outcome := range outcomes {
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if id == 0 {
			id = outcome.result.Contact.ID
		}
		if outcome.result.Contact.ID != id || outcome.result.Contact.Name != "Ana" {
			t.Fatalf("different concurrent result: %+v", outcome.result)
		}
		if outcome.result.Replayed {
			replayed++
		} else {
			fresh++
		}
	}
	if fresh != 1 || replayed != 11 {
		t.Fatalf("fresh=%d replayed=%d", fresh, replayed)
	}
	assertIdempotencyCounts(t, db, 1, 1)
}

// Race different payloads under one caller/key. One request wins the binding;
// the other must conflict, without creating a second business row.
func testConcurrentCreationConflict(t *testing.T, db *sql.DB, dsn string) {
	store := idempotencyStore(t, db, dsn)
	start := make(chan struct{})
	outcomes := make(chan creationOutcome, 2)
	var workers sync.WaitGroup
	for _, name := range []string{"Ana", "Bea"} {
		workers.Go(func() {
			<-start
			result, err := store.CreateOnce(t.Context(), "user", "key", contactsql.Input{Name: name})
			outcomes <- creationOutcome{result, err}
		})
	}
	close(start)
	workers.Wait()
	close(outcomes)
	success, conflicts := 0, 0
	for outcome := range outcomes {
		switch {
		case outcome.err == nil && !outcome.result.Replayed:
			success++
		case errors.Is(outcome.err, contactidem.ErrKeyReuse):
			conflicts++
		default:
			t.Fatalf("unexpected competing outcome: %+v", outcome)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflicts=%d", success, conflicts)
	}
	assertIdempotencyCounts(t, db, 1, 1)
}

// A saved creation is historical data. Editing/deleting the contact must not
// alter its replayed result or make the old key recreate the missing business row.
func testHistoricalCreationReplay(t *testing.T, db *sql.DB, dsn string) {
	store := idempotencyStore(t, db, dsn)
	email := "original@example.com"
	input := contactsql.Input{Name: "Ana", Email: &email}
	first, err := store.CreateOnce(t.Context(), "user", "key", input)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"UPDATE sql_idem_contacts SET name = 'Edited', email = NULL WHERE id = $1",
		"DELETE FROM sql_idem_contacts WHERE id = $1",
	} {
		if _, err := db.ExecContext(t.Context(), statement, first.Contact.ID); err != nil {
			t.Fatal(err)
		}
		again, err := store.CreateOnce(t.Context(), "user", "key", input)
		if err != nil || !again.Replayed {
			t.Fatalf("historical replay = (%+v, %v)", again, err)
		}
		assertCreationContact(t, again.Contact, first.Contact)
	}
	assertIdempotencyCounts(t, db, 0, 1)
}

// Run the finite demo beside another caller's retained result. Demo cleanup
// must remove its own data while leaving the other operation replayable.
func testIdempotencyDemo(t *testing.T, db *sql.DB, dsn string) {
	store := idempotencyStore(t, db, dsn)
	input := contactsql.Input{Name: "Keep this contact"}
	first, err := store.CreateOnce(t.Context(), "other-user", "key", input)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := idempotencydemo.Run(t.Context(), dsn, &output); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"one contact created", "ErrKeyReuse", "New pool", "contact stays absent"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("demo output missing %q", text)
		}
	}
	again, err := store.CreateOnce(t.Context(), "other-user", "key", input)
	if err != nil || !again.Replayed {
		t.Fatalf("other caller replay = (%+v, %v)", again, err)
	}
	assertCreationContact(t, again.Contact, first.Contact)
	assertIdempotencyCounts(t, db, 1, 1)
}
