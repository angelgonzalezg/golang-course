//go:build integration

package integration_test

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"testing"

	"example.com/course/sql-lab/contactsql"
	"example.com/course/sql-lab/contacttx"
	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Frozen files from the old runner let adoption tests use its actual digests.
//
//go:embed testdata/legacy/*.sql
var legacyFiles embed.FS

func createLegacySchema(t *testing.T, db *sql.DB, version int) int64 {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE sql_tx_migrations (
		version integer PRIMARY KEY, checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT current_timestamp)`); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"001_contacts.sql", "002_versions_and_changes.sql"}[:version] {
		body, err := legacyFiles.ReadFile("testdata/legacy/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), string(body)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), "INSERT INTO sql_tx_migrations (version, checksum) VALUES ($1, $2)",
			i+1, fmt.Sprintf("%x", sha256.Sum256(body))); err != nil {
			t.Fatal(err)
		}
	}
	var id int64
	if err := db.QueryRowContext(t.Context(), "INSERT INTO sql_tx_contacts (name) VALUES ('Legacy contact') RETURNING id").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func testLegacyMigrations(t *testing.T, db *sql.DB, dsn string) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprintf("version %d", version), func(t *testing.T) {
			resetTransactionTables(t, db)
			id := createLegacySchema(t, db, version)
			if _, err := contacttx.Migrate(dsn); !errors.Is(err, contacttx.ErrMigrationState) {
				t.Fatalf("implicit adoption = %v", err)
			}
			if got, err := contacttx.AdoptLegacy(dsn); err != nil || got != 2 {
				t.Fatalf("adopt = (%d, %v)", got, err)
			}
			migrateTransactions(t, dsn)
			got, err := contacttx.NewStore(db).Get(t.Context(), id)
			if err != nil || got.Name != "Legacy contact" || got.Version != 1 {
				t.Fatalf("legacy row = (%+v, %v)", got, err)
			}
			assertMigrationState(t, db, 2, false)
		})
	}
	for _, statement := range []string{
		"UPDATE sql_tx_migrations SET checksum = 'changed' WHERE version = 1",
		"INSERT INTO sql_tx_migrations (version, checksum) VALUES (99, 'unknown')",
		"DELETE FROM sql_tx_migrations WHERE version = 1",
	} {
		t.Run(statement, func(t *testing.T) {
			resetTransactionTables(t, db)
			id := createLegacySchema(t, db, 2)
			if _, err := db.ExecContext(t.Context(), statement); err != nil {
				t.Fatal(err)
			}
			if _, err := contacttx.AdoptLegacy(dsn); !errors.Is(err, contacttx.ErrMigrationState) {
				t.Fatalf("untrusted adoption = %v", err)
			}
			if _, err := contacttx.NewStore(db).Get(t.Context(), id); err != nil {
				t.Fatalf("rejected adoption changed data: %v", err)
			}
			var count int
			if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sql_tx_schema_migrations").Scan(&count); err != nil || count != 0 {
				t.Fatalf("rejected baseline rows = %d, err = %v", count, err)
			}
		})
	}
}

func testMigrationRoundTrip(t *testing.T, db *sql.DB, dsn string) {
	migrateTransactions(t, dsn)
	initial := createVersionedContact(t, db)
	// Only this isolated test uses golang-migrate's destructive down API.
	// It owns a separate pool because runner.Close closes the driver's pool.
	pool, err := contactsql.Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	source, err := iofs.New(os.DirFS("../contacttx"), "migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { source.Close() })
	driver, err := migratepgx.WithInstance(pool, &migratepgx.Config{MigrationsTable: "sql_tx_schema_migrations"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { driver.Close() })
	runner, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		t.Fatal(err)
	}
	// Version 2 down discards its history/version column but retains contacts.
	if err := runner.Steps(-1); err != nil {
		t.Fatal(err)
	}
	assertMigrationState(t, db, 1, false)
	var name string
	if err := db.QueryRowContext(t.Context(), "SELECT name FROM sql_tx_contacts WHERE id = $1", initial.ID).Scan(&name); err != nil || name != initial.Name {
		t.Fatalf("contact after down = (%q, %v)", name, err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	assertMigrationState(t, db, 2, false)
	if err := runner.Down(); err != nil {
		t.Fatal(err)
	}
	var gone bool
	if err := db.QueryRowContext(t.Context(), "SELECT to_regclass('public.sql_tx_contacts') IS NULL").Scan(&gone); err != nil || !gone {
		t.Fatalf("contacts after full down = (%v, %v)", gone, err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	assertMigrationState(t, db, 2, false)
}
