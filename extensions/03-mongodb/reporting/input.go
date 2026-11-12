package reporting

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Bound the small teaching model and own its slice and optional reference value.
func normalize(input ProfileInput) (ProfileInput, error) {
	var err error
	input.Name, err = text("profile name", input.Name, 80)
	if err != nil {
		return ProfileInput{}, err
	}
	input.City, err = text("city", input.City, 80)
	if err != nil {
		return ProfileInput{}, err
	}
	if len(input.Tags) > 8 {
		return ProfileInput{}, fmt.Errorf("provide at most 8 tags: %w", ErrInvalidInput)
	}
	tags := make([]string, 0, len(input.Tags))
	seen := make(map[string]bool)
	for _, value := range input.Tags {
		// Validate before case conversion, which can replace malformed UTF-8.
		tag, err := text("tag", value, 24)
		if err != nil {
			return ProfileInput{}, err
		}
		tag = strings.ToLower(tag)
		if !seen[tag] {
			tags = append(tags, tag)
			seen[tag] = true
		}
	}
	input.Tags = tags
	if input.CompanyID != nil {
		if err := validID(*input.CompanyID); err != nil {
			return ProfileInput{}, err
		}
		id := *input.CompanyID
		input.CompanyID = &id
	}
	return input, nil
}

func text(field, value string, limit int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || utf8.RuneCountInString(value) > limit {
		return "", fmt.Errorf("%s requires 1–%d UTF-8 runes without NUL: %w", field, limit, ErrInvalidInput)
	}
	return value, nil
}

func query(city string, limit int) (string, error) {
	if limit < 1 || limit > 100 {
		return "", fmt.Errorf("limit must be between 1 and 100: %w", ErrInvalidInput)
	}
	city = strings.TrimSpace(city)
	if city == "" {
		return "", nil
	}
	return text("city", city, 80)
}

func validID(id bson.ObjectID) error {
	if id.IsZero() {
		return fmt.Errorf("ObjectID must not be zero: %w", ErrInvalidInput)
	}
	return nil
}
