package contactmongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// BSON encoding is real without a server. Inspect the wire document to show
// that nil/omitempty, an empty string, and explicit null have different shapes.
func TestBSONPresence(t *testing.T) {
	for _, test := range []struct {
		name  string
		email *string
	}{
		{"missing", nil}, {"empty string", text("")}, {"nonempty", text("ana@example.com")},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := Contact{ID: bson.NewObjectID(), Name: "Ana", Email: test.email}
			data, err := bson.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			value, err := bson.Raw(data).LookupErr("email")
			if test.email == nil {
				if err == nil {
					t.Fatal("nil email was not omitted")
				}
			} else if err != nil || value.Type != bson.TypeString || value.StringValue() != *test.email {
				t.Fatalf("stored email = %v, %v", value, err)
			}
			var decoded Contact
			if err := bson.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.ID != original.ID || decoded.Name != original.Name ||
				(decoded.Email == nil) != (test.email == nil) ||
				(test.email != nil && *decoded.Email != *test.email) {
				t.Fatalf("round trip = %+v", decoded)
			}
		})
	}
	t.Run("explicit null also decodes to nil", func(t *testing.T) {
		data, err := bson.Marshal(bson.D{{Key: "name", Value: "Ana"}, {Key: "email", Value: nil}})
		if err != nil {
			t.Fatal(err)
		}
		if bson.Raw(data).Lookup("email").Type != bson.TypeNull {
			t.Fatal("expected BSON null")
		}
		var decoded Contact
		if err := bson.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Email != nil {
			t.Fatalf("null decoded as %v", decoded.Email)
		}
	})
}
