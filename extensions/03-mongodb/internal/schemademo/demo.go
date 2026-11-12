// Package schemademo demonstrates expand, backfill, and contract on an owned
// collection. Application data access and schema administration use separate clients.
package schemademo

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"example.com/course/mongo-lab/contactmongo"
	"example.com/course/mongo-lab/schemaevolution"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Run creates and finally drops only its own random collection. A full rollout
// is not automatically repeated against a shared application collection.
func Run(ctx context.Context, dataURI, schemaURI string, output io.Writer) (result error) {
	dataClient, err := contactmongo.Open(ctx, dataURI)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		result = errors.Join(result, dataClient.Disconnect(cleanup))
	}()
	schemaClient, err := contactmongo.Open(ctx, schemaURI)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		result = errors.Join(result, schemaClient.Disconnect(cleanup))
	}()
	database := dataClient.Database("mongo_lab")
	name := "schema_demo_" + strings.ToLower(rand.Text()[:12])
	if err := database.CreateCollection(ctx, name); err != nil {
		return err
	}
	collection := database.Collection(name)
	defer func() {
		// Register ownership only after successful creation. No other lab
		// collections or validators are changed, and cleanup gets fresh time.
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result = errors.Join(result, collection.Drop(cleanup))
	}()
	schemaDatabase := schemaClient.Database("mongo_lab")
	fmt.Fprintf(output, "Owned demo collection: %s\n", name)
	fmt.Fprintln(output, "\n== 1. Documents from before server validation ==")
	legacyID, badID := bson.NewObjectID(), bson.NewObjectID()
	_, err = collection.InsertMany(ctx, []any{
		bson.D{{Key: "_id", Value: legacyID}, {Key: "name", Value: "Ana"}, {Key: "city", Value: "Guadalajara"}, {Key: "notes", Value: "keep me"}},
		bson.D{{Key: "name", Value: "Beto"}, {Key: "city", Value: "Puebla"}, {Key: "schemaVersion", Value: int32(1)}},
		bson.D{{Key: "_id", Value: badID}, {Key: "name", Value: "Needs repair"}, {Key: "city", Value: int32(42)}},
	})
	if err != nil {
		return err
	}
	current, err := schemaevolution.Insert(ctx, collection, "Carla", "Monterrey")
	if err != nil {
		return err
	}

	fmt.Fprintln(output, "\n== 2. Expand: strict validation accepting v1 and v2 ==")
	if err := schemaevolution.InstallCompatible(ctx, schemaDatabase, name); err != nil {
		return err
	}
	_, err = collection.InsertOne(ctx, bson.D{{Key: "name", Value: "Rejected"}, {Key: "city", Value: int32(42)}})
	if !schemaevolution.IsValidationFailure(err) {
		return fmt.Errorf("expected server validation failure, got %v", err)
	}
	if err := schemaevolution.CheckCompatible(ctx, collection); !errors.Is(err, schemaevolution.ErrInvalidDocument) {
		return fmt.Errorf("expected invalid pre-existing data, got %v", err)
	}
	fmt.Fprintln(output, "New invalid write -> server code 121; an invalid old document still exists.")
	old, err := schemaevolution.Read(ctx, collection, legacyID)
	if err != nil {
		return err
	}
	newer, err := schemaevolution.Read(ctx, collection, current.ID)
	if err != nil {
		return err
	}
	if old.StoredVersion != 1 || newer.StoredVersion != 2 {
		return errors.New("mixed-version reader reported the wrong versions")
	}
	fmt.Fprintln(output, "One reader understands v1 city and v2 address.city; new Go writes use v2.")

	fmt.Fprintln(output, "\n== 3. Repair explicitly, then audit before migration ==")
	_, err = collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: badID}}, bson.D{{Key: "$set", Value: bson.D{{Key: "city", Value: "Monterrey"}}}})
	if err != nil {
		return err
	}
	if err := schemaevolution.CheckCompatible(ctx, collection); err != nil {
		return err
	}
	fmt.Fprintln(output, "Repaired the known bad value; existing documents now match a supported schema.")

	fmt.Fprintln(output, "\n== 4. Backfill in bounded, repeatable batches ==")
	first, err := schemaevolution.BackfillBatch(ctx, collection, 2)
	if err != nil {
		return err
	}
	if first.Modified != 2 {
		return fmt.Errorf("first batch modified %d documents, want 2", first.Modified)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := schemaevolution.BackfillBatch(canceled, collection, 2); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("expected canceled batch, got %v", err)
	}
	total := first.Modified
	for {
		batch, err := schemaevolution.BackfillBatch(ctx, collection, 2)
		if err != nil {
			return err
		}
		total += batch.Modified
		if batch.Selected == 0 {
			break
		}
	}
	if total != 3 {
		return fmt.Errorf("backfill modified %d documents, want 3", total)
	}
	repeat, err := schemaevolution.BackfillBatch(ctx, collection, 2)
	if err != nil {
		return err
	}
	if repeat.Modified != 0 {
		return errors.New("repeat backfill changed current documents")
	}
	raw, err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: legacyID}}).Raw()
	if err != nil {
		return err
	}
	if raw.Lookup("notes").StringValue() != "keep me" {
		return errors.New("migration lost an unrelated field")
	}
	fmt.Fprintln(output, "First batch committed; cancellation did not undo it. Resume -> 3 migrated; repeat -> 0.")

	fmt.Fprintln(output, "\n== 5. Contract: require v2 and audit again ==")
	if err := schemaevolution.Finalize(ctx, schemaDatabase, collection); err != nil {
		return err
	}
	_, err = collection.InsertOne(ctx, bson.D{{Key: "name", Value: "Old writer"}, {Key: "city", Value: "Puebla"}})
	if !schemaevolution.IsValidationFailure(err) {
		return fmt.Errorf("expected old writer rejection, got %v", err)
	}
	if _, err := schemaevolution.Insert(ctx, collection, "Current writer", "Puebla"); err != nil {
		return err
	}
	converted, err := schemaevolution.Read(ctx, collection, legacyID)
	if err != nil {
		return err
	}
	if converted.StoredVersion != 2 || converted.City != old.City {
		return errors.New("migration changed the legacy city")
	}
	fmt.Fprintln(output, "v2-only policy: legacy writer rejected; current writer accepted; original city preserved.")
	fmt.Fprintln(output, "Cleanup drops only this demo's random collection and its validator.")
	return nil
}
