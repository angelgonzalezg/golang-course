//go:build integration

// Shared PostgreSQL setup for the fundamentals and transaction suites. The tag
// selects real container-based tests; this file provides their owned resources.
package integration_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"example.com/course/sql-lab/contactsql"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Each top-level test owns its database. No fixture reads DATABASE_URL.
func newTestDatabase(t *testing.T) (*sql.DB, string) {
	t.Helper()
	// This budget bounds startup and the initial Ping. Canceling it when this
	// helper returns does not replace the container/pool cleanup registered below.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	container, err := postgres.Run(ctx, "postgres:18.6-alpine3.24",
		postgres.WithDatabase("sql_lab_test"),
		postgres.WithUsername("sql_lab"),
		postgres.WithPassword("sql_lab_test_password"),
		postgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("start PostgreSQL (check docker info): %v", err)
	}
	// Testcontainers discovers this container's mapped port. The resulting DSN
	// points to the isolated test database, not the learner's Compose database.
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db, err := contactsql.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Cleanups run in reverse registration order: close Go's pool before the
	// earlier container cleanup removes PostgreSQL.
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db, dsn
}
