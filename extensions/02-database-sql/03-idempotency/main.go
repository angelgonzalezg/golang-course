// Durable idempotency: run a finite create/replay walkthrough or prepare its
// independent schema. See README.md for the learning passes and real-database tests.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"example.com/course/sql-lab/contactidem"
	"example.com/course/sql-lab/internal/idempotencydemo"
)

func main() {
	// Run finishes deferred pool/row cleanup before process exit.
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run owns the command's configuration and business-operation budget. Migration
// SQL has its own per-operation limits because the library takes no context.
func run() error {
	migrateOnly := flag.Bool("migrate-only", false, "apply this lesson's forward migrations and exit")
	flag.Parse()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("set DATABASE_URL to the PostgreSQL lab database; see extensions/02-database-sql/03-idempotency/README.md")
	}
	if *migrateOnly {
		version, err := contactidem.Migrate(dsn)
		if err == nil {
			fmt.Printf("Idempotency schema version: %d\n", version)
		}
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return idempotencydemo.Run(ctx, dsn, os.Stdout)
}
