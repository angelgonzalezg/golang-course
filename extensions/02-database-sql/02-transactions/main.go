// Transactions: run the finite business examples or apply schema changes only.
// DATABASE_URL points to the same dedicated lab database used by stage 1.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"example.com/course/sql-lab/contacttx"
	"example.com/course/sql-lab/internal/txdemo"
)

func main() {
	// Report errors only after run has returned and its owned cleanup has finished.
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run chooses the learning demo, forward migrations, or explicit legacy adoption.
// With no flags, the demo prepares its schema and shows commit/rollback behavior.
func run() error {
	migrateOnly := flag.Bool("migrate-only", false, "apply forward migrations and exit")
	adoptLegacy := flag.Bool("adopt-legacy", false, "adopt verified history from the old runner, migrate, and exit")
	flag.Parse()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("set DATABASE_URL to the PostgreSQL lab database; see extensions/02-database-sql/02-transactions/README.md")
	}
	// This context bounds the business demo. golang-migrate has no context
	// parameter; contacttx configures its connection/lock/statement limits.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if !*migrateOnly && !*adoptLegacy {
		return txdemo.Run(ctx, dsn, os.Stdout)
	}
	var version int
	var err error
	if *adoptLegacy {
		// Adoption verifies the old runner's recorded history before setting a
		// baseline. It preserves existing rows and does not clear dirty state.
		version, err = contacttx.AdoptLegacy(dsn)
	} else {
		// Repeating a completed migration is an expected no-change result.
		version, err = contacttx.Migrate(dsn)
	}
	if err == nil {
		fmt.Printf("Schema version: %d\n", version)
	}
	return err
}
