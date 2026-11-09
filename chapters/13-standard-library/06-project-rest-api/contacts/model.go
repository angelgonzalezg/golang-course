// Package contacts defines contact rules independently of HTTP or RPC.
package contacts

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

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

var (
	ErrInvalidInput = errors.New("invalid contact input")
	ErrNotFound     = errors.New("contact not found")
	ErrNameTaken    = errors.New("contact name already exists")
	ErrIDExhausted  = errors.New("contact IDs exhausted")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Message) }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

func normalize(input Input) (Input, error) {
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

func validateID(id int64) error {
	if id <= 0 {
		return &ValidationError{Field: "id", Message: "must be positive"}
	}
	return nil
}
