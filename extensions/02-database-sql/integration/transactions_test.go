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

func resetTransactionTables(t *testing.T, db *sql.DB) {
	t.Helper()
	// This database belongs only to this test. Drops reset migration history as
	// well as business state; ordinary examples never run this reset operation.
	_, err := db.ExecContext(t.Context(), "DROP TABLE IF EXISTS sql_tx_contact_changes, sql_tx_contacts, sql_tx_migrations, sql_tx_schema_migrations CASCADE")
	if err != nil {
		t.Fatal(err)
	}
}

func migrateTransactions(t *testing.T, dsn string) {
	t.Helper()
	version, err := contacttx.Migrate(dsn)
	if err != nil || version != contacttx.LatestVersion {
		t.Fatalf("migrate = (%d, %v)", version, err)
	}
}

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

func testMigrationFailure(t *testing.T, db *sql.DB, dsn string) {
	// Version 1 commits before version 2 starts. A conflicting table makes the
	// second file fail: its ALTER rolls back, while version 1 remains committed.
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

func createVersionedContact(t *testing.T, db *sql.DB) contacttx.Contact {
	t.Helper()
	created, err := contacttx.NewStore(db).CreateBatch(t.Context(), []contactsql.Input{{Name: "Original"}})
	if err != nil {
		t.Fatal(err)
	}
	return created[0]
}

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

func changeCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sql_tx_contact_changes").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

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
