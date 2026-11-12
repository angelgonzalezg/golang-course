// Run the finite references/aggregation lab. Its README has standalone setup.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"example.com/course/mongo-lab/internal/reportdemo"
)

func main() {
	// Cleanup finishes in run before main exits with a failure status.
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		return errors.New("set MONGODB_URI; see extensions/03-mongodb/04-references-and-aggregation/README.md")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return reportdemo.Run(ctx, uri, os.Stdout)
}
