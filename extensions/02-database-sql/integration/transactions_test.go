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

	"example.com/course/sql-lab/contactsql"
	"example.com/course/sql-lab/contacttx"
	"example.com/course/sql-lab/internal/txdemo"
	"github.com/golang-migrate/migrate/v4"
)

// Run stage-2 schema and business-transaction scenarios on one disposable server.
// Reset the lesson's tables before each serial subtest to isolate history and rows.
func TestTransactions(t *testing.T) {
	db, dsn := newTestDatabase(t)
	for _, test := range []struct {
		name string
		run  func(*testing.T, *sql.DB, string)
	}{
		{"upgrade preserves rows and repeated migrations", testMigrationUpgrade},
		{"failed migration is atomic and dirty", testMigrationFailure},
		{"dirty and future migration state", testMigrationHistory},
		{"concurrent migrators", testConcurrentMigrations},
		{"explicit legacy adoption preserves rows", testLegacyMigrations},
		{"up down up in an isolated database", testMigrationRoundTrip},
		{"batch commit and rollback", testBatchTransactions},
		{"history failure rolls back replacement", testHistoryFailure},
		{"concurrent version checks", testVersionRace},
		{"cancellation after update rolls back", testTransactionCancellation},
		{"transaction demonstration", testTransactionDemo},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetTransactionTables(t, db)
			test.run(t, db, dsn)
		})
	}
}

// resetTransactionTables clears migration history and business state inside
// this suite's disposable database. Ordinary examples never run this reset.
func resetTransactionTables(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), "DROP TABLE IF EXISTS sql_tx_contact_changes, sql_tx_contacts, sql_tx_migrations, sql_tx_schema_migrations CASCADE")
	if err != nil {
		t.Fatal(err)
	}
}

// migrateTransactions prepares the latest schema and fails the calling subtest
// immediately if setup did not reach the version required by its business operations.
func migrateTransactions(t *testing.T, dsn string) {
	t.Helper()
	version, err := contacttx.Migrate(dsn)
	if err != nil || version != contacttx.LatestVersion {
		t.Fatalf("migrate = (%d, %v)", version, err)
	}
}

// Insert a row at schema version 1, upgrade, then repeat migration. Preserve
// the row, add its default row version, keep one clean schema record, and reject downgrade.
func testMigrationUpgrade(t *testing.T, db *sql.DB, dsn string) {
	version, err := contacttx.MigrateTo(dsn, 1)
	if err != nil || version != 1 {
		t.Fatalf("migrate to 1 = (%d, %v)", version, err)
	}
	var id int64
	if err := db.QueryRowContext(t.Context(), "INSERT INTO sql_tx_contacts (name) VALUES ('Before upgrade') RETURNING id").Scan(&id); err != nil {
		t.Fatal(err)
	}
	migrateTransactions(t, dsn)
	got, err := contacttx.NewStore(db).Get(t.Context(), id)
	if err != nil || got.Name != "Before upgrade" || got.Version != 1 {
		t.Fatalf("upgraded row = (%+v, %v)", got, err)
	}
	migrateTransactions(t, dsn)
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sql_tx_schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("version table rows = %d", count)
	}
	assertMigrationState(t, db, 2, false)
	if _, err := contacttx.MigrateTo(dsn, 1); !errors.Is(err, contacttx.ErrMigrationState) {
		t.Fatalf("backward target = %v", err)
	}
}

// Force version 2 to fail after version 1 committed. Roll back version 2's DDL,
// retain version 1, and leave dirty state that blocks another migration attempt.
func testMigrationFailure(t *testing.T, db *sql.DB, dsn string) {
	// Pre-create the history table so version 2's CREATE TABLE fails after its
	// ALTER has been reached. This forces the transaction's rollback path.
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE sql_tx_contact_changes (id bigint)"); err != nil {
		t.Fatal(err)
	}
	if _, err := contacttx.Migrate(dsn); err == nil {
		t.Fatal("conflicting migration unexpectedly succeeded")
	}
	var contactsRemain, versionAbsent bool
	err := db.QueryRowContext(t.Context(), `SELECT to_regclass('public.sql_tx_contacts') IS NOT NULL,
		NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public'
		AND table_name = 'sql_tx_contacts' AND column_name = 'version')`).Scan(&contactsRemain, &versionAbsent)
	if err != nil || !contactsRemain || !versionAbsent {
		t.Fatalf("migration rollback = (%v, %v, %v)", contactsRemain, versionAbsent, err)
	}
	assertMigrationState(t, db, 2, true)
	var dirty migrate.ErrDirty
	if _, err := contacttx.Migrate(dsn); !errors.As(err, &dirty) || dirty.Version != 2 {
		t.Fatalf("dirty migration was not rejected: %v", err)
	}
}

// assertMigrationState checks the library's persisted version/dirty marker.
// Business-row versions are separate and cannot establish migration completion.
func assertMigrationState(t *testing.T, db *sql.DB, wantVersion int, wantDirty bool) {
	t.Helper()
	var version int
	var dirty bool
	if err := db.QueryRowContext(t.Context(), "SELECT version, dirty FROM sql_tx_schema_migrations").Scan(&version, &dirty); err != nil {
		t.Fatal(err)
	}
	if version != wantVersion || dirty != wantDirty {
		t.Fatalf("migration state = (%d, %v), want (%d, %v)", version, dirty, wantVersion, wantDirty)
	}
}

// Mark a schema dirty or give it an unknown future version. The adapter must
// stop with the corresponding error instead of clearing state or migrating backward.
func testMigrationHistory(t *testing.T, db *sql.DB, dsn string) {
	for _, test := range []struct{ name, statement string }{
		{"dirty", "UPDATE sql_tx_schema_migrations SET dirty = true"},
		{"future version", "UPDATE sql_tx_schema_migrations SET version = 99"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetTransactionTables(t, db)
			migrateTransactions(t, dsn)
			if _, err := db.ExecContext(t.Context(), test.statement); err != nil {
				t.Fatal(err)
			}
			_, err := contacttx.Migrate(dsn)
			var dirty migrate.ErrDirty
			if test.name == "dirty" && !errors.As(err, &dirty) || test.name != "dirty" && !errors.Is(err, contacttx.ErrMigrationState) {
				t.Fatalf("invalid history = %v", err)
			}
		})
	}
}

// Release four migrators together. Cooperative locking must let all finish
// at version 2 with one clean version-table row, without duplicate DDL execution.
func testConcurrentMigrations(t *testing.T, db *sql.DB, dsn string) {
	start := make(chan struct{})
	results := make(chan error, 4)
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			<-start
			version, err := contacttx.Migrate(dsn)
			if err == nil && version != 2 {
				err = errors.New("unexpected schema version")
			}
			results <- err
		})
	}
	close(start)
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Error(err)
		}
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sql_tx_schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migration records = %d", count)
	}
	assertMigrationState(t, db, 2, false)
}

// createVersionedContact seeds a committed version-1 row named Original.
// Later tests can distinguish a successful replacement from a rolled-back edit.
func createVersionedContact(t *testing.T, db *sql.DB) contacttx.Contact {
	t.Helper()
	created, err := contacttx.NewStore(db).CreateBatch(t.Context(), []contactsql.Input{{Name: "Original"}})
	if err != nil {
		t.Fatal(err)
	}
	return created[0]
}

// Fail the second insert of one batch, then commit another valid batch. The
// failed batch must leave no tentative rows/results; the valid batch must save both.
func testBatchTransactions(t *testing.T, db *sql.DB, dsn string) {
	migrateTransactions(t, dsn)
	store := contacttx.NewStore(db)
	initial := createVersionedContact(t, db)
	created, err := store.CreateBatch(t.Context(), []contactsql.Input{{Name: "Rolled back"}, {Name: initial.Name}})
	if !errors.Is(err, contactsql.ErrNameTaken) || created != nil {
		t.Fatalf("failed batch = (%v, %v)", created, err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sql_tx_contacts").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("failed batch left %d rows", count)
	}
	created, err = store.CreateBatch(t.Context(), []contactsql.Input{{Name: "Ana"}, {Name: "Bea"}})
	if err != nil || len(created) != 2 {
		t.Fatalf("committed batch = (%v, %v)", created, err)
	}
	for _, contact := range created {
		if _, err := store.Get(t.Context(), contact.ID); err != nil {
			t.Fatal(err)
		}
	}
}

// changeCount reads persisted audit rows after a transaction finishes. Tests
// use it to check that contact changes and their history committed together.
func changeCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sql_tx_contact_changes").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// Reject the history INSERT after the contact UPDATE succeeds inside its
// transaction. Neither the new fields/version nor a change record may remain.
func testHistoryFailure(t *testing.T, db *sql.DB, dsn string) {
	migrateTransactions(t, dsn)
	store := contacttx.NewStore(db)
	initial := createVersionedContact(t, db)
	// A test-only database constraint forces the second statement to fail.
	if _, err := db.ExecContext(t.Context(), "ALTER TABLE sql_tx_contact_changes ADD CONSTRAINT test_reject_history CHECK (false)"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceIfVersion(t.Context(), initial.ID, initial.Version, contactsql.Input{Name: "Tentative"}); err == nil {
		t.Fatal("history failure was ignored")
	}
	got, err := store.Get(t.Context(), initial.ID)
	if err != nil || got.Name != initial.Name || got.Version != initial.Version || changeCount(t, db) != 0 {
		t.Fatalf("failed replacement remained: (%+v, %v)", got, err)
	}
}

// Race two replacements using the same expected version. Exactly one wins;
// its fields, incremented version, and single matching history row must agree.
func testVersionRace(t *testing.T, db *sql.DB, dsn string) {
	migrateTransactions(t, dsn)
	store := contacttx.NewStore(db)
	initial := createVersionedContact(t, db)
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, name := range []string{"Writer A", "Writer B"} {
		workers.Go(func() {
			<-start
			_, err := store.ReplaceIfVersion(t.Context(), initial.ID, initial.Version, contactsql.Input{Name: name})
			results <- err
		})
	}
	close(start)
	workers.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, contacttx.ErrVersionConflict) {
			conflicts++
		} else {
			t.Error(err)
		}
	}
	got, err := store.Get(t.Context(), initial.ID)
	if err != nil || success != 1 || conflicts != 1 || got.Version != 2 || changeCount(t, db) != 1 {
		t.Fatalf("version race: success=%d conflicts=%d contact=%+v err=%v", success, conflicts, got, err)
	}
	if got.Name != "Writer A" && got.Name != "Writer B" {
		t.Fatalf("winning fields were not stored: %+v", got)
	}
	var from, to int64
	if err := db.QueryRowContext(t.Context(), "SELECT from_version, to_version FROM sql_tx_contact_changes WHERE contact_id = $1", initial.ID).Scan(&from, &to); err != nil {
		t.Fatal(err)
	}
	if from != 1 || to != got.Version {
		t.Fatalf("history transition = %d -> %d, contact version = %d", from, to, got.Version)
	}
}

// Block the history INSERT after UPDATE has run, then cancel the writer.
// The caller must observe cancellation, and both tentative writes must roll back.
func testTransactionCancellation(t *testing.T, db *sql.DB, dsn string) {
	migrateTransactions(t, dsn)
	store := contacttx.NewStore(db)
	initial := createVersionedContact(t, db)
	blocker, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err := blocker.ExecContext(t.Context(), "LOCK TABLE sql_tx_contact_changes IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	result, done := make(chan error, 1), make(chan struct{})
	t.Cleanup(func() {
		cancel()
		blocker.Rollback()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("canceled writer did not finish")
		}
	})
	go func() {
		defer close(done)
		_, err := store.ReplaceIfVersion(ctx, initial.ID, initial.Version, contactsql.Input{Name: "Canceled update"})
		result <- err
	}()
	// Observe the real server waiting on the history INSERT. Reaching that
	// statement proves UPDATE has run; no fixed sleep chooses when to cancel.
	waitForHistoryLock(t, db)
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled replacement = %v", err)
	}
	if err := blocker.Rollback(); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(t.Context(), initial.ID)
	if err != nil || got.Name != initial.Name || got.Version != 1 || changeCount(t, db) != 0 {
		t.Fatalf("canceled transaction remained: (%+v, %v)", got, err)
	}
}

// waitForHistoryLock polls the server's actual blocked INSERT state. The timer
// bounds a broken test; it does not guess how long the writer needs to reach that state.
func waitForHistoryLock(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		err := db.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity WHERE datname = current_database()
			AND wait_event_type = 'Lock' AND query LIKE '%INSERT INTO sql_tx_contact_changes%'
		)`).Scan(&waiting)
		if err != nil {
			t.Fatalf("observe waiting writer: %v", err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("writer did not reach the history lock")
		case <-ticker.C:
		}
	}
}

// Run the finite transaction walkthrough and inspect its database afterward.
// The expected lessons must be printed, with no demo contacts or history left behind.
func testTransactionDemo(t *testing.T, db *sql.DB, dsn string) {
	var output bytes.Buffer
	if err := txdemo.Run(t.Context(), dsn, &output); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Schema version: 2", "Committed 2", "prior insert rolled back", "stale version rejected"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("demo output missing %q", text)
		}
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sql_tx_contacts").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 || changeCount(t, db) != 0 {
		t.Fatal("demo left contacts or change records")
	}
}
