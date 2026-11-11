// Package contacttx teaches explicit SQL transactions and versioned migrations.
// Its sql_tx_* tables are independent from the introductory sql_lab_contacts.
package contacttx

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"example.com/course/sql-lab/contactsql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// LatestVersion is the schema required by the transaction store.
const LatestVersion = 2

// ErrMigrationState reports a backward target, an unknown version, or a legacy
// schema that needs explicit adoption. Dirty state is migrate.ErrDirty instead.
var ErrMigrationState = errors.New("migration state requires review")

//go:embed migrations/*.sql
var migrationSQL embed.FS

// Migrate applies pending up migrations using golang-migrate. It owns a separate
// pool: the migration driver's Close also closes its pool, so it must never
// borrow the business store's *sql.DB.
func Migrate(dsn string) (int, error) {
	return MigrateTo(dsn, LatestVersion)
}

// MigrateTo applies only forward migrations up to target. The library does not
// accept a context, so connection, lock, and statement timeouts bound its SQL.
// Business operations continue to use context-aware database/sql methods.
func MigrateTo(dsn string, target int) (int, error) {
	return runMigrations(dsn, target, false)
}

// AdoptLegacy verifies the previous teaching runner's known history, records its
// baseline under the migration lock, then applies any remaining up migrations.
// Stop old migrators and review schema drift before explicitly choosing this.
func AdoptLegacy(dsn string) (int, error) {
	return runMigrations(dsn, LatestVersion, true)
}

// runMigrations connects three library pieces: an embedded-file source, a pgx
// database driver, and the migration runner. It owns and closes those resources.
// Read the up SQL files first; the adapter/legacy checks are a later learning pass.
func runMigrations(dsn string, target int, adoptLegacy bool) (version int, result error) {
	if target < 1 || target > LatestVersion {
		return 0, fmt.Errorf("migration target must be 1–%d: %w", LatestVersion, contactsql.ErrInvalidInput)
	}
	if dsn == "" {
		return 0, fmt.Errorf("empty database URL: %w", contactsql.ErrInvalidInput)
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		// Do not echo a connection URL, which can contain credentials.
		return 0, errors.New("invalid migration database configuration")
	}
	// The library does not receive the business context. Bound database work
	// here instead of starting a goroutine that could outlive a canceled caller.
	config.ConnectTimeout = 5 * time.Second
	config.RuntimeParams["search_path"] = "public"
	config.RuntimeParams["statement_timeout"] = "10000"
	config.RuntimeParams["lock_timeout"] = "5000"
	db := stdlib.OpenDB(*config)
	// The driver reserves one connection; legacy inspection uses another.
	db.SetMaxOpenConns(2)
	// Closing the reserved connection must end its PostgreSQL session, including
	// any remaining advisory lock after an error.
	db.SetMaxIdleConns(0)
	defer db.Close()

	// iofs reads the embedded up/down files. WithInstance tells the pgx driver
	// which pool and version table to use; neither step applies the up files yet.
	source, err := iofs.New(migrationSQL, "migrations")
	if err != nil {
		return 0, err
	}
	driver, err := migratepgx.WithInstance(db, &migratepgx.Config{
		MigrationsTable:  "sql_tx_schema_migrations",
		SchemaName:       "public",
		StatementTimeout: 10 * time.Second,
	})
	if err != nil {
		return 0, errors.Join(err, source.Close())
	}
	// Delegate the driver's methods, overriding only the version check. The
	// library still owns the migration lock, SQL execution, and dirty flag.
	guard := &forwardOnlyDriver{Driver: driver, db: db, target: target, adoptLegacy: adoptLegacy}
	runner, err := migrate.NewWithInstance("iofs", source, "pgx5", guard)
	if err != nil {
		return 0, errors.Join(err, source.Close(), driver.Close())
	}
	defer func() {
		sourceErr, databaseErr := runner.Close()
		result = errors.Join(result, sourceErr, databaseErr)
	}()
	// PostgreSQL's lock_timeout also bounds the driver's advisory-lock query.
	// Keep it shorter than the library's outer wait for that query.
	runner.LockTimeout = 15 * time.Second
	// Already being at target is success for this command. A dirty migration or
	// SQL failure stays an error and is never silently forced to a clean version.
	if err := runner.Migrate(uint(target)); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, fmt.Errorf("migrate schema: %w", err)
	}
	return target, nil
}

// golang-migrate calls Version after acquiring its lock. Checking here prevents
// a concurrent upgrade from turning a requested older target into a downgrade.
// Execution, version recording, dirty state, and locking remain library-owned.
type forwardOnlyDriver struct {
	database.Driver
	db          *sql.DB
	target      int
	adoptLegacy bool
}

func (d *forwardOnlyDriver) Version() (int, bool, error) {
	version, dirty, err := d.Driver.Version()
	if err != nil || dirty {
		// Leave dirty state untouched so golang-migrate stops for operator review.
		return version, dirty, err
	}
	if version > d.target {
		return version, false, fmt.Errorf("schema version %d exceeds target %d: %w", version, d.target, ErrMigrationState)
	}
	if version == database.NilVersion {
		// No new-style version exists yet: distinguish a fresh database from one
		// prepared by the previous runner before applying any CREATE TABLE file.
		return d.legacyVersion()
	}
	return version, false, nil
}
