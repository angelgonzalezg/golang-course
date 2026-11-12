package schemaevolution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Missing versions mean the known legacy shape, not any arbitrary document.
// A reader normalizes supported versions while rejecting ambiguous/future data.
func TestDecodeVersions(t *testing.T) {
	id := bson.NewObjectID()
	for _, test := range []struct {
		name      string
		fields    bson.D
		version   int32
		wantError error
	}{
		{"missing version", bson.D{{Key: "city", Value: "Guadalajara"}}, 1, nil},
		{"explicit v1", bson.D{{Key: "city", Value: "Guadalajara"}, {Key: "schemaVersion", Value: int32(1)}}, 1, nil},
		{"int64 v1", bson.D{{Key: "city", Value: "Guadalajara"}, {Key: "schemaVersion", Value: int64(1)}}, 1, nil},
		{"v2", currentFields(int32(2)), 2, nil},
		{"int64 v2", currentFields(int64(2)), 2, nil},
		{"future", currentFields(int32(3)), 0, ErrUnsupportedVersion},
		{"large future", currentFields(int64(1) << 40), 0, ErrUnsupportedVersion},
		{"null version", currentFields(nil), 0, ErrInvalidDocument},
		{"string version", currentFields("2"), 0, ErrInvalidDocument},
		{"float version", currentFields(float64(2)), 0, ErrInvalidDocument},
		{"missing city", nil, 0, ErrInvalidDocument},
		{"wrong city type", bson.D{{Key: "city", Value: int32(42)}}, 0, ErrInvalidDocument},
		{"null city", bson.D{{Key: "city", Value: nil}}, 0, ErrInvalidDocument},
		{"long city", bson.D{{Key: "city", Value: strings.Repeat("ñ", 81)}}, 0, ErrInvalidDocument},
		{"NUL city", bson.D{{Key: "city", Value: "a\x00b"}}, 0, ErrInvalidDocument},
		{"legacy address collision", bson.D{{Key: "city", Value: "Guadalajara"}, {Key: "address", Value: nil}}, 0, ErrInvalidDocument},
		{"v2 with old city", append(currentFields(int32(2)), bson.E{Key: "city", Value: "Guadalajara"}), 0, ErrInvalidDocument},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := append(bson.D{{Key: "_id", Value: id}, {Key: "name", Value: "Ana"}}, test.fields...)
			raw, err := bson.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			profile, err := Decode(raw)
			if test.wantError != nil {
				if !errors.Is(err, test.wantError) {
					t.Fatalf("error = %v, want %v", err, test.wantError)
				}
				return
			}
			if err != nil || profile.ID != id || profile.Name != "Ana" || profile.City != "Guadalajara" || profile.StoredVersion != test.version {
				t.Fatalf("Decode = (%+v, %v)", profile, err)
			}
		})
	}
}

// Malformed BSON and invalid call parameters fail without needing a client.
func TestInvalidInputs(t *testing.T) {
	if _, err := Decode(bson.Raw{1, 2, 3}); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("bad BSON = %v", err)
	}
	if _, err := Insert(t.Context(), nil, " ", "Guadalajara"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank input = %v", err)
	}
	if _, err := Read(t.Context(), nil, bson.NilObjectID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero ID = %v", err)
	}
	for _, limit := range []int{0, -1, 101} {
		if _, err := BackfillBatch(t.Context(), nil, limit); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("limit %d = %v", limit, err)
		}
	}
	if _, err := compatibleValidator(); err != nil {
		t.Fatal(err)
	}
}

// Code-based classification keeps the driver cause through wrapping. Missing
// documents and cancellation likewise remain inspectable via errors.Is/As.
func TestErrorCauses(t *testing.T) {
	rejected := mongo.WriteException{WriteErrors: mongo.WriteErrors{{Code: 121, Message: "validation failed"}}}
	if !IsValidationFailure(fmt.Errorf("write: %w", rejected)) || IsValidationFailure(errors.New("validation failed")) || IsValidationFailure(nil) {
		t.Fatal("validation was not classified by server code")
	}
	err := queryError(t.Context(), mongo.ErrNoDocuments)
	if !errors.Is(err, ErrNotFound) || !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("missing = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = queryError(ctx, rejected)
	var cause mongo.WriteException
	if !errors.Is(err, context.Canceled) || !errors.As(err, &cause) {
		t.Fatalf("cancellation = %v", err)
	}
}

func currentFields(version any) bson.D {
	return bson.D{{Key: "schemaVersion", Value: version}, {Key: "address", Value: bson.D{{Key: "city", Value: "Guadalajara"}}}}
}
