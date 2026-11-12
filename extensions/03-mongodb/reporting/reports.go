package reporting

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// DirectoryEntry keeps an absent company separate from an absent reference:
// nil CompanyID means unassigned; a nonnil ID with nil Company is dangling.
type DirectoryEntry struct {
	ID        bson.ObjectID  `bson:"_id"`
	Name      string         `bson:"name"`
	City      string         `bson:"city"`
	CompanyID *bson.ObjectID `bson:"company_id,omitempty"`
	Company   *Company       `bson:"company,omitempty"`
}

// CityCount is a grouped read model; counts use int64 for growing datasets.
type CityCount struct {
	City     string `bson:"city"`
	Profiles int64  `bson:"profiles"`
}

// TagCount counts distinct source profiles for one tag, not occurrences in arrays.
type TagCount struct {
	Tag      string `bson:"tag"`
	Profiles int64  `bson:"profiles"`
}

// Directory uses a left join and preserves profiles even without a matching
// company. Limit precedes lookup because foreign _id supplies at most one match;
// that placement would change meaning for a later filter on joined fields.
func (s *Store) Directory(ctx context.Context, city string, limit int) ([]DirectoryEntry, error) {
	city, err := query(city, limit)
	if err != nil {
		return nil, err
	}
	filter := bson.D{}
	if city != "" {
		filter = append(filter, bson.E{Key: "city", Value: city})
	}
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}}},
		bson.D{{Key: "$limit", Value: int64(limit)}},
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: s.companies.Name()}, {Key: "localField", Value: "company_id"},
			{Key: "foreignField", Value: "_id"}, {Key: "as", Value: "company"},
		}}},
		// A lookup emits an array. Preserving empty arrays keeps missing targets.
		bson.D{{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$company"}, {Key: "preserveNullAndEmptyArrays", Value: true}}}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "name", Value: 1}, {Key: "city", Value: 1}, {Key: "company_id", Value: 1},
			{Key: "company._id", Value: 1}, {Key: "company.name", Value: 1},
		}}},
	}
	return aggregate[DirectoryEntry](ctx, s.profiles, pipeline)
}

// CityTotals counts profiles, not joined rows. Group before limit so a small
// requested report does not silently count only a prefix of the source data.
func (s *Store) CityTotals(ctx context.Context, limit int) ([]CityCount, error) {
	if _, err := query("", limit); err != nil {
		return nil, err
	}
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$city"}, {Key: "profiles", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
		bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "city", Value: "$_id"}, {Key: "profiles", Value: 1}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "profiles", Value: -1}, {Key: "city", Value: 1}}}},
		bson.D{{Key: "$limit", Value: int64(limit)}},
	}
	return aggregate[CityCount](ctx, s.profiles, pipeline)
}

// TagTotals counts each profile once per tag, even if another writer supplied
// duplicate array elements. Empty arrays contribute no tag rows after unwind.
func (s *Store) TagTotals(ctx context.Context, city string, limit int) ([]TagCount, error) {
	city, err := query(city, limit)
	if err != nil {
		return nil, err
	}
	filter := bson.D{}
	if city != "" {
		filter = append(filter, bson.E{Key: "city", Value: city})
	}
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		// Set-union removes duplicate tags within each source profile.
		bson.D{{Key: "$project", Value: bson.D{{Key: "tags", Value: bson.D{{Key: "$setUnion", Value: bson.A{"$tags", bson.A{}}}}}}}},
		bson.D{{Key: "$unwind", Value: "$tags"}},
		bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$tags"}, {Key: "profiles", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
		bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "tag", Value: "$_id"}, {Key: "profiles", Value: 1}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "profiles", Value: -1}, {Key: "tag", Value: 1}}}},
		bson.D{{Key: "$limit", Value: int64(limit)}},
	}
	return aggregate[TagCount](ctx, s.profiles, pipeline)
}

// One cursor owner serves three typed read models. The generic parameter keeps
// the public results typed; the visible pipelines describe each transformation.
func aggregate[T any](ctx context.Context, collection *mongo.Collection, pipeline mongo.Pipeline) ([]T, error) {
	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate report: %w", queryError(ctx, err))
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = cursor.Close(cleanup)
	}()
	rows := make([]T, 0)
	for cursor.Next(ctx) {
		var row T
		if err := cursor.Decode(&row); err != nil {
			return nil, fmt.Errorf("decode report: %w", err)
		}
		rows = append(rows, row)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate report: %w", queryError(ctx, err))
	}
	return rows, nil
}
