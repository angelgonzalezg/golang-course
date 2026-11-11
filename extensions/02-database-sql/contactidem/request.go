package contactidem

import (
	"crypto/sha256"
	"encoding/binary"
	json "encoding/json/v2"
	"fmt"

	"example.com/course/sql-lab/contactsql"
)

// prepare checks the typed request before touching PostgreSQL. Business fields
// use the earlier lesson's rules; caller/key identifiers use this lab's small
// ASCII contract. Keep supplied pointers stable until CreateOnce returns.
func prepare(subject, key string, input contactsql.Input) (contactsql.Input, [sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	if !validIdentifier(subject) || !validIdentifier(key) {
		return contactsql.Input{}, zero, fmt.Errorf("subject and key require 1–64 ASCII letters, digits, _ or -: %w", contactsql.ErrInvalidInput)
	}
	normalized, err := contactsql.NormalizeInput(input)
	if err != nil {
		return contactsql.Input{}, zero, err
	}
	// Hash the original field values, before trimming. Changing whitespace is
	// a different request; nil and a pointer to empty email also stay distinct.
	// This fixed typed encoding is our v1 identity format, not a claim that
	// arbitrary JSON documents have one universal canonical representation.
	body, err := json.Marshal(struct {
		Format int     `json:"format"`
		Name   string  `json:"name"`
		Email  *string `json:"email"`
	}{1, input.Name, input.Email})
	if err != nil {
		return contactsql.Input{}, zero, fmt.Errorf("encode request identity: %w", err)
	}
	return normalized, sha256.Sum256(body), nil
}

func validIdentifier(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-') {
			return false
		}
	}
	return true
}

// lockID must be computed the same way by every participating writer. The
// operation prefix scopes these keys to creation; NUL separates validated IDs.
// A rare 64-bit hash collision only makes unrelated keys wait for each other.
// The exact (subject, key) database primary key still determines request identity.
func lockID(subject, key string) int64 {
	hash := sha256.Sum256([]byte("sql-idem/create-contact/v1\x00" + subject + "\x00" + key))
	return int64(binary.BigEndian.Uint64(hash[:8]))
}
