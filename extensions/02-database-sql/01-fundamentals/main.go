// The finite demo connects to a dedicated PostgreSQL lab database. See the lab
// README for the local Compose setup and the integration-test command.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"example.com/course/sql-lab/internal/demo"
)

func main() {
	// run and demo.Run finish their deferred cleanup before main exits. Calling
	// os.Exit inside either helper would skip that cleanup.
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run reads configuration and owns the demo's time budget. Database operations
// and pool ownership are kept in internal/demo so the same run can be tested.
func run() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("set DATABASE_URL to your PostgreSQL lab database; see extensions/02-database-sql/README.md")
	}
	// One caller-owned budget includes connecting and all demo queries.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return demo.Run(ctx, dsn, os.Stdout)
}
