// Run the finite Valkey fundamentals lesson. See this directory's README.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"example.com/course/cache-lab/internal/demo"
)

func main() {
	// Let deferred client/key cleanup finish inside run before os.Exit.
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run reads explicit configuration and owns the demonstration's operation budget.
func run() error {
	uri := os.Getenv("VALKEY_URL")
	if uri == "" {
		return errors.New("set VALKEY_URL; see extensions/04-valkey-cache/01-fundamentals/README.md")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return demo.Run(ctx, uri, os.Stdout)
}
