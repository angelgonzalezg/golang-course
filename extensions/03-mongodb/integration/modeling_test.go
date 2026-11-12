//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"example.com/course/mongo-lab/contactdoc"
	"example.com/course/mongo-lab/contactmongo"
	"example.com/course/mongo-lab/internal/modelingdemo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// The second lesson owns a real server, while each case owns a fresh namespace.
// The module-wide integration command exercises both suites.
func TestDocumentModeling(t *testing.T) {
	client, uri := newTestDatabase(t)
	t.Run("embedding and nested updates", func(t *testing.T) { testEmbeddedUpdates(t, client) })
	t.Run("filters projections and ordering", func(t *testing.T) { testProfileQueries(t, client) })
	t.Run("cancellation and missing IDs", func(t *testing.T) { testProfileContexts(t, client) })
	t.Run("finite demo cleanup", func(t *testing.T) { testModelingDemo(t, client, uri) })
}

// These namespaces exist only in the owned Testcontainers server. A cleanup
// uses fresh time and drops only its case's database, never a manual lab database.
func newProfileStore(t *testing.T, client *mongo.Client) (*contactdoc.Store, *mongo.Database) {
	t.Helper()
	database := client.Database("profile_test_" + rand.Text())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := database.Drop(ctx); err != nil {
			t.Error(err)
		}
	})
	store := contactdoc.NewStore(database)
	if err := store.EnsureIndexes(operationContext(t)); err != nil {
		t.Fatal(err)
	}
	return store, database
}

// Inspect real BSON and update only city. Unknown sibling fields from another
// writer must survive, and a concurrent street update must not be overwritten.
func testEmbeddedUpdates(t *testing.T, client *mongo.Client) {
	ctx := operationContext(t)
	store, database := newProfileStore(t, client)
	input := contactdoc.Input{Name: " Ana ", Address: contactdoc.Address{Street: " Rio 1 ", City: " Guadalajara "}, Tags: []string{" Go ", "go", " Backend "}}
	created, err := store.Create(ctx, input)
	if err != nil || created.ID.IsZero() || created.Name != "Ana" || !slices.Equal(created.Tags, []string{"go", "backend"}) {
		t.Fatalf("create = (%+v, %v)", created, err)
	}
	input.Tags[0] = "caller edit"
	if created.Tags[0] != "go" {
		t.Fatal("create result shares caller's tag slice")
	}
	if err := store.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	collection := database.Collection("contact_profiles")
	filter := bson.D{{Key: "_id", Value: created.ID}}
	raw, err := collection.FindOne(ctx, filter).Raw()
	if err != nil {
		t.Fatal(err)
	}
	if raw.Lookup("address").Type != bson.TypeEmbeddedDocument || raw.Lookup("tags").Type != bson.TypeArray {
		t.Fatalf("stored document shape = %v", raw)
	}
	// A field not known to the Go struct illustrates why reading/modifying/saving
	// an entire embedded object would be a different, destructive editing contract.
	_, err = collection.UpdateOne(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "address.postal_code", Value: "44100"}}}})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		updated, err := store.UpdateCity(ctx, created.ID, " Puebla ")
		if err != nil || updated.Address != (contactdoc.Address{Street: "Rio 1", City: "Puebla"}) ||
			updated.Name != "Ana" || !slices.Equal(updated.Tags, created.Tags) {
			t.Fatalf("update = (%+v, %v)", updated, err)
		}
	}
	// Both writers address different child fields. Their final changes must
	// coexist regardless of which one MongoDB executes first.
	results := make(chan error, 2)
	go func() {
		_, err := store.UpdateCity(ctx, created.ID, "Monterrey")
		results <- err
	}()
	go func() {
		_, err := collection.UpdateOne(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "address.street", Value: "Sol 2"}}}})
		results <- err
	}()
	for range 2 {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	found, err := store.Get(ctx, created.ID)
	if err != nil || found.Address != (contactdoc.Address{Street: "Sol 2", City: "Monterrey"}) {
		t.Fatalf("concurrent child writes = (%+v, %v)", found, err)
	}
	raw, err = collection.FindOne(ctx, filter).Raw()
	if err != nil {
		t.Fatal(err)
	}
	if raw.Lookup("address", "postal_code").StringValue() != "44100" {
		t.Fatal("nested update lost an unknown sibling field")
	}
	found.Tags[0] = "local edit"
	again, err := store.Get(ctx, created.ID)
	if err != nil || !slices.Equal(again.Tags, []string{"go", "backend"}) {
		t.Fatalf("editing a result changed storage = (%+v, %v)", again, err)
	}
}

// Use known documents to verify AND criteria, array membership, normalized tag
// values, exact cities, projection fields, a sort tie, limits, and literal input.
func testProfileQueries(t *testing.T, client *mongo.Client) {
	ctx := operationContext(t)
	store, _ := newProfileStore(t, client)
	empty, err := store.Search(ctx, contactdoc.Query{Limit: 2})
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty query = (%v, %v)", empty, err)
	}
	inputs := []contactdoc.Input{
		{Name: "Beto", Address: contactdoc.Address{Street: "Sol 3", City: "Monterrey"}, Tags: []string{"go"}},
		{Name: "Ana", Address: contactdoc.Address{Street: "Rio 1", City: "Guadalajara"}, Tags: []string{"Go", "backend"}},
		{Name: "Ana", Address: contactdoc.Address{Street: "Rio 2", City: "Guadalajara"}, Tags: []string{"go"}},
		{Name: "Carla", Address: contactdoc.Address{Street: "Luna 4", City: "Guadalajara"}, Tags: []string{"frontend"}},
		{Name: "Literal", Address: contactdoc.Address{Street: "Literal 5", City: `{"$ne":null}`}, Tags: []string{`{"$ne":null}`}},
	}
	var profiles []contactdoc.Profile
	for _, input := range inputs {
		created, err := store.Create(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		profiles = append(profiles, created)
	}
	query := contactdoc.Query{City: " Guadalajara ", Tag: " GO ", Limit: 10}
	page, err := store.Search(ctx, query)
	if err != nil || len(page) != 2 || page[0].Name != "Ana" || page[1].Name != "Ana" ||
		bytes.Compare(page[0].ID[:], page[1].ID[:]) >= 0 {
		t.Fatalf("city AND tag sorted query = (%+v, %v)", page, err)
	}
	wantIDs := []bson.ObjectID{profiles[1].ID, profiles[2].ID}
	if !slices.Contains(wantIDs, page[0].ID) || !slices.Contains(wantIDs, page[1].ID) || page[0].ID == page[1].ID {
		t.Fatalf("AND query returned unrelated profiles: %+v", page)
	}
	query.Limit = 1
	limited, err := store.Search(ctx, query)
	if err != nil || len(limited) != 1 || limited[0] != page[0] {
		t.Fatalf("limited summary = (%+v, %v)", limited, err)
	}
	// The projected DTO has only fields meant for display; Get retrieves the
	// full stored tags. A summary must not be treated as a replacement Profile.
	full, err := store.Get(ctx, limited[0].ID)
	if err != nil || full.Address != limited[0].Address || full.Name != limited[0].Name || len(full.Tags) == 0 {
		t.Fatalf("full versus summary = (%+v, %v)", full, err)
	}
	for _, test := range []struct {
		name  string
		query contactdoc.Query
		count int
	}{
		{"city only", contactdoc.Query{City: "Guadalajara", Limit: 100}, 3},
		{"tag only", contactdoc.Query{Tag: " GO ", Limit: 100}, 3},
		{"city case differs", contactdoc.Query{City: "guadalajara", Limit: 100}, 0},
		{"no matching tag", contactdoc.Query{Tag: "rust", Limit: 100}, 0},
		{"no criteria", contactdoc.Query{Limit: 100}, 5},
		{"literal values", contactdoc.Query{City: `{"$ne":null}`, Tag: `{"$ne":null}`, Limit: 100}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			results, err := store.Search(ctx, test.query)
			if err != nil || len(results) != test.count {
				t.Fatalf("search = (%+v, %v), want %d", results, err, test.count)
			}
		})
	}
}

// Deterministic canceled/expired callers keep their causes. Missing profiles
// report domain errors, and a rejected write leaves the existing city intact.
func testProfileContexts(t *testing.T, client *mongo.Client) {
	ctx := operationContext(t)
	store, _ := newProfileStore(t, client)
	created, err := store.Create(ctx, contactdoc.Input{Name: "Ana", Address: contactdoc.Address{Street: "Rio 1", City: "Guadalajara"}})
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Search(canceled, contactdoc.Query{Limit: 2}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled search = %v", err)
	}
	expired, finish := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer finish()
	if _, err := store.UpdateCity(expired, created.ID, "Puebla"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired update = %v", err)
	}
	full, err := store.Get(ctx, created.ID)
	if err != nil || full.Address.City != "Guadalajara" || full.Tags == nil {
		t.Fatalf("state after rejected update = (%+v, %v)", full, err)
	}
	missing := bson.NewObjectID()
	if _, err := store.Get(ctx, missing); !errors.Is(err, contactdoc.ErrNotFound) || !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("missing get = %v", err)
	}
	if _, err := store.UpdateCity(ctx, missing, "Puebla"); !errors.Is(err, contactdoc.ErrNotFound) {
		t.Fatalf("missing update = %v", err)
	}
	if err := store.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, created.ID); !errors.Is(err, contactdoc.ErrNotFound) {
		t.Fatalf("repeated delete = %v", err)
	}
}

// Repeated real demo runs remove only their three owned profiles. Preserve both
// another profile and the fundamentals collection within this disposable server.
func testModelingDemo(t *testing.T, client *mongo.Client, uri string) {
	ctx := operationContext(t)
	database := client.Database("mongo_lab")
	store := contactdoc.NewStore(database)
	kept, err := store.Create(ctx, contactdoc.Input{Name: "Keep profile", Address: contactdoc.Address{Street: "Rio 1", City: "Guadalajara"}})
	if err != nil {
		t.Fatal(err)
	}
	fundamentals := contactmongo.NewStore(database)
	contact, err := fundamentals.Create(ctx, contactmongo.Input{Name: "Keep contact"})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		var output bytes.Buffer
		if err := modelingdemo.Run(ctx, uri, &output); err != nil {
			t.Fatal(err)
		}
		for _, summary := range []string{
			"Stored three profiles", "one projected summary", "Tag filter -> two summaries",
			"Updated address.city to Puebla", "missing update -> ErrNotFound", "Deleted only the three demo profiles",
		} {
			if !strings.Contains(output.String(), summary) {
				t.Errorf("demo missing %q", summary)
			}
		}
		page, err := store.Search(ctx, contactdoc.Query{Limit: 100})
		if err != nil || len(page) != 1 || page[0].ID != kept.ID {
			t.Fatalf("profile cleanup = (%+v, %v)", page, err)
		}
		if _, err := fundamentals.Get(ctx, contact.ID); err != nil {
			t.Fatalf("fundamentals contact was lost: %v", err)
		}
	}
}
