package reporting

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Both the reference and tags have independent returned storage. Copying the
// input struct alone would leave the pointer value and backing array shared.
func TestInputOwnership(t *testing.T) {
	id := bson.NewObjectID()
	originalID := id
	input := ProfileInput{Name: " Ana ", City: " Guadalajara ", Tags: []string{" Go ", "go", " Backend "}, CompanyID: &id}
	got, err := normalize(input)
	if err != nil || got.Name != "Ana" || got.City != "Guadalajara" || !slices.Equal(got.Tags, []string{"go", "backend"}) || got.CompanyID == nil || *got.CompanyID != id {
		t.Fatalf("normalize = (%+v, %v)", got, err)
	}
	got.Tags[0], *got.CompanyID = "local edit", bson.NewObjectID()
	if input.Tags[0] != " Go " || *input.CompanyID != originalID {
		t.Fatal("normalized result aliases input")
	}
}

// Check model and query boundaries without a server, including malformed UTF-8
// before case conversion and explicit zero references instead of missing ones.
func TestInputValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*ProfileInput)
	}{
		{"blank name", func(in *ProfileInput) { in.Name = " " }},
		{"long city", func(in *ProfileInput) { in.City = strings.Repeat("ñ", 81) }},
		{"NUL city", func(in *ProfileInput) { in.City = "a\x00b" }},
		{"invalid tag", func(in *ProfileInput) { in.Tags = []string{"a\xff"} }},
		{"blank tag", func(in *ProfileInput) { in.Tags = []string{" "} }},
		{"long tag", func(in *ProfileInput) { in.Tags = []string{strings.Repeat("é", 25)} }},
		{"many tags", func(in *ProfileInput) { in.Tags = []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} }},
		{"zero reference", func(in *ProfileInput) { zero := bson.NilObjectID; in.CompanyID = &zero }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := ProfileInput{Name: "Ana", City: "Guadalajara"}
			test.change(&input)
			if _, err := normalize(input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if _, err := normalize(ProfileInput{Name: strings.Repeat("ñ", 80), City: "Puebla", Tags: []string{strings.Repeat("é", 24)}}); err != nil {
		t.Fatal(err)
	}
	if city, err := query(" Puebla ", 100); err != nil || city != "Puebla" {
		t.Fatalf("query = (%q, %v)", city, err)
	}
	if city, err := query(" ", 1); err != nil || city != "" {
		t.Fatalf("blank criterion = (%q, %v)", city, err)
	}
}

// Invalid requests fail before using a collection handle. The intentionally
// empty Store ensures these calls cannot accidentally use a database.
func TestValidationBeforeDatabase(t *testing.T) {
	store := &Store{}
	if _, err := store.CreateCompany(t.Context(), " "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("company = %v", err)
	}
	if _, err := store.CreateProfile(t.Context(), ProfileInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("profile = %v", err)
	}
	if _, err := store.Directory(t.Context(), "", 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("directory = %v", err)
	}
	if _, err := store.CityTotals(t.Context(), 101); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("city report = %v", err)
	}
	if _, err := store.TagTotals(t.Context(), "a\xff", 1); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("tag report = %v", err)
	}
	if _, err := store.ExplainCity(t.Context(), " "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("explain = %v", err)
	}
	if _, err := store.RenameCompany(t.Context(), bson.NilObjectID, "Name"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("rename = %v", err)
	}
	if err := store.DeleteCompany(t.Context(), bson.NilObjectID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("delete = %v", err)
	}
}

// Missing documents and canceled requests preserve their original driver causes.
func TestErrorCauses(t *testing.T) {
	err := queryError(t.Context(), mongo.ErrNoDocuments)
	if !errors.Is(err, ErrNotFound) || !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("missing = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = queryError(ctx, mongo.ErrNoDocuments)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("canceled = %v", err)
	}
}
