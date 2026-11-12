//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"example.com/course/sql-lab/contactpg"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

type serviceOutcome struct {
	result contacts.CreationResult
	err    error
}

// Keep a result buffered and expose completion so cleanup can cancel and join
// every owned worker, including when an assertion fails before receiving it.
func startServiceCreate(ctx context.Context, store *contactpg.Store) (<-chan serviceOutcome, <-chan struct{}) {
	outcome, done := make(chan serviceOutcome, 1), make(chan struct{})
	go func() {
		defer close(done)
		result, err := store.CreateOnce(ctx, "user", "key", contacts.Input{Name: "Ana"})
		outcome <- serviceOutcome{result, err}
	}()
	return outcome, done
}

// Observe real server lock waits before canceling. A canceled original must
// roll back its contact/result; canceling only a duplicate must preserve its leader.
func testRESTCancellation(t *testing.T, db *sql.DB, _ string) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel original after insert", true: "cancel duplicate waiting for key"}[duplicate], func(t *testing.T) {
			if _, err := db.ExecContext(t.Context(), "TRUNCATE sql_service_create_requests, sql_service_contacts RESTART IDENTITY"); err != nil {
				t.Fatal(err)
			}
			store := contactpg.NewStore(db)
			blocker, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback()
			// SHARE allows the saved-result SELECT but blocks its INSERT, after
			// the business INSERT has already executed in the writer's transaction.
			if _, err := blocker.ExecContext(t.Context(), "LOCK TABLE sql_service_create_requests IN SHARE MODE"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			original, originalDone := startServiceCreate(ctx, store)
			t.Cleanup(func() { cancel(); blocker.Rollback(); awaitCreationDone(t, originalDone) })
			waitForCreationQuery(t, db, "INSERT INTO sql_service_create_requests")
			if duplicate {
				waitCtx, stopWait := context.WithCancel(t.Context())
				defer stopWait()
				waiter, waiterDone := startServiceCreate(waitCtx, store)
				t.Cleanup(func() { stopWait(); awaitCreationDone(t, waiterDone) })
				waitForCreationQuery(t, db, "pg_advisory_xact_lock")
				stopWait()
				awaitCreationDone(t, waiterDone)
				if got := <-waiter; !errors.Is(got.err, context.Canceled) {
					t.Fatalf("canceled duplicate = %+v", got)
				}
				if err := blocker.Rollback(); err != nil {
					t.Fatal(err)
				}
				awaitCreationDone(t, originalDone)
				if got := <-original; got.err != nil || got.result.Replayed {
					t.Fatalf("original create = %+v", got)
				}
				assertServiceCounts(t, db, 1, 1)
			} else {
				cancel()
				awaitCreationDone(t, originalDone)
				if got := <-original; !errors.Is(got.err, context.Canceled) {
					t.Fatalf("canceled writer = %+v", got)
				}
				if err := blocker.Rollback(); err != nil {
					t.Fatal(err)
				}
				assertServiceCounts(t, db, 0, 0)
			}
			retry, err := store.CreateOnce(t.Context(), "user", "key", contacts.Input{Name: "Ana"})
			if err != nil || retry.Replayed != duplicate {
				t.Fatalf("retry after cancellation = %+v, %v", retry, err)
			}
			assertServiceCounts(t, db, 1, 1)
		})
	}
	// Row-lock waiting is also cancellable for conditional mutations.
	t.Run("cancel mutation waiting for row", func(t *testing.T) {
		store := contactpg.NewStore(db)
		before, err := store.Create(t.Context(), contacts.Input{Name: "Row-lock fixture"})
		if err != nil {
			t.Fatal(err)
		}
		blocker, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer blocker.Rollback()
		if _, err := blocker.ExecContext(t.Context(), "SELECT id FROM sql_service_contacts WHERE id = $1 FOR UPDATE", before.ID); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done, outcome := make(chan struct{}), make(chan error, 1)
		go func() {
			defer close(done)
			email := "canceled"
			_, err := store.Patch(ctx, before.ID, contacts.Changes{Email: &email})
			outcome <- err
		}()
		t.Cleanup(func() { cancel(); blocker.Rollback(); awaitCreationDone(t, done) })
		waitForCreationQuery(t, db, "FOR UPDATE")
		cancel()
		awaitCreationDone(t, done)
		if err := <-outcome; !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled mutation = %v", err)
		}
		if err := blocker.Rollback(); err != nil {
			t.Fatal(err)
		}
		after, err := store.Get(t.Context(), before.ID)
		if err != nil || after != before {
			t.Fatalf("canceled PATCH changed row: %+v, %v", after, err)
		}
	})
}
