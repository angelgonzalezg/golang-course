//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/sql-lab/contactpg"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Report backend completion separately from the client status: a client can
// observe cancellation before the server has finished rolling its transaction back.
type observedBackend struct {
	*contactpg.Store
	finished chan error
}

func (s observedBackend) CreateOnce(ctx context.Context, subject, key string, input contacts.Input) (contacts.CreationResult, error) {
	result, err := s.Store.CreateOnce(ctx, subject, key, input)
	s.finished <- err
	return result, err
}

// Poll actual PostgreSQL lock state instead of sleeping to guess when a write
// starts. The watchdog bounds failure time; it does not schedule the cancellation.
func waitBlocked(t *testing.T, db *sql.DB, fragment string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE $1)`, "%"+fragment+"%").Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("no blocked statement containing %q", fragment)
		case <-ticker.C:
		}
	}
}

// Join one owned client worker, including during assertion-failure cleanup.
func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RPC worker did not finish")
	}
}

// Observe SQL rollback completion before checking committed effects.
func backendFinished(t *testing.T, finished <-chan error) error {
	t.Helper()
	select {
	case err := <-finished:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("backend did not finish")
		return nil
	}
}

// Cancellation/deadlines propagate through TLS, middleware, pool/lock waits,
// and the transaction. Canceling only a duplicate must preserve its original.
func testCancellation(t *testing.T, db *sql.DB, _ string) {
	for _, test := range []struct {
		name                string
		duplicate, deadline bool
	}{{"cancel original", false, false}, {"cancel duplicate", true, false}, {"deadline original", false, true}} {
		t.Run(test.name, func(t *testing.T) {
			resetService(t, db)
			backend := observedBackend{Store: contactpg.NewStore(db), finished: make(chan error, 4)}
			h := newEndpoint(t, backend, false, nil)
			blocker, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback()
			// SHARE permits the result SELECT but blocks its INSERT after the
			// contact INSERT. This creates a controlled pre-commit cancellation point.
			if _, err := blocker.ExecContext(t.Context(), "LOCK TABLE sql_service_create_requests IN SHARE MODE"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if test.deadline {
				cancel()
				ctx, cancel = context.WithTimeout(t.Context(), 500*time.Millisecond)
				defer cancel()
			}
			original, done := make(chan createOutcome, 1), make(chan struct{})
			go func() {
				defer close(done)
				value, md, err := invokeCreate(ctx, h.api, "key", &contactsv1.ContactInput{Name: "Ana"})
				original <- createOutcome{value, md, err}
			}()
			t.Cleanup(func() { cancel(); blocker.Rollback(); waitDone(t, done) })
			waitBlocked(t, db, "INSERT INTO sql_service_create_requests")
			if test.duplicate {
				waitCtx, stopWait := context.WithCancel(t.Context())
				defer stopWait()
				waiter, waiterDone := make(chan error, 1), make(chan struct{})
				go func() {
					defer close(waiterDone)
					_, _, err := invokeCreate(waitCtx, h.api, "key", &contactsv1.ContactInput{Name: "Ana"})
					waiter <- err
				}()
				t.Cleanup(func() { stopWait(); waitDone(t, waiterDone) })
				waitBlocked(t, db, "pg_advisory_xact_lock")
				stopWait()
				waitDone(t, waiterDone)
				if err := <-waiter; status.Code(err) != codes.Canceled {
					t.Fatalf("duplicate cancellation=%v", err)
				}
				if err := backendFinished(t, backend.finished); !errors.Is(err, context.Canceled) {
					t.Fatalf("duplicate backend=%v", err)
				}
				select {
				case <-done:
					t.Fatal("canceling duplicate ended original")
				default:
				}
				if err := blocker.Rollback(); err != nil {
					t.Fatal(err)
				}
				waitDone(t, done)
				if got := <-original; got.err != nil {
					t.Fatal(got.err)
				}
				if err := backendFinished(t, backend.finished); err != nil {
					t.Fatal(err)
				}
				assertCounts(t, db, 1, 1)
			} else {
				if !test.deadline {
					cancel()
				}
				waitDone(t, done)
				want := codes.Canceled
				if test.deadline {
					want = codes.DeadlineExceeded
				}
				if got := <-original; status.Code(got.err) != want {
					t.Fatalf("client cancellation=%v,want %s", got.err, want)
				}
				if err := backendFinished(t, backend.finished); !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("server cancellation=%v", err)
				}
				if err := blocker.Rollback(); err != nil {
					t.Fatal(err)
				}
				assertCounts(t, db, 0, 0)
			}
			_, md, err := invokeCreate(t.Context(), h.api, "key", &contactsv1.ContactInput{Name: "Ana"})
			if err != nil {
				t.Fatal(err)
			}
			wantReplay := "false"
			if test.duplicate {
				wantReplay = "true"
			}
			if values := md.Get("x-idempotency-replayed"); len(values) != 1 || values[0] != wantReplay {
				t.Fatalf("retry metadata=%v,want %s", md, wantReplay)
			}
			assertCounts(t, db, 1, 1)
		})
	}
}
