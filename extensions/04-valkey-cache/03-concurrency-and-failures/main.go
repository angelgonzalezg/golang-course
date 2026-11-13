// Run the finite concurrency/failure lesson against PostgreSQL and Valkey.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/course/cache-lab/internal/resiliencedemo"
)

func main() {
	// Keep deferred cleanup inside run before os.Exit.
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	dsn, uri := os.Getenv("DATABASE_URL"), os.Getenv("VALKEY_URL")
	if dsn == "" || uri == "" {
		return errors.New("set DATABASE_URL and VALKEY_URL; see extensions/04-valkey-cache/03-concurrency-and-failures/README.md")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	return resiliencedemo.Run(ctx, dsn, uri, os.Stdout)
}
