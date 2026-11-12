// Package contactmongo teaches MongoDB CRUD with the official Go driver.
// This small document model is separate from the REST/gRPC transport models.
package contactmongo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// Open configures a reusable client and verifies connectivity/authentication.
// The caller owns Disconnect; stores borrow this client through database handles.
func Open(ctx context.Context, uri string) (*mongo.Client, error) {
	if strings.TrimSpace(uri) == "" {
		return nil, errors.New("MongoDB connection URI is required")
	}
	// Connect starts the driver's background work; Ping performs a real round trip.
	// One client owns pools for its servers and is safe to share across goroutines.
	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetMaxPoolSize(4))
	if err != nil {
		return nil, fmt.Errorf("configure MongoDB: %w", err)
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		// Cleanup must still run if the connection attempt exhausted ctx's budget.
		cleanup, cancel := cleanupContext()
		defer cancel()
		return nil, errors.Join(fmt.Errorf("connect to MongoDB: %w", queryError(ctx, err)), client.Disconnect(cleanup))
	}
	return client, nil
}

// Cleanup outlives an expired operation but remains bounded.
func cleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}
