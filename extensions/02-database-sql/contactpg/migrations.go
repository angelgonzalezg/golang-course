package contactpg

import (
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// LatestVersion belongs to the service schema, independent of earlier lessons.
const LatestVersion = 1

//go:embed migrations/*.sql
var migrationSQL embed.FS

// Migrate uses golang-migrate's embedded source and its own owned pool/version
// table. Normal startup only moves forward and never forces dirty state clean.
// The library has no context API; connection, SQL, and lock waits have explicit
// per-operation limits, as taught in the transaction/migration lesson.
func Migrate(dsn string) (version int, result error) {
	if dsn == "" {
		return 0, errors.New("database URL is required")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return 0, errors.New("invalid migration database configuration")
	}
	config.ConnectTimeout = 5 * time.Second
	config.RuntimeParams["search_path"] = "public"
	config.RuntimeParams["statement_timeout"] = "10000"
	config.RuntimeParams["lock_timeout"] = "5000"
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	defer db.Close()
	source, err := iofs.New(migrationSQL, "migrations")
	if err != nil {
		return 0, err
	}
	driver, err := migratepgx.WithInstance(db, &migratepgx.Config{
		MigrationsTable: "sql_service_schema_migrations", SchemaName: "public", StatementTimeout: 10 * time.Second,
	})
	if err != nil {
		return 0, errors.Join(err, source.Close())
	}
	runner, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		return 0, errors.Join(err, source.Close(), driver.Close())
	}
	defer func() { sourceErr, databaseErr := runner.Close(); result = errors.Join(result, sourceErr, databaseErr) }()
	runner.LockTimeout = 15 * time.Second
	if err := runner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, fmt.Errorf("migrate service schema: %w", err)
	}
	current, dirty, err := runner.Version()
	if err != nil {
		return 0, err
	}
	if current != LatestVersion || dirty {
		return 0, fmt.Errorf("unexpected service schema: version=%d dirty=%t", current, dirty)
	}
	return int(current), nil
}
