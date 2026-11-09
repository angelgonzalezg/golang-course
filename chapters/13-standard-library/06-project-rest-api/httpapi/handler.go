// Package httpapi translates HTTP requests and responses to contact operations.
package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// ContactStore is the behavior this HTTP adapter needs. Another adapter, such
// as gRPC, can use the same contact rules without importing HTTP types.
type ContactStore interface {
	Create(context.Context, contacts.Input) (contacts.Contact, error)
	Get(context.Context, int64) (contacts.Contact, error)
	List(context.Context) ([]contacts.Contact, error)
	Replace(context.Context, int64, contacts.Input) (contacts.Contact, error)
	Delete(context.Context, int64) error
}

type Links struct {
	Self       string `json:"self"`
	Collection string `json:"collection,omitempty"`
}

type ContactResource struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
	Links Links  `json:"links"`
}

type Collection struct {
	Contacts []ContactResource `json:"contacts"`
	Links    Links             `json:"links"`
}

type api struct {
	store  ContactStore
	logger *slog.Logger
}

// NewHandler requires a usable store. A nil logger selects a local discard
// logger, so the package never changes the process-global logging settings.
func NewHandler(store ContactStore, logger *slog.Logger) http.Handler {
	if store == nil {
		panic("httpapi: store is required")
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	a := &api{store: store, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.index)
	mux.HandleFunc("/{$}", a.methodNotAllowed("GET, HEAD"))
	mux.HandleFunc("GET /contacts", a.list)
	mux.HandleFunc("POST /contacts", a.create)
	mux.HandleFunc("/contacts", a.methodNotAllowed("GET, HEAD, POST"))
	mux.HandleFunc("GET /contacts/{id}", a.get)
	mux.HandleFunc("PUT /contacts/{id}", a.replace)
	mux.HandleFunc("DELETE /contacts/{id}", a.delete)
	mux.HandleFunc("/contacts/{id}", a.methodNotAllowed("DELETE, GET, HEAD, PUT"))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		a.writeProblem(w, http.StatusNotFound, "resource not found")
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This first version makes responses explicitly non-cacheable. A later
		// exercise can add validators and conditional requests as a separate contract.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}

func resource(contact contacts.Contact) ContactResource {
	return ContactResource{
		ID: contact.ID, Name: contact.Name, Email: contact.Email, Phone: contact.Phone,
		Links: Links{Self: "/contacts/" + strconv.FormatInt(contact.ID, 10), Collection: "/contacts"},
	}
}

func (a *api) index(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, http.StatusOK, map[string]any{"links": map[string]string{"contacts": "/contacts"}})
}

func (a *api) list(w http.ResponseWriter, r *http.Request) {
	values, err := a.store.List(r.Context())
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	result := Collection{Contacts: make([]ContactResource, 0, len(values)), Links: Links{Self: "/contacts"}}
	for _, value := range values {
		result.Contacts = append(result.Contacts, resource(value))
	}
	a.writeJSON(w, http.StatusOK, result)
}

func (a *api) get(w http.ResponseWriter, r *http.Request) {
	id, ok := a.readID(w, r)
	if !ok {
		return
	}
	value, err := a.store.Get(r.Context(), id)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, resource(value))
}

func (a *api) create(w http.ResponseWriter, r *http.Request) {
	input, ok := a.readInput(w, r)
	if !ok {
		return
	}
	value, err := a.store.Create(r.Context(), input)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	result := resource(value)
	// A relative Location avoids deriving an absolute URL from an untrusted Host.
	w.Header().Set("Location", result.Links.Self)
	a.writeJSON(w, http.StatusCreated, result)
}

func (a *api) replace(w http.ResponseWriter, r *http.Request) {
	id, ok := a.readID(w, r)
	if !ok {
		return
	}
	input, ok := a.readInput(w, r)
	if !ok {
		return
	}
	value, err := a.store.Replace(r.Context(), id, input)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, resource(value))
}

func (a *api) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := a.readID(w, r)
	if !ok {
		return
	}
	if err := a.store.Delete(r.Context(), id); err != nil {
		a.writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent) // 204 carries no representation or JSON body.
}

func (a *api) readID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	text := r.PathValue("id")
	id, err := strconv.ParseInt(text, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != text {
		a.writeProblem(w, http.StatusBadRequest, "id must be a positive decimal integer without leading zeros")
		return 0, false
	}
	return id, true
}

func (a *api) methodNotAllowed(allow string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allow)
		a.writeProblem(w, http.StatusMethodNotAllowed, "method is not supported for this resource")
	}
}
