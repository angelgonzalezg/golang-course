// Package contactsql teaches PostgreSQL access through database/sql.
// Its small teaching model is separate from the REST/gRPC contact store.
package contactsql

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	// Register pgx's database/sql driver. The queries below still use sql.DB.
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed schema.sql
var schema string

// Open creates a bounded connection pool and verifies that PostgreSQL is reachable.
// The caller owns the returned pool and must close it after its users finish.
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("database connection string is required")
	}
	// sql.Open creates a handle; it usually does not establish a connection yet.
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("configure PostgreSQL: %w", err)
	}
	// Four connections bound this small lab's concurrency. Tune for a measured
	// workload when deploying a service; this is not a universal pool size.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	return db, nil
}

// EnsureSchema creates the lesson's table if absent. It preserves existing rows.
// Schema changes and versioned migrations belong to the next stage.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create teaching table: %w", err)
	}
	return nil
}
