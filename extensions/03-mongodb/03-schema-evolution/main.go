// Run the finite validation/schema-evolution lesson. This README includes the
// explicit schema-account setup and both required connection variables.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"example.com/course/mongo-lab/internal/schemademo"
)

func main() {
	// run returns after deferred cleanup; os.Exit must remain outside it.
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Use separate connections for ordinary data access and collection policy.
func run() error {
	dataURI, schemaURI := os.Getenv("MONGODB_URI"), os.Getenv("MONGODB_SCHEMA_URI")
	if dataURI == "" || schemaURI == "" {
		return errors.New("set MONGODB_URI and MONGODB_SCHEMA_URI; see extensions/03-mongodb/03-schema-evolution/README.md")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return schemademo.Run(ctx, dataURI, schemaURI, os.Stdout)
}
