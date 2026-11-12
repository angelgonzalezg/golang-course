//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/course/mongo-lab/contactmongo"
	"example.com/course/mongo-lab/internal/demo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Exercise storage and resource ownership against an actual MongoDB process.
// Cases are serial; each owns a namespace inside this disposable server.
func TestMongoDB(t *testing.T) {
	client, uri := newTestDatabase(t)
	t.Run("CRUD and BSON field presence", func(t *testing.T) { testCRUD(t, client) })
	t.Run("concurrent unique names", func(t *testing.T) { testConcurrentNames(t, client) })
	t.Run("cancellation and deadlines", func(t *testing.T) { testContexts(t, client) })
	t.Run("independent clients", func(t *testing.T) { testIndependentClients(t, client, uri) })
	t.Run("authentication failure", func(t *testing.T) { testAuthentication(t, uri) })
	t.Run("finite demo cleanup", func(t *testing.T) { testDemo(t, client, uri) })
}

// Check returned values and stored BSON, including repeated assignments and
// removal. Literal text containing query-like syntax must remain ordinary data.
func testCRUD(t *testing.T, client *mongo.Client) {
	ctx := operationContext(t)
	store, database := newStore(t, client)
	page, err := store.List(ctx, 2)
	if err != nil || page == nil || len(page) != 0 {
		t.Fatalf("empty list = (%v, %v)", page, err)
	}
	email := "ana@example.com"
	created, err := store.Create(ctx, contactmongo.Input{Name: "  Ana  ", Email: &email})
	if err != nil || created.ID.IsZero() || created.Name != "Ana" || created.Email == nil || *created.Email != email {
		t.Fatalf("create = (%+v, %v)", created, err)
	}
	email = "caller changed input"
	if *created.Email != "ana@example.com" {
		t.Fatal("create result aliases caller's input")
	}
	// Recreating the same named index must preserve both the document and index.
	if err := store.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, contactmongo.Input{Name: " Ana "}); !errors.Is(err, contactmongo.ErrDuplicate) {
		t.Fatalf("duplicate = %v", err)
	}
	literal := `{"$ne":null}`
	other, err := store.Create(ctx, contactmongo.Input{Name: literal})
	if err != nil || other.Name != literal || other.Email != nil {
		t.Fatalf("literal create = (%+v, %v)", other, err)
	}
	page, err = store.List(ctx, 2)
	if err != nil || len(page) != 2 || bytes.Compare(page[0].ID[:], page[1].ID[:]) >= 0 {
		t.Fatalf("ordered list = (%+v, %v)", page, err)
	}
	bounded, err := store.List(ctx, 1)
	if err != nil || len(bounded) != 1 || bounded[0].ID != page[0].ID {
		t.Fatalf("bounded list = (%+v, %v)", bounded, err)
	}
	for _, value := range []*string{stringPointer("new@example.com"), stringPointer(""), nil, nil} {
		updated, err := store.UpdateEmail(ctx, created.ID, value)
		if err != nil || updated.ID != created.ID || updated.Name != "Ana" ||
			(updated.Email == nil) != (value == nil) || (value != nil && *updated.Email != *value) {
			t.Fatalf("update = (%+v, %v)", updated, err)
		}
		// Raw BSON proves that $unset removed the field, instead of writing null.
		raw, err := database.Collection("contacts").FindOne(ctx, bson.D{{Key: "_id", Value: created.ID}}).Raw()
		if err != nil {
			t.Fatal(err)
		}
		field, lookupErr := raw.LookupErr("email")
		if value == nil {
			if lookupErr == nil {
				t.Fatal("email field still present after $unset")
			}
		} else if lookupErr != nil || field.Type != bson.TypeString || field.StringValue() != *value {
			t.Fatalf("email BSON = %v, %v", field, lookupErr)
		}
	}
	// Model an external writer supplying explicit null. The typed model accepts
	// it but cannot distinguish it from missing; raw BSON can make that distinction.
	_, err = database.Collection("contacts").UpdateOne(ctx, bson.D{{Key: "_id", Value: created.ID}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "email", Value: nil}}}})
	if err != nil {
		t.Fatal(err)
	}
	found, err := store.Get(ctx, created.ID)
	if err != nil || found.Email != nil {
		t.Fatalf("decode null = (%+v, %v)", found, err)
	}
	if err := store.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, created.ID); !errors.Is(err, contactmongo.ErrNotFound) || !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("get deleted = %v", err)
	}
	if _, err := store.UpdateEmail(ctx, created.ID, nil); !errors.Is(err, contactmongo.ErrNotFound) {
		t.Fatalf("update deleted = %v", err)
	}
	if err := store.Delete(ctx, created.ID); !errors.Is(err, contactmongo.ErrNotFound) {
		t.Fatalf("delete again = %v", err)
	}
	if _, err := store.Get(ctx, other.ID); err != nil {
		t.Fatalf("unrelated contact was lost: %v", err)
	}
}

// Start competing creates together. Exactly one succeeds because MongoDB owns
// uniqueness; a pre-insert Go lookup would have a race between check and insert.
func testConcurrentNames(t *testing.T, client *mongo.Client) {
	ctx := operationContext(t)
	store, database := newStore(t, client)
	const callers = 12
	start := make(chan struct{})
	results := make(chan error, callers)
	for range callers {
		go func() {
			<-start
			_, err := store.Create(ctx, contactmongo.Input{Name: "One winner"})
			results <- err
		}()
	}
	close(start)
	winners := 0
	for range callers {
		err := <-results
		if err == nil {
			winners++
		} else if !errors.Is(err, contactmongo.ErrDuplicate) || !mongo.IsDuplicateKeyError(err) {
			t.Errorf("competing create: %v", err)
		}
	}
	count, err := database.Collection("contacts").CountDocuments(ctx, bson.D{})
	if err != nil || winners != 1 || count != 1 {
		t.Fatalf("winners=%d documents=%d error=%v", winners, count, err)
	}
}

// Already-canceled/expired callers must keep their context causes. No sleeps or
// timing guesses are needed, and rejected writes must leave the document intact.
func testContexts(t *testing.T, client *mongo.Client) {
	store, _ := newStore(t, client)
	ctx := operationContext(t)
	created, err := store.Create(ctx, contactmongo.Input{Name: "Still here"})
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Get(canceled, created.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled get = %v", err)
	}
	if _, err := store.Create(canceled, contactmongo.Input{Name: "Canceled"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled create = %v", err)
	}
	if _, err := store.List(canceled, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled list = %v", err)
	}
	expired, finish := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer finish()
	if err := store.Delete(expired, created.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired delete = %v", err)
	}
	page, err := store.List(ctx, 2)
	if err != nil || len(page) != 1 || page[0].ID != created.ID {
		t.Fatalf("state after rejected operations = (%v, %v)", page, err)
	}
}

// Another client reads server-owned data. Changing its decoded optional field
// changes only that Go result, not the original result or stored document.
func testIndependentClients(t *testing.T, client *mongo.Client, uri string) {
	ctx := operationContext(t)
	store, database := newStore(t, client)
	created, err := store.Create(ctx, contactmongo.Input{Name: "Persistent", Email: stringPointer("saved")})
	if err != nil {
		t.Fatal(err)
	}
	second, err := contactmongo.Open(ctx, uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { disconnect(t, second) })
	other := contactmongo.NewStore(second.Database(database.Name()))
	found, err := other.Get(ctx, created.ID)
	if err != nil || found.Email == nil || *found.Email != "saved" {
		t.Fatalf("second client read = (%+v, %v)", found, err)
	}
	*found.Email = "local edit"
	again, err := store.Get(ctx, created.ID)
	if err != nil || again.Email == nil || *again.Email != "saved" || *created.Email != "saved" {
		t.Fatalf("decoded result leaked edits = (%+v, %v)", again, err)
	}
}

// Startup must verify credentials, rather than treating client construction as
// a successful connection. The URI comes only from the owned test container.
func testAuthentication(t *testing.T, uri string) {
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	parsed.User = url.UserPassword("mongo_test", "incorrect_password")
	client, err := contactmongo.Open(operationContext(t), parsed.String())
	if client != nil {
		disconnect(t, client)
		t.Fatal("Open returned a client after authentication failed")
	}
	if err == nil {
		t.Fatal("incorrect credentials were accepted")
	}
}

// Run the actual teaching flow twice and verify that its cleanup preserves a
// pre-existing contact. This fixed namespace exists only inside the test server.
func testDemo(t *testing.T, client *mongo.Client, uri string) {
	ctx := operationContext(t)
	store := contactmongo.NewStore(client.Database("mongo_lab"))
	if err := store.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	kept, err := store.Create(ctx, contactmongo.Input{Name: "Keep this contact"})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		var output bytes.Buffer
		if err := demo.Run(ctx, uri, &output); err != nil {
			t.Fatal(err)
		}
		for _, summary := range []string{
			"omitted email is a missing field", "Stored empty text with $set",
			"Duplicate name -> ErrDuplicate", "Data belongs to MongoDB",
			"repeated lookup -> ErrNotFound",
		} {
			if !strings.Contains(output.String(), summary) {
				t.Errorf("demo missing %q", summary)
			}
		}
		page, err := store.List(ctx, 100)
		if err != nil || len(page) != 1 || page[0].ID != kept.ID {
			t.Fatalf("demo cleanup = (%+v, %v)", page, err)
		}
	}
}

func stringPointer(value string) *string { return &value }
