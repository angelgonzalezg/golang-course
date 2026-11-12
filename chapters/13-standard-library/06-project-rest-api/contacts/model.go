// Package contacts defines contact rules independently of HTTP or RPC.
package contacts

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Contact is a stored value shared by both transports. Returned copies can be
// edited by the caller without changing the store; JSON/protobuf types stay in adapters.
type Contact struct {
	ID    int64
	Name  string
	Email string
	Phone string
}

// Input contains every editable field. Replacing a contact replaces all three.
type Input struct {
	Name  string
	Email string
	Phone string
}

// Changes selects the fields to update. nil keeps the stored field; a pointer
// supplies its new value. A pointer to "" clears Email or Phone.
type Changes struct {
	Name  *string
	Email *string
	Phone *string
}

var (
	ErrInvalidInput       = errors.New("invalid contact input")
	ErrNotFound           = errors.New("contact not found")
	ErrNameTaken          = errors.New("contact name already exists")
	ErrIDExhausted        = errors.New("contact IDs exhausted")
	ErrPreconditionFailed = errors.New("contact version precondition failed")
)

// ValidationError identifies a rejected field. Use errors.As to read the field
// and errors.Is(err, ErrInvalidInput) to recognize any input validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Message) }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

// NormalizeInput applies the contact rules shared by memory and database stores.
// It returns normalized values without changing the caller's Input.
func NormalizeInput(input Input) (Input, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.TrimSpace(input.Email)
	input.Phone = strings.TrimSpace(input.Phone)
	for _, field := range []struct {
		name  string
		value string
		limit int
	}{
		{name: "name", value: input.Name, limit: 80},
		{name: "email", value: input.Email, limit: 254},
		{name: "phone", value: input.Phone, limit: 40},
	} {
		if !utf8.ValidString(field.value) {
			return Input{}, &ValidationError{Field: field.name, Message: "must be valid UTF-8"}
		}
		if utf8.RuneCountInString(field.value) > field.limit {
			return Input{}, &ValidationError{Field: field.name, Message: fmt.Sprintf("must contain at most %d runes", field.limit)}
		}
	}
	if input.Name == "" {
		return Input{}, &ValidationError{Field: "name", Message: "must not be blank"}
	}
	return input, nil
}

// ValidateID rejects invalid identifiers before a storage operation starts.
func ValidateID(id int64) error {
	if id <= 0 {
		return &ValidationError{Field: "id", Message: "must be positive"}
	}
	return nil
}
