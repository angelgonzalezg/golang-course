package contactmongo

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// normalize keeps the same small text boundaries as the SQL fundamentals model.
// Application validation still matters when documents have a flexible schema.
func normalize(input Input) (Input, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || !validText(input.Name, 80) {
		return Input{}, fmt.Errorf("name must contain 1–80 UTF-8 runes without NUL: %w", ErrInvalidInput)
	}
	if err := validateEmail(input.Email); err != nil {
		return Input{}, err
	}
	return input, nil
}

func validateEmail(email *string) error {
	if email != nil && !validText(*email, 254) {
		return fmt.Errorf("email must contain at most 254 UTF-8 runes without NUL: %w", ErrInvalidInput)
	}
	return nil
}

// Count runes rather than bytes so the boundaries work with Unicode names.
func validText(value string, limit int) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= limit
}

func validateID(id bson.ObjectID) error {
	if id.IsZero() {
		return fmt.Errorf("ObjectID must not be zero: %w", ErrInvalidInput)
	}
	return nil
}
