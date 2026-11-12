// Package contactdoc teaches embedded documents, array queries, and projections.
// Its profiles collection gives the second lesson its own small document model.
package contactdoc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Address belongs to one profile and is stored inside that profile's document.
// Its fields are strings, so copying Address gives an independent Go value.
type Address struct {
	Street string `bson:"street"`
	City   string `bson:"city"`
}

// Profile embeds an address document and a bounded array of tags. Names can
// repeat; _id identifies a profile. A copied Profile can still share its Tags
// backing array, so callers must copy that slice when they need independent edits.
type Profile struct {
	ID      bson.ObjectID `bson:"_id"`
	Name    string        `bson:"name"`
	Address Address       `bson:"address"`
	Tags    []string      `bson:"tags"`
}

// Input supplies a profile's editable fields. Keep its slice stable during calls.
type Input struct {
	Name    string
	Address Address
	Tags    []string
}

// Summary is the read model for a projected query. It includes the complete
// address but has no Tags field: a partial read is not a full Profile to save back.
type Summary struct {
	ID      bson.ObjectID `bson:"_id"`
	Name    string        `bson:"name"`
	Address Address       `bson:"address"`
}

// Query uses optional exact city and tag criteria, combined with AND. A tag
// matches an element of the stored array. Limit is required and must be 1–100.
type Query struct {
	City  string
	Tag   string
	Limit int
}

var (
	ErrInvalidInput = errors.New("invalid profile input")
	ErrNotFound     = errors.New("profile not found")
)

// Store borrows a collection through an application-owned MongoDB client.
type Store struct{ collection *mongo.Collection }

// NewStore selects the second lesson's collection without issuing a command.
func NewStore(database *mongo.Database) *Store {
	return &Store{collection: database.Collection("contact_profiles")}
}

// EnsureIndexes bootstraps indexes for the example's city/order and tag queries.
// A query combining criteria still needs measurement; these are not a promise
// that every query/sort shape is covered by one index.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	_, err := s.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "address.city", Value: 1}, {Key: "name", Value: 1}, {Key: "_id", Value: 1}},
			Options: options.Index().SetName("city_name_id"),
		},
		{
			// Indexing an array creates a multikey index, with entries for its values.
			Keys: bson.D{{Key: "tags", Value: 1}}, Options: options.Index().SetName("profile_tags"),
		},
	})
	if err != nil {
		return fmt.Errorf("ensure profile indexes: %w", queryError(ctx, err))
	}
	return nil
}

// Create validates and copies caller-owned tags before encoding the full document.
// The embedded address is inserted with its parent in the same document write.
func (s *Store) Create(ctx context.Context, input Input) (Profile, error) {
	input, err := normalize(input)
	if err != nil {
		return Profile{}, err
	}
	profile := Profile{ID: bson.NewObjectID(), Name: input.Name, Address: input.Address, Tags: input.Tags}
	if _, err := s.collection.InsertOne(ctx, profile); err != nil {
		return Profile{}, fmt.Errorf("create profile: %w", queryError(ctx, err))
	}
	return profile, nil
}

// Get reads the full document, including tags omitted by Search's projection.
func (s *Store) Get(ctx context.Context, id bson.ObjectID) (Profile, error) {
	if err := validateID(id); err != nil {
		return Profile{}, err
	}
	var profile Profile
	if err := s.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&profile); err != nil {
		return Profile{}, fmt.Errorf("get profile: %w", queryError(ctx, err))
	}
	return profile, nil
}

// Search projects only summary fields and bounds returned documents. Sorting by
// name then _id gives equal names a deterministic tie breaker in this result set.
func (s *Store) Search(ctx context.Context, query Query) ([]Summary, error) {
	query, err := normalizeQuery(query)
	if err != nil {
		return nil, err
	}
	filter := bson.D{}
	if query.City != "" {
		// Dot notation matches a child field instead of the whole address object.
		filter = append(filter, bson.E{Key: "address.city", Value: query.City})
	}
	if query.Tag != "" {
		// A scalar equality on tags matches that value in the array. The caller
		// provides a value, not field names, operators, or executable query text.
		filter = append(filter, bson.E{Key: "tags", Value: query.Tag})
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}).
		SetLimit(int64(query.Limit)).
		SetProjection(bson.D{{Key: "name", Value: 1}, {Key: "address", Value: 1}})
	// Inclusion projection retains _id by default and excludes unlisted fields.
	cursor, err := s.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("search profiles: %w", queryError(ctx, err))
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = cursor.Close(cleanup)
	}()
	summaries := make([]Summary, 0)
	for cursor.Next(ctx) {
		var summary Summary
		if err := cursor.Decode(&summary); err != nil {
			return nil, fmt.Errorf("decode profile summary: %w", err)
		}
		summaries = append(summaries, summary)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate profile summaries: %w", queryError(ctx, err))
	}
	return summaries, nil
}

// UpdateCity changes only address.city. Setting the entire address to a document
// containing only city would replace it and lose street. The single-document
// update is atomic, and ReturnDocument(After) reads its resulting full state.
func (s *Store) UpdateCity(ctx context.Context, id bson.ObjectID, city string) (Profile, error) {
	if err := validateID(id); err != nil {
		return Profile{}, err
	}
	city, err := requiredText("city", city, 80)
	if err != nil {
		return Profile{}, err
	}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "address.city", Value: city}}}}
	var profile Profile
	err = s.collection.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: id}}, update,
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&profile)
	if err != nil {
		return Profile{}, fmt.Errorf("update city: %w", queryError(ctx, err))
	}
	return profile, nil
}

// Delete removes only the specified profile, including its embedded address/tags.
func (s *Store) Delete(ctx context.Context, id bson.ObjectID) error {
	if err := validateID(id); err != nil {
		return err
	}
	result, err := s.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return fmt.Errorf("delete profile: %w", queryError(ctx, err))
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// Preserve the domain, driver, and context causes instead of parsing messages.
func queryError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ctx.Err(), err)
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return err
}
