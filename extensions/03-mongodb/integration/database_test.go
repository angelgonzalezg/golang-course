//go:build integration

// Real MongoDB fixtures are opt-in. No test reads the learner's MONGODB_URI.
package integration_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"example.com/course/mongo-lab/contactmongo"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Own one disposable, authenticated server. Register its cleanup even if startup
// only partially succeeds, then disconnect the Go client before container removal.
func newTestDatabase(t *testing.T) (*mongo.Client, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	container, err := mongodb.Run(ctx, "mongo:8.0.38-noble",
		mongodb.WithUsername("mongo_test"), mongodb.WithPassword("mongo_test_local"))
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("start MongoDB (check docker info): %v", err)
	}
	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client, err := contactmongo.Open(ctx, uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { disconnect(t, client) })
	return client, uri
}

// Each case gets its own namespace. Dropping it cannot affect Compose data or
// another case; the server itself belongs exclusively to this test suite.
func newStore(t *testing.T, client *mongo.Client) (*contactmongo.Store, *mongo.Database) {
	t.Helper()
	database := client.Database("mongo_test_" + rand.Text())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := database.Drop(ctx); err != nil {
			t.Error(err)
		}
	})
	store := contactmongo.NewStore(database)
	if err := store.EnsureIndexes(operationContext(t)); err != nil {
		t.Fatal(err)
	}
	return store, database
}

// Bound each case independently of the package's overall go test timeout.
func operationContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// Use fresh cleanup time after an operation deadline or test cancellation.
func disconnect(t *testing.T, client *mongo.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Disconnect(ctx); err != nil {
		t.Error(err)
	}
}
