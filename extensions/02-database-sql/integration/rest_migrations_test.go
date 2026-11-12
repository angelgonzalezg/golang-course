//go:build integration

package integration_test

import (
	"database/sql"
	"errors"
	"sync"
	"testing"

	"example.com/course/sql-lab/contactidem"
	"example.com/course/sql-lab/contactpg"
	"example.com/course/sql-lab/contacttx"
	"github.com/golang-migrate/migrate/v4"
)

// Repeated/concurrent startup migrations retain their own version and earlier
// lesson versions. Unreviewed dirty/future state must fail without rewriting it.
func testRESTMigrations(t *testing.T, db *sql.DB, dsn string) {
	if _, err := contacttx.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	if _, err := contactidem.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			version, err := contactpg.Migrate(dsn)
			if err == nil && version != 1 {
				err = errors.New("unexpected service schema version")
			}
			results <- err
		})
	}
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if version, err := contacttx.Migrate(dsn); err != nil || version != 2 {
		t.Fatalf("stage 2 = %d, %v", version, err)
	}
	if version, err := contactidem.Migrate(dsn); err != nil || version != 1 {
		t.Fatalf("stage 3 = %d, %v", version, err)
	}
	for _, dirty := range []bool{true, false} {
		t.Run(map[bool]string{true: "dirty", false: "future"}[dirty], func(t *testing.T) {
			version := 99
			if dirty {
				version = 1
			}
			// Only this disposable fixture's metadata is tampered with/restored.
			defer db.ExecContext(t.Context(), "UPDATE sql_service_schema_migrations SET version = 1, dirty = false")
			if _, err := db.ExecContext(t.Context(), "UPDATE sql_service_schema_migrations SET version = $1, dirty = $2", version, dirty); err != nil {
				t.Fatal(err)
			}
			_, err := contactpg.Migrate(dsn)
			var cause migrate.ErrDirty
			if err == nil || dirty && !errors.As(err, &cause) {
				t.Fatalf("unreviewed schema accepted: %v", err)
			}
			var gotVersion int
			var gotDirty bool
			if err := db.QueryRowContext(t.Context(), "SELECT version, dirty FROM sql_service_schema_migrations").Scan(&gotVersion, &gotDirty); err != nil {
				t.Fatal(err)
			}
			if gotVersion != version || gotDirty != dirty {
				t.Fatal("migration rewrote unreviewed state")
			}
		})
	}
}
