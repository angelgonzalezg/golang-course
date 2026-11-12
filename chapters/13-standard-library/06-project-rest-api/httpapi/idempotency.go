package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// IdempotentStore adds atomic keyed creation to the existing HTTP store contract.
// Implementations must commit the contact and saved result together.
type IdempotentStore interface {
	ContactStore
	CreateOnce(context.Context, string, string, contacts.Input) (contacts.CreationResult, error)
}

// SubjectResolver reads a trusted caller identity set by the application.
// Do not implement it by trusting an arbitrary client-supplied identity header.
// An empty result means this caller cannot use keyed creation.
type SubjectResolver func(*http.Request) string

// NewHandlerWithIdempotency enables optional Idempotency-Key on POST /contacts.
// The basic chapter handler remains usable with its in-memory store. The SQL
// extension supplies durable storage and a configured single-user lab identity.
func NewHandlerWithIdempotency(store IdempotentStore, subject SubjectResolver, logger *slog.Logger) http.Handler {
	if subject == nil {
		panic("httpapi: subject resolver is required")
	}
	return newHandler(store, logger, subject)
}

// createContact selects ordinary creation or the configured durable policy.
// An invalid/unsupported key never silently becomes an ordinary create.
func (a *api) createContact(w http.ResponseWriter, r *http.Request, input contacts.Input) (contacts.Contact, bool) {
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) == 0 {
		value, err := a.store.Create(r.Context(), input)
		if err != nil {
			a.writeStoreError(w, err)
			return contacts.Contact{}, false
		}
		return value, true
	}
	if len(keys) != 1 {
		a.writeProblem(w, http.StatusBadRequest, "send exactly one Idempotency-Key")
		return contacts.Contact{}, false
	}
	if err := contacts.ValidateIdempotencyKey(keys[0]); err != nil {
		a.writeStoreError(w, err)
		return contacts.Contact{}, false
	}
	if a.subject == nil {
		a.writeProblem(w, http.StatusBadRequest, "keyed creation is not configured for this server")
		return contacts.Contact{}, false
	}
	subject := a.subject(r)
	if subject == "" {
		a.writeProblem(w, http.StatusForbidden, "caller cannot use keyed creation")
		return contacts.Contact{}, false
	}
	// An invalid trusted identity is an application configuration error. Do not
	// present its value as a client validation error or bind a key under it.
	if err := contacts.ValidateRequestIdentity(subject, keys[0]); err != nil {
		a.logger.Error("invalid idempotency subject configuration")
		a.writeProblem(w, http.StatusInternalServerError, "request could not be completed")
		return contacts.Contact{}, false
	}
	result, err := a.store.(IdempotentStore).CreateOnce(r.Context(), subject, keys[0], input)
	if err != nil {
		a.writeStoreError(w, err)
		return contacts.Contact{}, false
	}
	w.Header().Set("Idempotency-Replayed", strconv.FormatBool(result.Replayed))
	return result.Contact, true
}
