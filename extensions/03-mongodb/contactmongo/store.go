package contactmongo

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Contact is a BSON document. _id is an ObjectID, not a sequential SQL integer.
// A nil Email is omitted on insert; &"" stores an actual empty string. Decoding
// either a missing field or explicit BSON null into *string produces nil.
type Contact struct {
	ID    bson.ObjectID `bson:"_id"`
	Name  string        `bson:"name"`
	Email *string       `bson:"email,omitempty"`
}

// Input contains accepted fields. Name is trimmed; Email is optional text, not
// a full email-address validator. Keep referenced input stable during the call.
type Input struct {
	Name  string
	Email *string
}

var (
	ErrInvalidInput = errors.New("invalid contact input")
	ErrNotFound     = errors.New("contact not found")
	ErrDuplicate    = errors.New("contact uniqueness conflict")
)

// Store borrows a collection handle backed by the caller's reusable client.
// The database and collection names are explicit; the URI's authSource only
// chooses where credentials are checked, not which collection this store uses.
type Store struct{ collection *mongo.Collection }

// NewStore selects this lesson's collection; it does not connect or create data.
func NewStore(database *mongo.Database) *Store {
	return &Store{collection: database.Collection("contacts")}
}

// EnsureIndexes bootstraps a unique, case-sensitive name index without deleting
// documents. Existing duplicate names make creation fail; resolve them explicitly.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	// bson.D is an ordered document: each element is one key/value pair. Here
	// {name: 1} describes an ascending index, rather than a contact's field value.
	_, err := s.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetName("unique_contact_name").SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("ensure name index: %w", queryError(ctx, err))
	}
	return nil
}

// Create sends a typed document. User strings remain BSON values, rather than
// being parsed as query operators. The server's index arbitrates concurrent names.
func (s *Store) Create(ctx context.Context, input Input) (Contact, error) {
	input, err := normalize(input)
	if err != nil {
		return Contact{}, err
	}
	contact := Contact{ID: bson.NewObjectID(), Name: input.Name}
	if input.Email != nil {
		// Own the optional result value so later caller edits cannot change it.
		email := *input.Email
		contact.Email = &email
	}
	if _, err := s.collection.InsertOne(ctx, contact); err != nil {
		return Contact{}, fmt.Errorf("create contact: %w", queryError(ctx, err))
	}
	return contact, nil
}

// Get decodes one typed document. FindOne reports missing documents via Decode.
func (s *Store) Get(ctx context.Context, id bson.ObjectID) (Contact, error) {
	if err := validateID(id); err != nil {
		return Contact{}, err
	}
	var contact Contact
	err := s.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&contact)
	if err != nil {
		return Contact{}, fmt.Errorf("get contact: %w", queryError(ctx, err))
	}
	return contact, nil
}

// List returns an explicit, bounded _id ordering. ObjectIDs are unique but are
// not a globally monotonic creation sequence. An empty collection returns [].
func (s *Store) List(ctx context.Context, limit int) ([]Contact, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("limit must be between 1 and 100: %w", ErrInvalidInput)
	}
	cursor, err := s.collection.Find(ctx, bson.D{}, options.Find().
		SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", queryError(ctx, err))
	}
	// A cursor can own server resources. Close it even after failed iteration,
	// using a fresh cleanup budget if the caller's context has already expired.
	defer func() {
		cleanup, cancel := cleanupContext()
		defer cancel()
		_ = cursor.Close(cleanup)
	}()
	contacts := make([]Contact, 0)
	for cursor.Next(ctx) {
		var contact Contact
		if err := cursor.Decode(&contact); err != nil {
			return nil, fmt.Errorf("decode contact: %w", err)
		}
		contacts = append(contacts, contact)
	}
	// Find succeeding does not guarantee that later network batches succeeded.
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate contacts: %w", queryError(ctx, err))
	}
	return contacts, nil
}

// UpdateEmail changes one document atomically and returns its resulting state.
// nil removes the field with $unset; a nonnil pointer uses $set, including "".
// This teaching operation is separate from the REST lab's PATCH contract.
func (s *Store) UpdateEmail(ctx context.Context, id bson.ObjectID, email *string) (Contact, error) {
	if err := validateID(id); err != nil {
		return Contact{}, err
	}
	if err := validateEmail(email); err != nil {
		return Contact{}, err
	}
	update := bson.D{{Key: "$unset", Value: bson.D{{Key: "email", Value: ""}}}}
	if email != nil {
		update = bson.D{{Key: "$set", Value: bson.D{{Key: "email", Value: *email}}}}
	}
	var contact Contact
	err := s.collection.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: id}}, update,
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&contact)
	if err != nil {
		return Contact{}, fmt.Errorf("update email: %w", queryError(ctx, err))
	}
	return contact, nil
}

// Delete uses DeletedCount to distinguish removal from an already absent ID.
func (s *Store) Delete(ctx context.Context, id bson.ObjectID) error {
	if err := validateID(id); err != nil {
		return err
	}
	result, err := s.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return fmt.Errorf("delete contact: %w", queryError(ctx, err))
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// queryError preserves driver/context causes for errors.Is/As. Duplicate errors
// describe uniqueness generally, so another index is never mislabeled as a name
// conflict. The initial schema's only secondary unique index is on name.
func queryError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ctx.Err(), err)
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	if mongo.IsDuplicateKeyError(err) {
		return fmt.Errorf("%w: %w", ErrDuplicate, err)
	}
	return err
}
