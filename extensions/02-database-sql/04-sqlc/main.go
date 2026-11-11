// Compare manual database/sql operations with sqlc-generated bindings over the
// same PostgreSQL schema. The guide explains generation and shared contracts.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"example.com/course/sql-lab/internal/sqlcdemo"
)

func main() {
	// run's defers close the pool and clean up before main can exit.
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run supplies configuration and one finite budget for the demonstration.
func run() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("set DATABASE_URL to the PostgreSQL lab database; see extensions/02-database-sql/04-sqlc/README.md")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return sqlcdemo.Run(ctx, dsn, os.Stdout)
}
