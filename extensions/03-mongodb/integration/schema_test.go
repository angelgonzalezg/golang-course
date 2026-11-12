//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/course/mongo-lab/contactmongo"
	"example.com/course/mongo-lab/internal/schemademo"
	"example.com/course/mongo-lab/schemaevolution"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Use separate database-scoped accounts against a real, owned MongoDB server.
// Every case owns a collection; neither connection comes from environment variables.
func TestSchemaEvolution(t *testing.T) {
	root, uri := newTestDatabase(t)
	dataDB, schemaDB, dataURI, schemaURI := schemaAccounts(t, root, uri)
	t.Run("validation and permissions", func(t *testing.T) { testSchemaValidation(t, dataDB, schemaDB) })
	t.Run("invalid old documents", func(t *testing.T) { testInvalidLegacy(t, dataDB, schemaDB) })
	t.Run("resume and concurrent backfill", func(t *testing.T) { testBackfillResume(t, dataDB, schemaDB, dataURI) })
	t.Run("cutover audits after closing old writes", func(t *testing.T) { testSchemaCutover(t, dataDB, schemaDB) })
	t.Run("future version is retained", func(t *testing.T) { testFutureSchema(t, dataDB, schemaDB) })
	t.Run("finite demo cleanup", func(t *testing.T) { testSchemaDemo(t, dataDB, dataURI, schemaURI) })
}

// Root creates only fixture users. The exercised connections have the same
// readWrite/dbAdmin split as Compose; dbAdmin is not the application's data role.
func schemaAccounts(t *testing.T, root *mongo.Client, uri string) (*mongo.Database, *mongo.Database, string, string) {
	t.Helper()
	ctx := operationContext(t)
	database := root.Database("mongo_lab")
	open := func(username, role string) (*mongo.Database, string) {
		err := database.RunCommand(ctx, bson.D{
			{Key: "createUser", Value: username}, {Key: "pwd", Value: "fixture_password"},
			{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: role}, {Key: "db", Value: "mongo_lab"}}}},
		}).Err()
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := url.Parse(uri)
		if err != nil {
			t.Fatal(err)
		}
		parsed.User, parsed.Path = url.UserPassword(username, "fixture_password"), "/mongo_lab"
		query := parsed.Query()
		query.Set("authSource", "mongo_lab")
		parsed.RawQuery = query.Encode()
		client, err := contactmongo.Open(ctx, parsed.String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { disconnect(t, client) })
		return client.Database("mongo_lab"), parsed.String()
	}
	dataDB, dataURI := open("schema_data_test", "readWrite")
	schemaDB, schemaURI := open("schema_admin_test", "dbAdmin")
	return dataDB, schemaDB, dataURI, schemaURI
}

// Register cleanup immediately after creation. Only this case's random
// collection is dropped, and cleanup remains possible after operation timeout.
func schemaCollection(t *testing.T, database *mongo.Database) *mongo.Collection {
	t.Helper()
	name := "schema_test_" + strings.ToLower(rand.Text()[:12])
	if err := database.CreateCollection(operationContext(t), name); err != nil {
		t.Fatal(err)
	}
	collection := database.Collection(name)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := collection.Drop(ctx); err != nil {
			t.Error(err)
		}
	})
	return collection
}

// Server rules cover callers that bypass the Go input functions. The normal
// data account cannot change validation, and the schema account cannot insert data.
func testSchemaValidation(t *testing.T, dataDB, schemaDB *mongo.Database) {
	ctx := operationContext(t)
	collection := schemaCollection(t, dataDB)
	err := schemaevolution.InstallCompatible(ctx, dataDB, collection.Name())
	var denied mongo.CommandError
	if !errors.As(err, &denied) || denied.Code != 13 {
		t.Fatalf("data account collMod = %v", err)
	}
	_, err = schemaDB.Collection(collection.Name()).InsertOne(ctx, legacySchemaDoc("No data permission", "Puebla"))
	var server mongo.ServerError
	if !errors.As(err, &server) || !server.HasErrorCode(13) {
		t.Fatalf("schema account insert = %v", err)
	}
	if err := schemaevolution.InstallCompatible(ctx, schemaDB, collection.Name()); err != nil {
		t.Fatal(err)
	}
	if _, err := collection.InsertOne(ctx, legacySchemaDoc("Legacy", "Guadalajara")); err != nil {
		t.Fatal(err)
	}
	current, err := schemaevolution.Insert(ctx, collection, " Current ", " Puebla ")
	if err != nil || current.Name != "Current" || current.City != "Puebla" || current.StoredVersion != 2 {
		t.Fatalf("current = (%+v, %v)", current, err)
	}
	for _, document := range []bson.D{
		legacySchemaDoc("Wrong city", int32(42)),
		legacySchemaDoc("Null city", nil),
		legacySchemaDoc("Long city", strings.Repeat("ñ", 81)),
		legacySchemaDoc("NUL city", "a\x00b"),
		{{Key: "name", Value: int32(42)}, {Key: "city", Value: "Puebla"}},
		{{Key: "_id", Value: "not-an-object-id"}, {Key: "name", Value: "Wrong ID"}, {Key: "city", Value: "Puebla"}},
		append(legacySchemaDoc("Address collision", "Puebla"), bson.E{Key: "address", Value: nil}),
		append(legacySchemaDoc("Null version", "Puebla"), bson.E{Key: "schemaVersion", Value: nil}),
		append(legacySchemaDoc("Float version", "Puebla"), bson.E{Key: "schemaVersion", Value: float64(1)}),
		{{Key: "name", Value: "Invalid current"}, {Key: "schemaVersion", Value: int32(2)}, {Key: "address", Value: nil}},
	} {
		if _, err := collection.InsertOne(ctx, document); !schemaevolution.IsValidationFailure(err) {
			t.Errorf("bad raw write = %v", err)
		}
	}
	_, err = collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: current.ID}}, bson.D{{Key: "$set", Value: bson.D{{Key: "name", Value: int32(42)}}}})
	if !schemaevolution.IsValidationFailure(err) {
		t.Fatalf("invalid update = %v", err)
	}
}

// Strict/error checks new writes, not pre-existing data. Backfill skips invalid
// data, the audit flags it, and an explicit valid repair makes migration possible.
func testInvalidLegacy(t *testing.T, dataDB, schemaDB *mongo.Database) {
	ctx := operationContext(t)
	collection := schemaCollection(t, dataDB)
	id := bson.NewObjectID()
	document := append(legacySchemaDoc("Needs repair", int32(42)), bson.E{Key: "_id", Value: id})
	if _, err := collection.InsertOne(ctx, document); err != nil {
		t.Fatal(err)
	}
	if err := schemaevolution.InstallCompatible(ctx, schemaDB, collection.Name()); err != nil {
		t.Fatal(err)
	}
	if err := schemaevolution.CheckCompatible(ctx, collection); !errors.Is(err, schemaevolution.ErrInvalidDocument) {
		t.Fatalf("old-data audit = %v", err)
	}
	if _, err := schemaevolution.Read(ctx, collection, id); !errors.Is(err, schemaevolution.ErrInvalidDocument) {
		t.Fatalf("read invalid = %v", err)
	}
	batch, err := schemaevolution.BackfillBatch(ctx, collection, 2)
	if err != nil || batch.Selected != 0 {
		t.Fatalf("invalid data was selected = (%+v, %v)", batch, err)
	}
	_, err = collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: id}}, bson.D{{Key: "$set", Value: bson.D{{Key: "notes", Value: "not a repair"}}}})
	if !schemaevolution.IsValidationFailure(err) {
		t.Fatalf("update leaving invalid data = %v", err)
	}
	_, err = collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: id}}, bson.D{{Key: "$set", Value: bson.D{{Key: "city", Value: "Puebla"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := schemaevolution.CheckCompatible(ctx, collection); err != nil {
		t.Fatal(err)
	}
	batch, err = schemaevolution.BackfillBatch(ctx, collection, 2)
	if err != nil || batch.Modified != 1 {
		t.Fatalf("repaired migration = (%+v, %v)", batch, err)
	}
}

// Commit one batch, cancel the next, reconnect, and let two workers finish.
// Per-document version filters prevent duplicate transformations and preserve
// fields outside the migration, including text starting with a dollar sign.
func testBackfillResume(t *testing.T, dataDB, schemaDB *mongo.Database, dataURI string) {
	ctx := operationContext(t)
	collection := schemaCollection(t, dataDB)
	var documents []any
	for i := range 7 {
		document := legacySchemaDoc("Legacy", "$literal-city")
		document = append(document, bson.E{Key: "notes", Value: bson.D{{Key: "sequence", Value: int32(i)}}})
		if i%2 == 0 {
			document = append(document, bson.E{Key: "schemaVersion", Value: int64(1)})
		}
		documents = append(documents, document)
	}
	if _, err := collection.InsertMany(ctx, documents); err != nil {
		t.Fatal(err)
	}
	if _, err := schemaevolution.Insert(ctx, collection, "Already current", "Puebla"); err != nil {
		t.Fatal(err)
	}
	if err := schemaevolution.InstallCompatible(ctx, schemaDB, collection.Name()); err != nil {
		t.Fatal(err)
	}
	first, err := schemaevolution.BackfillBatch(ctx, collection, 2)
	if err != nil || first.Selected != 2 || first.Modified != 2 {
		t.Fatalf("first batch = (%+v, %v)", first, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := schemaevolution.BackfillBatch(canceled, collection, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled batch = %v", err)
	}
	client, err := contactmongo.Open(ctx, dataURI)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { disconnect(t, client) })
	resumed := client.Database(dataDB.Name()).Collection(collection.Name())
	type outcome struct {
		modified int64
		err      error
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			var total int64
			for {
				batch, err := schemaevolution.BackfillBatch(ctx, resumed, 2)
				total += batch.Modified
				if err != nil || batch.Selected == 0 {
					results <- outcome{total, err}
					return
				}
			}
		}()
	}
	total := first.Modified
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Error(result.err)
		}
		total += result.modified
	}
	if total != 7 {
		t.Fatalf("total transformations = %d, want 7", total)
	}
	repeat, err := schemaevolution.BackfillBatch(ctx, resumed, 2)
	if err != nil || repeat.Selected != 0 || repeat.Modified != 0 {
		t.Fatalf("repeat = (%+v, %v)", repeat, err)
	}
	count, err := resumed.CountDocuments(ctx, bson.D{{Key: "schemaVersion", Value: int32(2)}, {Key: "address.city", Value: "$literal-city"}, {Key: "notes", Value: bson.D{{Key: "$exists", Value: true}}}})
	if err != nil || count != 7 {
		t.Fatalf("preserved legacy values = (%d, %v)", count, err)
	}
	if err := schemaevolution.Finalize(ctx, schemaDB, resumed); err != nil {
		t.Fatal(err)
	}
}

// Installing v2 alone cannot certify existing data. Finalize closes old writes
// before its audit; a failed audit keeps v2 active while old rows are backfilled.
func testSchemaCutover(t *testing.T, dataDB, schemaDB *mongo.Database) {
	ctx := operationContext(t)
	collection := schemaCollection(t, dataDB)
	if _, err := collection.InsertOne(ctx, legacySchemaDoc("Pending", "Puebla")); err != nil {
		t.Fatal(err)
	}
	if err := schemaevolution.InstallCompatible(ctx, schemaDB, collection.Name()); err != nil {
		t.Fatal(err)
	}
	if err := schemaevolution.Finalize(ctx, schemaDB, collection); !errors.Is(err, schemaevolution.ErrMigrationRequired) {
		t.Fatalf("premature finalization = %v", err)
	}
	if _, err := collection.InsertOne(ctx, legacySchemaDoc("Old writer", "Puebla")); !schemaevolution.IsValidationFailure(err) {
		t.Fatalf("old writer after failed audit = %v", err)
	}
	batch, err := schemaevolution.BackfillBatch(ctx, collection, 2)
	if err != nil || batch.Modified != 1 {
		t.Fatalf("backfill under v2 policy = (%+v, %v)", batch, err)
	}
	if err := schemaevolution.Finalize(ctx, schemaDB, collection); err != nil {
		t.Fatal(err)
	}
	if _, err := schemaevolution.BackfillBatch(ctx, collection, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := collection.InsertOne(ctx, legacySchemaDoc("Still rejected", "Puebla")); !schemaevolution.IsValidationFailure(err) {
		t.Fatalf("repeat backfill weakened policy: %v", err)
	}
	if _, err := schemaevolution.Insert(ctx, collection, "New writer", "Puebla"); err != nil {
		t.Fatal(err)
	}
}

// A future version is neither guessed as v1 nor silently downgraded. Keep the
// record unchanged and report that the collection is not ready for completion.
func testFutureSchema(t *testing.T, dataDB, schemaDB *mongo.Database) {
	ctx := operationContext(t)
	collection := schemaCollection(t, dataDB)
	id := bson.NewObjectID()
	_, err := collection.InsertOne(ctx, bson.D{{Key: "_id", Value: id}, {Key: "name", Value: "Future"}, {Key: "city", Value: "Puebla"}, {Key: "schemaVersion", Value: int32(99)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := schemaevolution.InstallCompatible(ctx, schemaDB, collection.Name()); err != nil {
		t.Fatal(err)
	}
	if _, err := schemaevolution.Read(ctx, collection, id); !errors.Is(err, schemaevolution.ErrUnsupportedVersion) {
		t.Fatalf("future reader = %v", err)
	}
	if err := schemaevolution.CheckCompatible(ctx, collection); !errors.Is(err, schemaevolution.ErrInvalidDocument) {
		t.Fatalf("future audit = %v", err)
	}
	batch, err := schemaevolution.BackfillBatch(ctx, collection, 2)
	if err != nil || batch.Modified != 0 {
		t.Fatalf("future backfill = (%+v, %v)", batch, err)
	}
	if err := schemaevolution.Finalize(ctx, schemaDB, collection); !errors.Is(err, schemaevolution.ErrMigrationRequired) {
		t.Fatalf("future finalization = %v", err)
	}
	raw, err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Raw()
	if err != nil || raw.Lookup("schemaVersion").Int32() != 99 {
		t.Fatalf("future document changed: %v", err)
	}
}

// The actual demo runs twice with scoped credentials, removes its collection,
// and preserves existing data from an earlier stage in the same database.
func testSchemaDemo(t *testing.T, dataDB *mongo.Database, dataURI, schemaURI string) {
	ctx := operationContext(t)
	kept, err := contactmongo.NewStore(dataDB).Create(ctx, contactmongo.Input{Name: "Keep schema-demo neighbor"})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		var output bytes.Buffer
		if err := schemademo.Run(ctx, dataURI, schemaURI, &output); err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{"server code 121", "One reader understands v1", "Resume -> 3 migrated; repeat -> 0", "legacy writer rejected"} {
			if !strings.Contains(output.String(), expected) {
				t.Errorf("demo missing %q", expected)
			}
		}
		if _, err := contactmongo.NewStore(dataDB).Get(ctx, kept.ID); err != nil {
			t.Fatalf("neighbor was lost: %v", err)
		}
		names, err := dataDB.ListCollectionNames(ctx, bson.D{{Key: "name", Value: bson.D{{Key: "$regex", Value: "^schema_demo_"}}}})
		if err != nil || len(names) != 0 {
			t.Fatalf("demo collections remained = (%v, %v)", names, err)
		}
	}
}

func legacySchemaDoc(name string, city any) bson.D {
	return bson.D{{Key: "name", Value: name}, {Key: "city", Value: city}}
}
