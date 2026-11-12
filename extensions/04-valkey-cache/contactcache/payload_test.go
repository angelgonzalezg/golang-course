package contactcache

import (
	"errors"
	"testing"

	"example.com/course/cache-lab/cache"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// The full contact survives JSON with exact int64 IDs, Unicode, optional empty
// fields, and embedded NUL allowed by the shared model. Its content version stays equal.
func TestPayloadRoundTrip(t *testing.T) {
	want := contacts.Contact{ID: 9007199254740993, Name: "Ana\x00 García", Email: "ana@example.com", Phone: ""}
	value, err := encode(want, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decode(value, want.ID)
	if err != nil || got != want || contacts.Version(got) != contacts.Version(want) {
		t.Fatalf("round trip=(%+v, %v), want %+v", got, err, want)
	}
}

// A payload can be valid JSON and still be unusable: wrong key ID, unsupported
// version, invalid field types, or unnormalized contact fields are not cache misses.
func TestInvalidPayloads(t *testing.T) {
	for _, value := range []string{
		"{", "\xff", `null`,
		`{"schema_version":2,"id":1,"name":"Ana"}`,
		`{"schema_version":1,"id":2,"name":"Ana"}`,
		`{"schema_version":1,"id":1,"name":" "}`,
		`{"schema_version":1,"id":1,"name":" Ana "}`,
		`{"schema_version":1,"id":"1","name":"Ana"}`,
	} {
		if _, err := decode(value, 1); !errors.Is(err, cache.ErrInvalidPayload) || errors.Is(err, cache.ErrMiss) {
			t.Errorf("decode %q = %v", value, err)
		}
	}
}
