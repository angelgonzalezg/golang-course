package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Contact is the small fundamentals model. The contactcache package uses the
// complete shared SQL/API model; cache lifetime is separate from record lifetime.
type Contact struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type contactPayload struct {
	SchemaVersion int     `json:"schema_version"`
	Contact       Contact `json:"contact"`
}

// EncodeContact normalizes a typed value and versions its JSON representation.
// The envelope describes a cache format, not a database migration version.
func EncodeContact(contact Contact) (string, error) {
	contact, err := normalizeContact(contact)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(contactPayload{SchemaVersion: 1, Contact: contact})
	if err != nil {
		return "", fmt.Errorf("encode cached contact: %w", err)
	}
	return string(data), nil
}

// DecodeContact rejects malformed/future payloads instead of calling them a
// missing key. Unknown additive fields are allowed by encoding/json's decoder.
func DecodeContact(value string) (Contact, error) {
	if !utf8.ValidString(value) {
		return Contact{}, fmt.Errorf("cached JSON is not UTF-8: %w", ErrInvalidPayload)
	}
	var payload contactPayload
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		return Contact{}, fmt.Errorf("%w: %w", ErrInvalidPayload, err)
	}
	if payload.SchemaVersion != 1 {
		return Contact{}, fmt.Errorf("unsupported cache payload version %d: %w", payload.SchemaVersion, ErrInvalidPayload)
	}
	contact, err := normalizeContact(payload.Contact)
	if err != nil {
		return Contact{}, fmt.Errorf("%w: %w", ErrInvalidPayload, err)
	}
	return contact, nil
}

// SetContact stores the serialized value through the same atomic SET/PX path.
func (s *Store) SetContact(ctx context.Context, key string, contact Contact, ttl time.Duration) error {
	value, err := EncodeContact(contact)
	if err != nil {
		return err
	}
	return s.Set(ctx, key, value, ttl)
}

// GetContact preserves ErrMiss separately from a malformed stored representation.
func (s *Store) GetContact(ctx context.Context, key string) (Contact, error) {
	value, err := s.Get(ctx, key)
	if err != nil {
		return Contact{}, err
	}
	return DecodeContact(value)
}

func normalizeContact(contact Contact) (Contact, error) {
	contact.Name = strings.TrimSpace(contact.Name)
	if contact.ID <= 0 || contact.Name == "" || !utf8.ValidString(contact.Name) || strings.ContainsRune(contact.Name, 0) || utf8.RuneCountInString(contact.Name) > 80 {
		return Contact{}, fmt.Errorf("contact needs a positive ID and 1–80 UTF-8 name runes without NUL: %w", ErrInvalidInput)
	}
	return contact, nil
}
