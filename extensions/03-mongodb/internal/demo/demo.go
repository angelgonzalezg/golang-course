// Package demo runs a finite walkthrough against a real MongoDB database.
package demo

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"example.com/course/mongo-lab/contactmongo"
)

// Run owns its clients and attempts to delete only its own randomly named
// document. It preserves other documents, the collection, and its index.
func Run(ctx context.Context, uri string, output io.Writer) (result error) {
	client, err := contactmongo.Open(ctx, uri)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		result = errors.Join(result, client.Disconnect(cleanup))
	}()
	// Handles select a namespace; the index/write commands create server resources.
	store := contactmongo.NewStore(client.Database("mongo_lab"))
	if err := store.EnsureIndexes(ctx); err != nil {
		return err
	}
	fmt.Fprintln(output, "== 1. Client, collection, index, and a BSON insert ==")
	input := contactmongo.Input{Name: " Ana " + rand.Text()[:8] + " "}
	created, err := store.Create(ctx, input)
	if err != nil {
		return err
	}
	// Register data cleanup immediately, before another operation can fail. Its
	// fresh context still works after timeout and runs before client disconnect.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := store.Delete(cleanup, created.ID); err != nil && !errors.Is(err, contactmongo.ErrNotFound) {
			result = errors.Join(result, fmt.Errorf("clean up demo contact: %w", err))
		}
	}()
	fmt.Fprintf(output, "Created ObjectID %s; omitted email is a missing field.\n", created.ID.Hex())

	fmt.Fprintln(output, "\n== 2. Decode one document, then iterate a bounded cursor ==")
	found, err := store.Get(ctx, created.ID)
	if err != nil {
		return err
	}
	page, err := store.List(ctx, 2)
	if err != nil {
		return err
	}
	// This page may include other lab users' data; only our assigned ID is owned.
	fmt.Fprintf(output, "Read %s; ordered page contains %d contact(s).\n", found.ID.Hex(), len(page))

	fmt.Fprintln(output, "\n== 3. Set empty text, then remove the field ==")
	empty := ""
	updated, err := store.UpdateEmail(ctx, created.ID, &empty)
	if err != nil {
		return err
	}
	if updated.Email == nil || *updated.Email != "" {
		return errors.New("empty email was not preserved")
	}
	updated, err = store.UpdateEmail(ctx, created.ID, nil)
	if err != nil {
		return err
	}
	if updated.Email != nil {
		return errors.New("removed email was not decoded as nil")
	}
	fmt.Fprintln(output, "Stored empty text with $set, then removed email with $unset.")

	fmt.Fprintln(output, "\n== 4. Unique indexes and caller cancellation ==")
	if _, err := store.Create(ctx, input); !errors.Is(err, contactmongo.ErrDuplicate) {
		return fmt.Errorf("expected uniqueness conflict, got %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Get(canceled, created.ID); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("expected cancellation, got %v", err)
	}
	fmt.Fprintln(output, "Duplicate name -> ErrDuplicate; canceled read -> context.Canceled.")

	fmt.Fprintln(output, "\n== 5. Another client reads the same persisted document ==")
	second, err := contactmongo.Open(ctx, uri)
	if err != nil {
		return err
	}
	// Each Open has an owner; do not create a new client for each normal query.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		result = errors.Join(result, second.Disconnect(cleanup))
	}()
	if _, err := contactmongo.NewStore(second.Database("mongo_lab")).Get(ctx, created.ID); err != nil {
		return err
	}
	fmt.Fprintln(output, "Data belongs to MongoDB, rather than to one Go client.")
	if err := store.Delete(ctx, created.ID); err != nil {
		return err
	}
	if _, err := store.Get(ctx, created.ID); !errors.Is(err, contactmongo.ErrNotFound) {
		return fmt.Errorf("expected a missing document after deletion, got %v", err)
	}
	fmt.Fprintln(output, "Deleted the demo contact; repeated lookup -> ErrNotFound.")
	return nil
}
