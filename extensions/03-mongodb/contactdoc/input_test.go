package contactdoc

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Trim owned scalar values, normalize/deduplicate tags, and prove that modifying
// the normalized slice does not change the input's original backing array.
func TestNormalizationOwnsTags(t *testing.T) {
	input := Input{Name: " Ana ", Address: Address{Street: " Rio 1 ", City: " Guadalajara "},
		Tags: []string{" Go ", "go", " Backend "}}
	got, err := normalize(input)
	if err != nil {
		t.Fatal(err)
	}
	want := Input{Name: "Ana", Address: Address{Street: "Rio 1", City: "Guadalajara"}, Tags: []string{"go", "backend"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalize = %+v, want %+v", got, want)
	}
	got.Tags[0] = "local edit"
	if input.Name != " Ana " || input.Address.City != " Guadalajara " || input.Tags[0] != " Go " || input.Tags[1] != "go" {
		t.Fatalf("normalization changed caller input: %+v", input)
	}
}

// These domain boundaries run without MongoDB. Invalid UTF-8 must be rejected
// before lowercasing can silently turn it into replacement characters.
func TestInputValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Input)
	}{
		{"blank name", func(in *Input) { in.Name = " \t" }},
		{"long name", func(in *Input) { in.Name = strings.Repeat("ñ", 81) }},
		{"invalid name", func(in *Input) { in.Name = "a\xff" }},
		{"NUL name", func(in *Input) { in.Name = "a\x00b" }},
		{"blank street", func(in *Input) { in.Address.Street = "" }},
		{"long street", func(in *Input) { in.Address.Street = strings.Repeat("é", 121) }},
		{"blank city", func(in *Input) { in.Address.City = " \n" }},
		{"long city", func(in *Input) { in.Address.City = strings.Repeat("é", 81) }},
		{"invalid city", func(in *Input) { in.Address.City = "a\xff" }},
		{"too many tags", func(in *Input) { in.Tags = []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} }},
		{"blank tag", func(in *Input) { in.Tags = []string{" "} }},
		{"long tag", func(in *Input) { in.Tags = []string{strings.Repeat("é", 25)} }},
		{"invalid tag", func(in *Input) { in.Tags = []string{"a\xff"} }},
		{"NUL tag", func(in *Input) { in.Tags = []string{"a\x00b"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := validInput()
			test.change(&input)
			if _, err := normalize(input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
		})
	}
	t.Run("Unicode maximums and eight tags", func(t *testing.T) {
		input := Input{Name: strings.Repeat("ñ", 80),
			Address: Address{Street: strings.Repeat("é", 120), City: strings.Repeat("ñ", 80)},
			Tags:    []string{strings.Repeat("é", 24), "b", "c", "d", "e", "f", "g", "h"}}
		if _, err := normalize(input); err != nil {
			t.Fatal(err)
		}
	})
}

// Optional query criteria share storage normalization; blank criteria select
// all profiles, while an explicit bounded limit is always required.
func TestQueryValidation(t *testing.T) {
	got, err := normalizeQuery(Query{City: " Guadalajara ", Tag: " Go ", Limit: 2})
	if err != nil || got != (Query{City: "Guadalajara", Tag: "go", Limit: 2}) {
		t.Fatalf("normalized query = (%+v, %v)", got, err)
	}
	for _, limit := range []int{1, 100} {
		if _, err := normalizeQuery(Query{City: " ", Tag: "\t", Limit: limit}); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []Query{
		{}, {Limit: -1}, {Limit: 101},
		{City: strings.Repeat("x", 81), Limit: 1}, {City: "a\x00b", Limit: 1},
		{Tag: strings.Repeat("x", 25), Limit: 1}, {Tag: "a\xff", Limit: 1},
	} {
		if _, err := normalizeQuery(query); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("query %+v: %v", query, err)
		}
	}
}

// Zero identifiers and invalid input fail before using a collection. The Store
// deliberately has no client, so these cases cannot reach a real database.
func TestValidationBeforeDatabase(t *testing.T) {
	store := &Store{}
	if _, err := store.Create(t.Context(), Input{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("create invalid = %v", err)
	}
	if _, err := store.Search(t.Context(), Query{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("search invalid = %v", err)
	}
	if _, err := store.Get(t.Context(), bson.NilObjectID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("get zero = %v", err)
	}
	if _, err := store.UpdateCity(t.Context(), bson.NilObjectID, "Puebla"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("update zero = %v", err)
	}
	if _, err := store.UpdateCity(t.Context(), bson.NewObjectID(), " "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("update blank city = %v", err)
	}
	if err := store.Delete(t.Context(), bson.NilObjectID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("delete zero = %v", err)
	}
}

// Keep both domain and driver errors inspectable, including a canceled caller.
func TestErrorCauses(t *testing.T) {
	err := queryError(t.Context(), mongo.ErrNoDocuments)
	if !errors.Is(err, ErrNotFound) || !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("missing error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = queryError(ctx, mongo.ErrNoDocuments)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("canceled error = %v", err)
	}
}

func validInput() Input {
	return Input{Name: "Ana", Address: Address{Street: "Rio 1", City: "Guadalajara"}, Tags: []string{"go"}}
}
