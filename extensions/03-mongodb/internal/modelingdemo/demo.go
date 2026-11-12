// Package modelingdemo runs the document-modeling lesson against real MongoDB.
package modelingdemo

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"example.com/course/mongo-lab/contactdoc"
	"example.com/course/mongo-lab/contactmongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Run owns one client and tracks its inserted IDs for cleanup. Only its own
// profiles are removed; other profiles and the collection/indexes remain.
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
	store := contactdoc.NewStore(client.Database("mongo_lab"))
	if err := store.EnsureIndexes(ctx); err != nil {
		return err
	}
	var owned []bson.ObjectID
	defer func() {
		// Fresh cleanup time remains available after an expired operation. Each
		// ID is registered as soon as Create returns; deletion precedes Disconnect.
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, id := range owned {
			if err := store.Delete(cleanup, id); err != nil && !errors.Is(err, contactdoc.ErrNotFound) {
				result = errors.Join(result, fmt.Errorf("clean up demo profile: %w", err))
			}
		}
	}()

	fmt.Fprintln(output, "== 1. Embed an address and a bounded array of tags ==")
	suffix := strings.ToLower(rand.Text()[:12])
	// A per-run tag makes query counts independent of other learners' profiles.
	goTag := "go-" + suffix
	inputs := []contactdoc.Input{
		{Name: "Ana " + suffix, Address: contactdoc.Address{Street: "Rio 1", City: "Guadalajara"}, Tags: []string{goTag, "backend"}},
		{Name: "Beto " + suffix, Address: contactdoc.Address{Street: "Sol 2", City: "Guadalajara"}, Tags: []string{goTag}},
		{Name: "Carla " + suffix, Address: contactdoc.Address{Street: "Luna 3", City: "Monterrey"}, Tags: []string{"docs-" + suffix}},
	}
	var first contactdoc.Profile
	for i, input := range inputs {
		created, err := store.Create(ctx, input)
		if err != nil {
			return err
		}
		owned = append(owned, created.ID)
		if i == 0 {
			first = created
		}
	}
	fmt.Fprintf(output, "Stored three profiles; the Go query tag for this run is %q.\n", goTag)

	fmt.Fprintln(output, "\n== 2. Combine a city and an array-element filter ==")
	query := contactdoc.Query{City: "Guadalajara", Tag: goTag, Limit: 1}
	page, err := store.Search(ctx, query)
	if err != nil {
		return err
	}
	if len(page) != 1 || page[0].ID != first.ID || page[0].Address != first.Address {
		return errors.New("city/tag query did not return the expected first summary")
	}
	fmt.Fprintln(output, "City + tag filter -> one projected summary (limit=1), ordered by name and ID.")
	fmt.Fprintf(output, "Summary: %s, %s, %s; tags were excluded by the projection.\n", page[0].Name, page[0].Address.Street, page[0].Address.City)

	fmt.Fprintln(output, "\n== 3. Query by tag and retrieve a full document ==")
	page, err = store.Search(ctx, contactdoc.Query{Tag: goTag, Limit: 10})
	if err != nil {
		return err
	}
	if len(page) != 2 {
		return errors.New("tag query did not match both Go profiles")
	}
	found, err := store.Get(ctx, first.ID)
	if err != nil {
		return err
	}
	if !slices.Equal(found.Tags, first.Tags) {
		return errors.New("full document did not retain its tags")
	}
	fmt.Fprintln(output, "Tag filter -> two summaries; Get still returns the full profile with tags.")

	fmt.Fprintln(output, "\n== 4. Change one child field with dot notation ==")
	updated, err := store.UpdateCity(ctx, first.ID, " Puebla ")
	if err != nil {
		return err
	}
	if updated.Address.City != "Puebla" || updated.Address.Street != first.Address.Street ||
		updated.Name != first.Name || !slices.Equal(updated.Tags, first.Tags) {
		return errors.New("nested city update lost an unrelated field")
	}
	fmt.Fprintln(output, "Updated address.city to Puebla; street, name, and tags stayed intact.")
	query.Limit = 10
	page, err = store.Search(ctx, query)
	if err != nil {
		return err
	}
	if len(page) != 1 || page[0].ID != owned[1] {
		return errors.New("city query did not reflect the nested update")
	}
	fmt.Fprintln(output, "The original Guadalajara + tag query now matches only Beto.")

	fmt.Fprintln(output, "\n== 5. Caller cancellation, missing IDs, and cleanup ==")
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Search(canceled, query); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("expected canceled search, got %v", err)
	}
	if _, err := store.UpdateCity(ctx, bson.NewObjectID(), "Puebla"); !errors.Is(err, contactdoc.ErrNotFound) {
		return fmt.Errorf("expected missing profile, got %v", err)
	}
	fmt.Fprintln(output, "Canceled search -> context.Canceled; missing update -> ErrNotFound.")
	for _, id := range owned {
		if err := store.Delete(ctx, id); err != nil {
			return err
		}
	}
	fmt.Fprintln(output, "Deleted only the three demo profiles; existing data is retained.")
	return nil
}
