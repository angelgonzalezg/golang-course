package contactdoc

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A nested Go struct becomes a BSON document; a normalized slice becomes an
// array, including [] when no tags are supplied. Round trips retain the model.
func TestEmbeddedBSON(t *testing.T) {
	for _, tags := range [][]string{nil, {"go", "backend"}} {
		input := validInput()
		input.Tags = tags
		normalized, err := normalize(input)
		if err != nil {
			t.Fatal(err)
		}
		original := Profile{ID: bson.NewObjectID(), Name: normalized.Name, Address: normalized.Address, Tags: normalized.Tags}
		encoded, err := bson.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		raw := bson.Raw(encoded)
		if raw.Lookup("address").Type != bson.TypeEmbeddedDocument || raw.Lookup("address", "city").StringValue() != "Guadalajara" {
			t.Fatalf("embedded address = %v", raw.Lookup("address"))
		}
		if raw.Lookup("tags").Type != bson.TypeArray {
			t.Fatalf("tags did not encode as an array: %v", raw.Lookup("tags"))
		}
		var decoded Profile
		if err := bson.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded, original) {
			t.Fatalf("round trip = %+v, want %+v", decoded, original)
		}
	}
}
