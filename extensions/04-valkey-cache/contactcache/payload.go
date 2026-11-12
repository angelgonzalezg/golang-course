package contactcache

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"example.com/course/cache-lab/cache"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// Keep every field used by REST/gRPC, including empty optional fields. The
// fundamentals' two-field Contact is intentionally a separate teaching DTO.
type payload struct {
	SchemaVersion int    `json:"schema_version"`
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
}

func encode(contact contacts.Contact, expectedID int64) (string, error) {
	if err := validateContact(contact, expectedID); err != nil {
		return "", err
	}
	data, err := json.Marshal(payload{1, contact.ID, contact.Name, contact.Email, contact.Phone})
	if err != nil {
		return "", fmt.Errorf("encode contact cache: %w", err)
	}
	return string(data), nil
}

// A JSON value under the wrong ID must never become another contact's hit.
// Typed int64 decoding also preserves IDs larger than JavaScript's 2^53 limit.
func decode(value string, expectedID int64) (contacts.Contact, error) {
	if !utf8.ValidString(value) {
		return contacts.Contact{}, fmt.Errorf("cache JSON is not UTF-8: %w", cache.ErrInvalidPayload)
	}
	var data payload
	if err := json.Unmarshal([]byte(value), &data); err != nil {
		return contacts.Contact{}, fmt.Errorf("%w: %w", cache.ErrInvalidPayload, err)
	}
	if data.SchemaVersion != 1 {
		return contacts.Contact{}, fmt.Errorf("unsupported contact cache version %d: %w", data.SchemaVersion, cache.ErrInvalidPayload)
	}
	contact := contacts.Contact{ID: data.ID, Name: data.Name, Email: data.Email, Phone: data.Phone}
	if err := validateContact(contact, expectedID); err != nil {
		return contacts.Contact{}, err
	}
	return contact, nil
}

// Cached/source data must already satisfy the shared normalized contact rules.
// Do not silently change a cached value: that would change its content version.
func validateContact(contact contacts.Contact, expectedID int64) error {
	if contact.ID != expectedID || contact.ID <= 0 {
		return fmt.Errorf("contact cache ID does not match its key: %w", cache.ErrInvalidPayload)
	}
	input := contacts.Input{Name: contact.Name, Email: contact.Email, Phone: contact.Phone}
	normalized, err := contacts.NormalizeInput(input)
	if err != nil {
		return fmt.Errorf("%w: %w", cache.ErrInvalidPayload, err)
	}
	if normalized != input {
		return fmt.Errorf("contact cache fields are not normalized: %w", cache.ErrInvalidPayload)
	}
	return nil
}
