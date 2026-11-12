package cache

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// Typed JSON preserves integer identifiers beyond float64's exact range and
// retains Unicode text. Additive fields remain compatible within payload v1.
func TestContactRoundTrip(t *testing.T) {
	want := Contact{ID: 9007199254740993, Name: "Ángel"}
	encoded, err := EncodeContact(Contact{ID: want.ID, Name: " Ángel "})
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeContact(encoded)
	if err != nil || got != want {
		t.Fatalf("round trip = (%+v, %v)", got, err)
	}
	got, err = DecodeContact(`{"schema_version":1,"contact":{"id":1,"name":"Ana","extra":true},"extra":true}`)
	if err != nil || got != (Contact{ID: 1, Name: "Ana"}) {
		t.Fatalf("additive fields = (%+v, %v)", got, err)
	}
}

// Bad cached content is a payload error, not a key miss. Syntax causes remain
// available through wrapping; unsupported versions are never silently guessed.
func TestInvalidContactPayloads(t *testing.T) {
	for _, input := range []string{"", "{", "null", `{"schema_version":2}`, `{"schema_version":1}`, `{"contact":{"id":1,"name":"Ana"}}`,
		`{"schema_version":1,"contact":{"id":0,"name":"Ana"}}`, `{"schema_version":1,"contact":{"id":"1","name":"Ana"}}`, "\xff"} {
		_, err := DecodeContact(input)
		if !errors.Is(err, ErrInvalidPayload) || errors.Is(err, ErrMiss) {
			t.Fatalf("invalid payload error = %v", err)
		}
	}
	_, err := DecodeContact("{")
	var syntax *json.SyntaxError
	if !errors.As(err, &syntax) {
		t.Fatalf("JSON cause was lost: %v", err)
	}
	for _, input := range []Contact{{ID: 0, Name: "Ana"}, {ID: 1, Name: " "}, {ID: 1, Name: "a\xff"}, {ID: 1, Name: strings.Repeat("ñ", 81)}} {
		if _, err := EncodeContact(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("bad contact accepted: %v", err)
		}
	}
}
