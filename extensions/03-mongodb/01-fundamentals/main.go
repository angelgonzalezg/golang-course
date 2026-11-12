// Run the finite MongoDB walkthrough. Its README includes Compose and test setup.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"example.com/course/mongo-lab/internal/demo"
)

func main() {
	// Let run's deferred cleanup finish before os.Exit terminates the process.
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run reads explicit configuration and gives all demo operations one budget.
func run() error {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		return errors.New("set MONGODB_URI; see extensions/03-mongodb/01-fundamentals/README.md")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return demo.Run(ctx, uri, os.Stdout)
}
