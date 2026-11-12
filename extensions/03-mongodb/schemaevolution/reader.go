package schemaevolution

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Profile is a normalized read model, not a document to replace in MongoDB.
// Both storage versions expose City here; StoredVersion shows which was read.
type Profile struct {
	ID            bson.ObjectID
	Name          string
	City          string
	StoredVersion int32
}

// Insert writes only v2. New writers can deploy while the reader still accepts
// old documents. Input normalization belongs to Go; the server checks all writers.
func Insert(ctx context.Context, collection *mongo.Collection, name, city string) (Profile, error) {
	name, city = strings.TrimSpace(name), strings.TrimSpace(city)
	if !validText(name) || !validText(city) {
		return Profile{}, fmt.Errorf("name and city require 1–80 UTF-8 runes without NUL: %w", ErrInvalidInput)
	}
	profile := Profile{ID: bson.NewObjectID(), Name: name, City: city, StoredVersion: 2}
	document := bson.D{
		{Key: "_id", Value: profile.ID}, {Key: "name", Value: name},
		{Key: "schemaVersion", Value: int32(2)},
		{Key: "address", Value: bson.D{{Key: "city", Value: city}}},
	}
	if _, err := collection.InsertOne(ctx, document); err != nil {
		return Profile{}, fmt.Errorf("insert current profile: %w", queryError(ctx, err))
	}
	return profile, nil
}

// Read recognizes storage versions instead of assuming every document has the
// newest shape. Unknown versions and invalid shapes remain explicit failures.
func Read(ctx context.Context, collection *mongo.Collection, id bson.ObjectID) (Profile, error) {
	if id.IsZero() {
		return Profile{}, fmt.Errorf("ObjectID must not be zero: %w", ErrInvalidInput)
	}
	raw, err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Raw()
	if err != nil {
		return Profile{}, fmt.Errorf("read versioned profile: %w", queryError(ctx, err))
	}
	return Decode(raw)
}

// Decode is a pure compatibility adapter. A missing schemaVersion denotes the
// original v1 shape; explicit null, strings, floats, or future versions do not.
// LookupErr distinguishes missing fields from explicitly present null values.
func Decode(raw bson.Raw) (Profile, error) {
	if err := raw.Validate(); err != nil {
		return Profile{}, fmt.Errorf("malformed BSON: %w: %w", ErrInvalidDocument, err)
	}
	id, idOK := raw.Lookup("_id").ObjectIDOK()
	name, nameOK := raw.Lookup("name").StringValueOK()
	if !idOK || !nameOK || !validText(name) {
		return Profile{}, ErrInvalidDocument
	}
	version := int64(1)
	value, err := raw.LookupErr("schemaVersion")
	if err == nil {
		switch value.Type {
		case bson.TypeInt32:
			version = int64(value.Int32())
		case bson.TypeInt64:
			version = value.Int64()
		default:
			return Profile{}, ErrInvalidDocument
		}
	}
	var city string
	var cityOK bool
	switch version {
	case 1:
		if _, err := raw.LookupErr("address"); err == nil {
			return Profile{}, ErrInvalidDocument
		}
		city, cityOK = raw.Lookup("city").StringValueOK()
	case 2:
		if _, err := raw.LookupErr("city"); err == nil || raw.Lookup("address").Type != bson.TypeEmbeddedDocument {
			return Profile{}, ErrInvalidDocument
		}
		city, cityOK = raw.Lookup("address", "city").StringValueOK()
	default:
		return Profile{}, fmt.Errorf("schemaVersion %d: %w", version, ErrUnsupportedVersion)
	}
	if !cityOK || !validText(city) {
		return Profile{}, ErrInvalidDocument
	}
	return Profile{ID: id, Name: name, City: city, StoredVersion: int32(version)}, nil
}

// Existing text is preserved. Only new writes trim it; migration never invents
// a replacement for malformed values or changes the meaning of legacy text.
func validText(value string) bool {
	length := utf8.RuneCountInString(value)
	return utf8.ValidString(value) && length >= 1 && length <= 80 && !strings.ContainsRune(value, 0)
}
