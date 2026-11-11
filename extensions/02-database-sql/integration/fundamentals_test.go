//go:build integration

// Integration tests require a Docker-compatible engine. Explicitly selecting
// this tag makes a missing engine a failure rather than a silently skipped test.
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
	"example.com/course/sql-lab/contactsqlc"
	"example.com/course/sql-lab/internal/demo"
)

// Run the same contracts against the manual and generated-query stores. One
// private PostgreSQL database serves serial cases, each with its own empty table.
func TestPostgreSQL(t *testing.T) {
	// One disposable database serves serial cases. Each case starts with an
	// empty table; no case uses DATABASE_URL or a learner's persistent database.
	db, dsn := newTestDatabase(t)
	if err := contactsql.EnsureSchema(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	for _, backend := range []struct {
		name     string
		newStore storeFactory
	}{
		{"manual", func(db *sql.DB) contactStore { return contactsql.NewStore(db) }},
		{"sqlc", func(db *sql.DB) contactStore { return contactsqlc.NewStore(db) }},
	} {
		t.Run(backend.name, func(t *testing.T) {
			for _, test := range []struct {
				name string
				run  func(*testing.T, *sql.DB, string, storeFactory)
			}{
				{"CRUD and NULL", testCRUD},
				{"repeated email assignments", testRepeatedEmailAssignments},
				{"parameters remain data", testParameters},
				{"data survives pool closure", testReconnect},
				{"concurrent unique names", testConcurrentCreate},
				{"pool wait cancellation", testPoolCancellation},
				{"validation and error causes", testStoreErrors},
				{"independent result values", testStoreResultOwnership},
			} {
				t.Run(test.name, func(t *testing.T) {
					resetFundamentalsTable(t, db)
					test.run(t, db, dsn, backend.newStore)
				})
			}
		})
	}
	for _, test := range []struct {
		name string
		run  func(*testing.T, *sql.DB, string)
	}{
		{"finite fundamentals demonstration", testDemo},
		{"generated transaction binding", testSQLCTransactions},
		{"finite comparison preserves other rows", testSQLCDemo},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetFundamentalsTable(t, db)
			test.run(t, db, dsn)
		})
	}
}

// Follow create/read/update/delete with real rows. Check NULL versus empty
// email, bounded ID ordering, name conflicts, and absence after repeated deletion.
func testCRUD(t *testing.T, db *sql.DB, _ string, newStore storeFactory) {
	ctx := t.Context()
	store := newStore(db)
	page, err := store.List(ctx, 2)
	if err != nil || page == nil || len(page) != 0 {
		t.Fatalf("empty page = (%v, %v)", page, err)
	}
	ana, err := store.Create(ctx, contactsql.Input{Name: " Ana "})
	if err != nil || ana.ID <= 0 || ana.Name != "Ana" || ana.Email != nil {
		t.Fatalf("create = (%+v, %v)", ana, err)
	}
	empty := ""
	bea, err := store.Create(ctx, contactsql.Input{Name: "Bea", Email: &empty})
	if err != nil || bea.Email == nil || *bea.Email != "" {
		t.Fatalf("empty email = (%+v, %v)", bea, err)
	}
	read, err := store.Get(ctx, bea.ID)
	if err != nil || read.Email == nil || *read.Email != "" {
		t.Fatalf("read empty email = (%+v, %v)", read, err)
	}
	email := "ana@example.com"
	updated, err := store.UpdateEmail(ctx, ana.ID, &email)
	if err != nil || updated.Email == nil || *updated.Email != email {
		t.Fatalf("update = (%+v, %v)", updated, err)
	}
	updated, err = store.UpdateEmail(ctx, ana.ID, nil)
	if err != nil || updated.Email != nil {
		t.Fatalf("clear email = (%+v, %v)", updated, err)
	}
	page, err = store.List(ctx, 1)
	if err != nil || len(page) != 1 || page[0].ID != ana.ID {
		t.Fatalf("bounded page = (%v, %v)", page, err)
	}
	if _, err := store.Create(ctx, contactsql.Input{Name: "Ana"}); !errors.Is(err, contactsql.ErrNameTaken) {
		t.Fatalf("duplicate = %v", err)
	}
	if err := store.Delete(ctx, ana.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, ana.ID); !errors.Is(err, contactsql.ErrNotFound) {
		t.Fatalf("missing get = %v", err)
	}
	if _, err := store.UpdateEmail(ctx, ana.ID, nil); !errors.Is(err, contactsql.ErrNotFound) {
		t.Fatalf("missing update = %v", err)
	}
	if err := store.Delete(ctx, ana.ID); !errors.Is(err, contactsql.ErrNotFound) {
		t.Fatalf("missing delete = %v", err)
	}
	if _, err := store.Get(ctx, ana.ID); !errors.Is(err, contactsql.ErrNotFound) {
		t.Fatalf("contact must stay absent after repeated deletion: %v", err)
	}
}

// Assign nonempty text, empty text, and NULL twice each. Returned values and
// fresh reads must keep the same identity/state, leaving exactly one contact.
func testRepeatedEmailAssignments(t *testing.T, db *sql.DB, _ string, newStore storeFactory) {
	email, empty := "after@example.com", ""
	for _, test := range []struct {
		name  string
		email *string
	}{
		{name: "nonempty value", email: &email},
		{name: "empty text", email: &empty},
		{name: "NULL", email: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			// These serial cases own an isolated Testcontainers table. Start each
			// with a different email so a missing first update cannot pass.
			if _, err := db.ExecContext(ctx, "TRUNCATE sql_lab_contacts RESTART IDENTITY"); err != nil {
				t.Fatal(err)
			}
			store := newStore(db)
			before := "before@example.com"
			created, err := store.Create(ctx, contactsql.Input{Name: "Ana", Email: &before})
			if err != nil {
				t.Fatal(err)
			}
			checkContact := func(got contactsql.Contact) {
				t.Helper()
				if got.ID != created.ID || got.Name != created.Name {
					t.Fatalf("email assignment changed identity or name: %+v", got)
				}
				if test.email == nil {
					if got.Email != nil {
						t.Fatalf("email = %q, want SQL NULL", *got.Email)
					}
				} else {
					if got.Email == nil {
						t.Fatalf("email = SQL NULL, want %q", *test.email)
					}
					if *got.Email != *test.email {
						t.Fatalf("email = %q, want %q", *got.Email, *test.email)
					}
				}
			}
			// Repeating an assignment preserves its intended contact state.
			// PostgreSQL may execute UPDATE again; this is not request replay.
			for attempt := 1; attempt <= 2; attempt++ {
				updated, err := store.UpdateEmail(ctx, created.ID, test.email)
				if err != nil {
					t.Fatalf("assignment %d: %v", attempt, err)
				}
				checkContact(updated)
				stored, err := store.Get(ctx, created.ID)
				if err != nil {
					t.Fatal(err)
				}
				checkContact(stored)
			}
			page, err := store.List(ctx, 2)
			if err != nil || len(page) != 1 {
				t.Fatalf("repeated assignment must leave one contact: (%v, %v)", page, err)
			}
			checkContact(page[0])
		})
	}
}

// Store a name that looks like a DROP TABLE command. Parameter binding must
// preserve it as literal text, and later inserts must prove the table still exists.
func testParameters(t *testing.T, db *sql.DB, _ string, newStore storeFactory) {
	store := newStore(db)
	name := "Robert'); DROP TABLE sql_lab_contacts; --"
	created, err := store.Create(t.Context(), contactsql.Input{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(t.Context(), created.ID)
	if err != nil || got.Name != name {
		t.Fatalf("literal name = (%+v, %v)", got, err)
	}
	if _, err := store.Create(t.Context(), contactsql.Input{Name: "Still here"}); err != nil {
		t.Fatal(err)
	}
}

// Create through one Go pool, close it, and read through a newly opened pool.
// The row must survive because PostgreSQL owns the data, not the client pool.
func testReconnect(t *testing.T, _ *sql.DB, dsn string, newStore storeFactory) {
	first, err := contactsql.Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { first.Close() })
	created, err := newStore(first).Create(t.Context(), contactsql.Input{Name: "Persistent"})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := contactsql.Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { second.Close() })
	got, err := newStore(second).Get(t.Context(), created.ID)
	if err != nil || got.Name != created.Name {
		t.Fatalf("reconnected read = (%+v, %v)", got, err)
	}
}

// Release six inserts with one name together. PostgreSQL must allow one row
// and return five name conflicts, regardless of which goroutine wins.
func testConcurrentCreate(t *testing.T, db *sql.DB, _ string, newStore storeFactory) {
	store := newStore(db)
	start := make(chan struct{})
	results := make(chan error, 6)
	var workers sync.WaitGroup
	for range 6 {
		workers.Go(func() {
			<-start
			_, err := store.Create(t.Context(), contactsql.Input{Name: "One name"})
			results <- err
		})
	}
	close(start)
	workers.Wait()
	close(results)
	created, conflicts := 0, 0
	for err := range results {
		if err == nil {
			created++
		} else if errors.Is(err, contactsql.ErrNameTaken) {
			conflicts++
		} else {
			t.Errorf("unexpected create error: %v", err)
		}
	}
	if created != 1 || conflicts != 5 {
		t.Fatalf("created=%d conflicts=%d", created, conflicts)
	}
	page, err := store.List(t.Context(), 10)
	if err != nil || len(page) != 1 {
		t.Fatalf("concurrent result = (%v, %v)", page, err)
	}
}

// Reserve the pool's only connection so List must wait for capacity. Its
// deadline must stop that wait, and releasing the connection must restore normal reads.
func testPoolCancellation(t *testing.T, db *sql.DB, _ string, newStore storeFactory) {
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.SetMaxOpenConns(4) })
	// Reserve the only connection. A query must now wait in sql.DB's queue;
	// the deadline, rather than a sleep or a PostgreSQL timing assumption, ends it.
	reserved, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reserved.Close() })
	before := db.Stats().WaitCount
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err = newStore(db).List(ctx, 10)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting query = %v", err)
	}
	if db.Stats().WaitCount <= before {
		t.Fatal("query did not wait for the reserved connection")
	}
	if err := reserved.Close(); err != nil {
		t.Fatal(err)
	}
	// Cancellation while waiting did not write a contact or break later reads.
	page, err := newStore(db).List(t.Context(), 10)
	if err != nil || len(page) != 0 {
		t.Fatalf("read after cancellation = (%v, %v)", page, err)
	}
}

// Run the finite SQL demo against this isolated database. Expected summaries
// must appear, and its own contact must be removed before the demo returns.
func testDemo(t *testing.T, db *sql.DB, dsn string) {
	var output bytes.Buffer
	if err := demo.Run(t.Context(), dsn, &output); err != nil {
		t.Fatal(err)
	}
	for _, summary := range []string{"SQL NULL", "ErrNameTaken", "context.Canceled", "different pool", "ErrNotFound"} {
		if !strings.Contains(output.String(), summary) {
			t.Errorf("demo output missing %q", summary)
		}
	}
	page, err := contactsql.NewStore(db).List(t.Context(), 10)
	if err != nil || len(page) != 0 {
		t.Fatalf("demo cleanup = (%v, %v)", page, err)
	}
}
