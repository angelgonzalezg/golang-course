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
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	migrateOnly := flag.Bool("migrate-only", false, "apply forward migrations and exit")
	adoptLegacy := flag.Bool("adopt-legacy", false, "adopt verified history from the old runner, migrate, and exit")
	flag.Parse()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("set DATABASE_URL to the PostgreSQL lab database; see extensions/02-database-sql/02-transactions/README.md")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if !*migrateOnly && !*adoptLegacy {
		return txdemo.Run(ctx, dsn, os.Stdout)
	}
	var version int
	var err error
	if *adoptLegacy {
		version, err = contacttx.AdoptLegacy(dsn)
	} else {
		version, err = contacttx.Migrate(dsn)
	}
	if err == nil {
		fmt.Printf("Schema version: %d\n", version)
	}
	return err
}
