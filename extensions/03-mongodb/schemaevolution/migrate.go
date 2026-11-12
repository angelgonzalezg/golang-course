package schemaevolution

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// BatchResult reports work from this call. On an error, writes may already have
// committed; retry the batch and inspect documents instead of assuming rollback.
type BatchResult struct {
	Selected int64
	Matched  int64
	Modified int64
}

// BackfillBatch selects at most limit valid v1 IDs, then atomically transforms
// each still-legacy document. Repeating it selects only remaining v1 documents.
// This is a data backfill, not a general migration-history/locking framework.
func BackfillBatch(ctx context.Context, collection *mongo.Collection, limit int) (BatchResult, error) {
	if limit < 1 || limit > 100 {
		return BatchResult{}, fmt.Errorf("batch size must be between 1 and 100: %w", ErrInvalidInput)
	}
	legacy, err := decodeValidator(legacyJSON)
	if err != nil {
		return BatchResult{}, err
	}
	cursor, err := collection.Find(ctx, legacy, options.Find().
		SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(limit)).
		SetProjection(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return BatchResult{}, fmt.Errorf("select legacy IDs: %w", queryError(ctx, err))
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = cursor.Close(cleanup)
	}()
	ids := make([]bson.ObjectID, 0, limit)
	for cursor.Next(ctx) {
		var row struct {
			ID bson.ObjectID `bson:"_id"`
		}
		if err := cursor.Decode(&row); err != nil {
			return BatchResult{}, fmt.Errorf("decode legacy ID: %w", err)
		}
		ids = append(ids, row.ID)
	}
	if err := cursor.Err(); err != nil {
		return BatchResult{}, fmt.Errorf("iterate legacy IDs: %w", queryError(ctx, err))
	}
	batch := BatchResult{Selected: int64(len(ids))}
	if len(ids) == 0 {
		return batch, nil
	}
	filter := bson.D{{Key: "$and", Value: bson.A{
		bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: ids}}}}, legacy,
	}}}
	// Pipeline expressions read the city's CURRENT server value, not a stale
	// client-side copy. Version/shape is rechecked in the write filter. Both stages
	// form one atomic update per document; unrelated fields are never replaced.
	update := mongo.Pipeline{
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "address.city", Value: "$city"}, {Key: "schemaVersion", Value: int32(2)},
		}}},
		bson.D{{Key: "$unset", Value: "city"}},
	}
	result, err := collection.UpdateMany(ctx, filter, update)
	if result != nil {
		batch.Matched, batch.Modified = result.MatchedCount, result.ModifiedCount
	}
	if err != nil {
		return batch, fmt.Errorf("backfill legacy profiles: %w", queryError(ctx, err))
	}
	return batch, nil
}
