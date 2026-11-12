package contactdoc

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// normalize bounds the embedded model and copies its reference-valued field.
// At most eight input tags keeps this teaching array intentionally small.
func normalize(input Input) (Input, error) {
	var err error
	input.Name, err = requiredText("name", input.Name, 80)
	if err != nil {
		return Input{}, err
	}
	input.Address.Street, err = requiredText("street", input.Address.Street, 120)
	if err != nil {
		return Input{}, err
	}
	input.Address.City, err = requiredText("city", input.Address.City, 80)
	if err != nil {
		return Input{}, err
	}
	if len(input.Tags) > 8 {
		return Input{}, fmt.Errorf("provide at most 8 tags: %w", ErrInvalidInput)
	}
	// Make a new slice before trimming/lowercasing. A shallow Input copy alone
	// would still share the caller's backing array. Empty tags encode as [], not null.
	tags := make([]string, 0, len(input.Tags))
	seen := make(map[string]bool)
	for _, value := range input.Tags {
		tag, err := normalizeTag(value)
		if err != nil {
			return Input{}, err
		}
		if !seen[tag] {
			tags = append(tags, tag)
			seen[tag] = true
		}
	}
	input.Tags = tags
	return input, nil
}

// Empty optional criteria mean no filter. A nonempty tag follows the same rule
// as stored tags, so querying " Go " matches the normalized value "go".
func normalizeQuery(query Query) (Query, error) {
	if query.Limit < 1 || query.Limit > 100 {
		return Query{}, fmt.Errorf("limit must be between 1 and 100: %w", ErrInvalidInput)
	}
	query.City = strings.TrimSpace(query.City)
	if query.City != "" {
		city, err := requiredText("city", query.City, 80)
		if err != nil {
			return Query{}, err
		}
		query.City = city
	}
	query.Tag = strings.TrimSpace(query.Tag)
	if query.Tag != "" {
		tag, err := normalizeTag(query.Tag)
		if err != nil {
			return Query{}, err
		}
		query.Tag = tag
	}
	return query, nil
}

func normalizeTag(value string) (string, error) {
	// Validate before case conversion; Unicode conversion can replace bad UTF-8.
	tag, err := requiredText("tag", value, 24)
	if err != nil {
		return "", err
	}
	return strings.ToLower(tag), nil
}

// Use rune counts for Unicode text; reject malformed text and embedded NUL.
func requiredText(field, value string, limit int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || utf8.RuneCountInString(value) > limit {
		return "", fmt.Errorf("%s must contain 1–%d UTF-8 runes without NUL: %w", field, limit, ErrInvalidInput)
	}
	return value, nil
}

func validateID(id bson.ObjectID) error {
	if id.IsZero() {
		return fmt.Errorf("ObjectID must not be zero: %w", ErrInvalidInput)
	}
	return nil
}
