// Run the finite PostgreSQL + Valkey cache-aside lesson. See this directory's README.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/course/cache-lab/internal/asidedemo"
)

func main() {
	// Cleanup belongs inside run; os.Exit itself does not execute deferred calls.
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	dsn, uri := os.Getenv("DATABASE_URL"), os.Getenv("VALKEY_URL")
	if dsn == "" || uri == "" {
		return errors.New("set DATABASE_URL and VALKEY_URL; see extensions/04-valkey-cache/02-cache-aside/README.md")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return asidedemo.Run(ctx, dsn, uri, os.Stdout)
}
