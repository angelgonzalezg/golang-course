// Run a finite PostgreSQL-backed REST walkthrough, or keep serving for curl.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/course/sql-lab/contactpg"
	"example.com/course/sql-lab/internal/restapp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run owns flags and cancellation; deferred cleanup completes before main exits.
func run() error {
	serve := flag.Bool("serve", false, "keep serving instead of running the finite demo")
	addr := flag.String("addr", "127.0.0.1:8081", "listen address with -serve")
	subject := flag.String("subject", "local-user", "single-user lab key namespace with -serve")
	migrateOnly := flag.Bool("migrate-only", false, "apply service schema migrations and exit")
	flag.Parse()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("set DATABASE_URL; see extensions/02-database-sql/05-rest-postgres/README.md")
	}
	if *migrateOnly {
		version, err := contactpg.Migrate(dsn)
		if err == nil {
			fmt.Printf("Service schema version: %d\n", version)
		}
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *serve {
		return restapp.Serve(ctx, dsn, *addr, *subject, os.Stdout)
	}
	ctx, timeout := context.WithTimeout(ctx, 20*time.Second)
	defer timeout()
	return restapp.RunDemo(ctx, dsn, os.Stdout)
}
