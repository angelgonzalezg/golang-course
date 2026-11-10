package contacts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
)

// Version identifies the contact's current values. It is opaque to callers.
// Equal content has an equal version, including after a no-op update.
// It describes content, rather than counting writes or recording their times.
func Version(contact Contact) string {
	hash := sha256.New()
	for _, value := range []string{strconv.FormatInt(contact.ID, 10), contact.Name, contact.Email, contact.Phone} {
		// Length prefixes make boundaries unambiguous even if text contains colons.
		fmt.Fprintf(hash, "%d:", len(value))
		hash.Write([]byte(value))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// VersionMatch matches any existing contact or one of the supplied versions.
// An empty list matches nothing. The store does not depend on HTTP header syntax.
type VersionMatch struct {
	Any      bool
	Versions []string
}

// Conditions combines optional version requirements. Nil fields impose no
// restriction; a non-nil empty match list still matches nothing. The zero value
// requests an unconditional operation. When both are supplied, Match is checked first.
type Conditions struct {
	Match     *VersionMatch // If supplied, the current contact must match.
	NoneMatch *VersionMatch // If supplied, a matching current contact is rejected.
}

func (m *VersionMatch) matches(contact Contact) bool {
	if m.Any {
		return true
	}
	version := Version(contact)
	for _, candidate := range m.Versions {
		if candidate == version {
			return true
		}
	}
	return false
}

// A zero Conditions value allows an unconditional operation. Call this while
// holding the same lock that protects the mutation, after normal validation.
func (c Conditions) check(contact Contact) error {
	if c.Match != nil && !c.Match.matches(contact) {
		return ErrPreconditionFailed
	}
	if c.NoneMatch != nil && c.NoneMatch.matches(contact) {
		return ErrPreconditionFailed
	}
	return nil
}
