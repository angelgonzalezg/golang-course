// Run the finite embedded-document/query lesson. See this directory's README.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"example.com/course/mongo-lab/internal/modelingdemo"
)

func main() {
	// Deferred cleanup finishes inside run before main exits on an error.
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run owns configuration and the complete walkthrough's operation budget.
func run() error {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		return errors.New("set MONGODB_URI; see extensions/03-mongodb/02-document-modeling/README.md")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return modelingdemo.Run(ctx, uri, os.Stdout)
}
