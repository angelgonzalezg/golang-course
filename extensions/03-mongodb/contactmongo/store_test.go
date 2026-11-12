package contactmongo

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Check Unicode boundaries without a server. Optional email text preserves
// presence rather than pretending to validate complete RFC email addresses.
func TestInputBoundaries(t *testing.T) {
	for _, test := range []struct {
		name    string
		input   Input
		want    string
		invalid bool
	}{
		{name: "trim Unicode whitespace", input: Input{Name: "\u00a0Ana\t"}, want: "Ana"},
		{name: "Unicode boundary", input: Input{Name: strings.Repeat("ñ", 80)}, want: strings.Repeat("ñ", 80)},
		{name: "blank", input: Input{Name: " \t"}, invalid: true},
		{name: "long name", input: Input{Name: strings.Repeat("a", 81)}, invalid: true},
		{name: "malformed name", input: Input{Name: "a\xff"}, invalid: true},
		{name: "NUL name", input: Input{Name: "A\x00B"}, invalid: true},
		{name: "empty email", input: Input{Name: "Ana", Email: text("")}, want: "Ana"},
		{name: "email boundary", input: Input{Name: "Ana", Email: text(strings.Repeat("é", 254))}, want: "Ana"},
		{name: "long email", input: Input{Name: "Ana", Email: text(strings.Repeat("a", 255))}, invalid: true},
		{name: "malformed email", input: Input{Name: "Ana", Email: text("a\xff")}, invalid: true},
		{name: "NUL email", input: Input{Name: "Ana", Email: text("a\x00b")}, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalize(test.input)
			if test.invalid {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil || got.Name != test.want {
				t.Fatalf("normalize = (%+v, %v)", got, err)
			}
		})
	}
}

// Invalid operations fail before using the collection. The zero Store has no
// client, so a validation regression cannot accidentally use a real database.
func TestInvalidOperations(t *testing.T) {
	store := &Store{}
	id := bson.NewObjectID()
	for _, test := range []struct {
		name string
		run  func() error
	}{
		{"create", func() error {
			_, err := store.Create(t.Context(), Input{})
			return err
		}},
		{"get zero ID", func() error {
			_, err := store.Get(t.Context(), bson.NilObjectID)
			return err
		}},
		{"update zero ID", func() error {
			_, err := store.UpdateEmail(t.Context(), bson.NilObjectID, nil)
			return err
		}},
		{"update long email", func() error {
			_, err := store.UpdateEmail(t.Context(), id, text(strings.Repeat("x", 255)))
			return err
		}},
		{"delete zero ID", func() error {
			return store.Delete(t.Context(), bson.NilObjectID)
		}},
		{"list zero", func() error {
			_, err := store.List(t.Context(), 0)
			return err
		}},
		{"list too large", func() error {
			_, err := store.List(t.Context(), 101)
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

// Domain errors retain MongoDB/context causes without parsing error messages.
func TestQueryErrorPreservesCauses(t *testing.T) {
	err := queryError(t.Context(), mongo.ErrNoDocuments)
	if !errors.Is(err, ErrNotFound) || !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("missing document error = %v", err)
	}
	duplicate := mongo.WriteException{WriteErrors: mongo.WriteErrors{{Code: 11000, Message: "duplicate"}}}
	err = queryError(t.Context(), duplicate)
	var cause mongo.WriteException
	if !errors.Is(err, ErrDuplicate) || !errors.As(err, &cause) || !mongo.IsDuplicateKeyError(cause) {
		t.Fatalf("duplicate error = %v", err)
	}
	other := errors.New("another failure")
	if got := queryError(t.Context(), other); got != other {
		t.Fatalf("unrelated error = %v", got)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = queryError(ctx, duplicate)
	if !errors.Is(err, context.Canceled) || !errors.As(err, &cause) {
		t.Fatalf("canceled error = %v", err)
	}
}

// Empty configuration is rejected instead of silently using localhost defaults.
func TestOpenRequiresURI(t *testing.T) {
	client, err := Open(t.Context(), " \t")
	if err == nil || client != nil {
		t.Fatalf("Open blank = (%v, %v)", client, err)
	}
}

func text(value string) *string { return &value }
