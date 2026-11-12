// Package reporting teaches references, typed aggregation reports, and explain.
// Its two collections belong to the same database and have independent documents.
package reporting

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Company can be updated independently of profiles that reference its ID.
type Company struct {
	ID   bson.ObjectID `bson:"_id"`
	Name string        `bson:"name"`
}

// Profile stores an optional company ID, not a copy of the company document.
// CompanyID is a Go pointer for presence; the stored reference is its ObjectID.
type Profile struct {
	ID        bson.ObjectID  `bson:"_id"`
	Name      string         `bson:"name"`
	City      string         `bson:"city"`
	Tags      []string       `bson:"tags"`
	CompanyID *bson.ObjectID `bson:"company_id,omitempty"`
}

// ProfileInput supplies report data. Keep referenced values stable during calls.
type ProfileInput struct {
	Name      string
	City      string
	Tags      []string
	CompanyID *bson.ObjectID
}

var (
	ErrInvalidInput = errors.New("invalid reporting input")
	ErrNotFound     = errors.New("company not found")
)

const cityIndex = "report_city_name_id"

// Store borrows application-owned handles. The collection names are trusted
// application configuration; choose two distinct collections in this database.
type Store struct {
	profiles  *mongo.Collection
	companies *mongo.Collection
}

// NewStore selects both collections on the same database/client. It issues no
// commands; the application owns creation, client shutdown, and collection lifetime.
func NewStore(database *mongo.Database, profileCollection, companyCollection string) *Store {
	return &Store{profiles: database.Collection(profileCollection), companies: database.Collection(companyCollection)}
}

// EnsureIndexes adds the local city/order index. The foreign _id index already
// supports the equality join; a local reference alone does not enforce integrity.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	_, err := s.profiles.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "city", Value: 1}, {Key: "name", Value: 1}, {Key: "_id", Value: 1}},
		Options: options.Index().SetName(cityIndex),
	})
	if err != nil {
		return fmt.Errorf("ensure report index: %w", queryError(ctx, err))
	}
	return nil
}

// CreateCompany gives the independent company document its own ObjectID.
func (s *Store) CreateCompany(ctx context.Context, name string) (Company, error) {
	name, err := text("company name", name, 80)
	if err != nil {
		return Company{}, err
	}
	company := Company{ID: bson.NewObjectID(), Name: name}
	if _, err := s.companies.InsertOne(ctx, company); err != nil {
		return Company{}, fmt.Errorf("create company: %w", queryError(ctx, err))
	}
	return company, nil
}

// CreateProfile copies optional/slice input before returning it. It deliberately
// accepts a valid but nonexistent company ID: MongoDB does not provide an automatic
// foreign-key check. Application lifecycle/consistency policies must define that.
func (s *Store) CreateProfile(ctx context.Context, input ProfileInput) (Profile, error) {
	input, err := normalize(input)
	if err != nil {
		return Profile{}, err
	}
	profile := Profile{ID: bson.NewObjectID(), Name: input.Name, City: input.City, Tags: input.Tags, CompanyID: input.CompanyID}
	if _, err := s.profiles.InsertOne(ctx, profile); err != nil {
		return Profile{}, fmt.Errorf("create report profile: %w", queryError(ctx, err))
	}
	return profile, nil
}

// RenameCompany changes a shared source of truth. Later lookups see that value
// without rewriting every referencing profile; this is not a cross-query snapshot.
func (s *Store) RenameCompany(ctx context.Context, id bson.ObjectID, name string) (Company, error) {
	if err := validID(id); err != nil {
		return Company{}, err
	}
	name, err := text("company name", name, 80)
	if err != nil {
		return Company{}, err
	}
	var company Company
	err = s.companies.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "name", Value: name}}}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&company)
	if err != nil {
		return Company{}, fmt.Errorf("rename company: %w", queryError(ctx, err))
	}
	return company, nil
}

// DeleteCompany removes only the company. Existing references are retained;
// the directory demonstrates missing-target handling instead of cascading deletes.
func (s *Store) DeleteCompany(ctx context.Context, id bson.ObjectID) error {
	if err := validID(id); err != nil {
		return err
	}
	result, err := s.companies.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return fmt.Errorf("delete company: %w", queryError(ctx, err))
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func queryError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ctx.Err(), err)
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return err
}
