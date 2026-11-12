package contacts

import "errors"

// ErrKeyReuse means a committed operation already binds this caller/key to
// different input. Retry the original request to recover its original result.
var ErrKeyReuse = errors.New("idempotency key is bound to another request")

// CreationResult holds the original successful creation snapshot. Replayed
// describes this invocation and is not part of the stored contact itself.
type CreationResult struct {
	Contact  Contact
	Replayed bool
}

// ValidateRequestIdentity defines this lab's small caller/key namespace. The
// caller identity must be trusted; validating its shape does not authenticate it.
func ValidateRequestIdentity(subject, key string) error {
	if err := validateIdentifier("subject", subject); err != nil {
		return err
	}
	return ValidateIdempotencyKey(key)
}

// ValidateIdempotencyKey also lets a transport reject malformed headers before
// resolving caller identity or sending any database operation.
func ValidateIdempotencyKey(key string) error { return validateIdentifier("idempotency_key", key) }

func validateIdentifier(field, value string) error {
	valid := len(value) >= 1 && len(value) <= 64
	for _, char := range value {
		valid = valid && (char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-')
	}
	if !valid {
		return &ValidationError{Field: field, Message: "must contain 1–64 ASCII letters, digits, _ or -"}
	}
	return nil
}
