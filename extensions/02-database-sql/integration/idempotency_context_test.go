//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"example.com/course/sql-lab/contactidem"
	"example.com/course/sql-lab/contactsql"
)

// Hold only writes to the result table. SHARE permits the initial SELECT, so
// the writer can insert a contact before its result INSERT becomes blocked.
func blockCreationResult(t *testing.T, db *sql.DB) *sql.Tx {
	t.Helper()
	blocker, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { blocker.Rollback() })
	if _, err := blocker.ExecContext(t.Context(), "LOCK TABLE sql_idem_create_requests IN SHARE MODE"); err != nil {
		t.Fatal(err)
	}
	return blocker
}

// Keep outcomes buffered so cleanup can join a worker even after an assertion
// fails before the test reads its result. Each caller still owns cancellation.
func startCreation(ctx context.Context, store *contactidem.Store) (<-chan creationOutcome, <-chan struct{}) {
	outcome, done := make(chan creationOutcome, 1), make(chan struct{})
	go func() {
		defer close(done)
		result, err := store.CreateOnce(ctx, "user", "key", contactsql.Input{Name: "Ana"})
		outcome <- creationOutcome{result, err}
	}()
	return outcome, done
}

// Wait for PostgreSQL to report an actual blocked statement. The ticker polls
// state; its duration does not choose when the test assumes work has started.
func waitForCreationQuery(t *testing.T, db *sql.DB, fragment string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity WHERE datname = current_database()
			AND wait_event_type = 'Lock' AND query LIKE $1
		)`, "%"+fragment+"%").Scan(&waiting); err != nil {
			t.Fatalf("observe blocked creation: %v", err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("no blocked creation statement containing %q", fragment)
		case <-ticker.C:
		}
	}
}

// Join an owned asynchronous create. Five seconds is a failure watchdog, not
// a guess about when an INSERT or cancellation has reached the server.
func awaitCreationDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("creation worker did not finish")
	}
}

// Cancel only the duplicate while an original create is blocked saving its
// result. The original must still commit, and a later retry must replay it.
func testCanceledCreationWaiter(t *testing.T, db *sql.DB, dsn string) {
	store := idempotencyStore(t, db, dsn)
	blocker := blockCreationResult(t, db)
	leaderCtx, stopLeader := context.WithCancel(t.Context())
	defer stopLeader()
	leader, leaderDone := startCreation(leaderCtx, store)
	t.Cleanup(func() {
		stopLeader()
		blocker.Rollback()
		awaitCreationDone(t, leaderDone)
	})
	waitForCreationQuery(t, db, "INSERT INTO sql_idem_create_requests")
	duplicateCtx, stopDuplicate := context.WithCancel(t.Context())
	defer stopDuplicate()
	duplicate, duplicateDone := startCreation(duplicateCtx, store)
	t.Cleanup(func() {
		stopDuplicate()
		awaitCreationDone(t, duplicateDone)
	})
	// The duplicate must be waiting on the key, not failing at name uniqueness
	// or running its own business insert before the original has finished.
	waitForCreationQuery(t, db, "pg_advisory_xact_lock")
	stopDuplicate()
	awaitCreationDone(t, duplicateDone)
	if outcome := <-duplicate; !errors.Is(outcome.err, context.Canceled) {
		t.Fatalf("canceled duplicate = %+v", outcome)
	}
	select {
	case <-leaderDone:
		t.Fatal("canceling the duplicate ended the blocked original")
	default:
	}
	if err := blocker.Rollback(); err != nil {
		t.Fatal(err)
	}
	awaitCreationDone(t, leaderDone)
	original := <-leader
	if original.err != nil || original.result.Replayed {
		t.Fatalf("original create = %+v", original)
	}
	again, err := store.CreateOnce(t.Context(), "user", "key", contactsql.Input{Name: "Ana"})
	if err != nil || !again.Replayed {
		t.Fatalf("retry after waiter cancellation = (%+v, %v)", again, err)
	}
	assertCreationContact(t, again.Contact, original.result.Contact)
	assertIdempotencyCounts(t, db, 1, 1)
}

// Observe a blocked result INSERT, proving the contact INSERT was reached,
// then cancel its context. Both rows must roll back and leave the key reusable.
func testCanceledCreationWrite(t *testing.T, db *sql.DB, dsn string) {
	store := idempotencyStore(t, db, dsn)
	blocker := blockCreationResult(t, db)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	outcome, done := startCreation(ctx, store)
	t.Cleanup(func() {
		cancel()
		blocker.Rollback()
		awaitCreationDone(t, done)
	})
	waitForCreationQuery(t, db, "INSERT INTO sql_idem_create_requests")
	cancel()
	awaitCreationDone(t, done)
	if got := <-outcome; !errors.Is(got.err, context.Canceled) {
		t.Fatalf("canceled result write = %+v", got)
	}
	if err := blocker.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertIdempotencyCounts(t, db, 0, 0)
	if got, err := store.CreateOnce(t.Context(), "user", "key", contactsql.Input{Name: "Ana"}); err != nil || got.Replayed {
		t.Fatalf("retry after canceled write = (%+v, %v)", got, err)
	}
	assertIdempotencyCounts(t, db, 1, 1)
}
