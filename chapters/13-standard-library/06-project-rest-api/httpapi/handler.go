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
	Search(context.Context, contacts.Query) (contacts.Page, error)
	ReplaceIf(context.Context, int64, contacts.Input, contacts.Conditions) (contacts.Contact, error)
	PatchIf(context.Context, int64, contacts.Changes, contacts.Conditions) (contacts.Contact, error)
	DeleteIf(context.Context, int64, contacts.Conditions) error
}

// Links exposes relative resource/navigation URLs in HTTP representations.
type Links struct {
	Self       string `json:"self"`
	Collection string `json:"collection,omitempty"`
	First      string `json:"first,omitempty"`
	Next       string `json:"next,omitempty"`
	Prev       string `json:"prev,omitempty"`
}

// ContactResource is the HTTP representation, including server-owned ID/links.
// It stays separate from writable input and the transport-independent Contact.
type ContactResource struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Missing optional data is absent from JSON, including after PATCH null.
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
	Links Links  `json:"links"`
}

// Collection combines one page with its navigation and snapshot pagination data.
type Collection struct {
	Contacts   []ContactResource `json:"contacts"`
	Links      Links             `json:"links"`
	Pagination Pagination        `json:"pagination"`
}

type api struct {
	store  ContactStore
	logger *slog.Logger
}

const contactMethods = "DELETE, GET, HEAD, OPTIONS, PATCH, PUT"

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
	// GET patterns also match HEAD. net/http suppresses the body on the wire;
	// running the same handler still supplies matching headers and status.
	mux.HandleFunc("GET /{$}", a.index)
	mux.HandleFunc("/{$}", a.methodNotAllowed("GET, HEAD"))
	mux.HandleFunc("GET /openapi.json", a.openAPI)
	mux.HandleFunc("/openapi.json", a.methodNotAllowed("GET, HEAD"))
	mux.HandleFunc("GET /contacts", a.list)
	mux.HandleFunc("POST /contacts", a.create)
	mux.HandleFunc("/contacts", a.methodNotAllowed("GET, HEAD, POST"))
	mux.HandleFunc("GET /contacts/{id}", a.get)
	mux.HandleFunc("PUT /contacts/{id}", a.replace)
	mux.HandleFunc("PATCH /contacts/{id}", a.patch)
	mux.HandleFunc("OPTIONS /contacts/{id}", a.options)
	mux.HandleFunc("DELETE /contacts/{id}", a.delete)
	mux.HandleFunc("/contacts/{id}", a.methodNotAllowed(contactMethods))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		a.writeProblem(w, http.StatusNotFound, "resource not found")
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Errors and mutation responses stay non-cacheable. Successful reads
		// select private revalidation in writeReadBytes.
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
	a.writeReadJSON(w, r, map[string]any{"links": map[string]string{"contacts": "/contacts", "openapi": "/openapi.json"}}, "")
}

func (a *api) list(w http.ResponseWriter, r *http.Request) {
	query, ok := a.readQuery(w, r)
	if !ok {
		return
	}
	page, err := a.store.Search(r.Context(), query)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	result := Collection{
		Contacts:   make([]ContactResource, 0, len(page.Contacts)),
		Links:      pageLinks(page),
		Pagination: Pagination{Limit: page.Query.Limit, Offset: page.Query.Offset, Total: page.Total},
	}
	for _, value := range page.Contacts {
		result.Contacts = append(result.Contacts, resource(value))
	}
	a.writeReadJSON(w, r, result, "")
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
	a.writeReadJSON(w, r, resource(value), contactTag(value))
}

func (a *api) create(w http.ResponseWriter, r *http.Request) {
	if a.rejectConditions(w, r) {
		return
	}
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
	w.Header().Set("ETag", contactTag(value))
	a.writeJSON(w, http.StatusCreated, result)
}

func (a *api) replace(w http.ResponseWriter, r *http.Request) {
	id, ok := a.readID(w, r)
	if !ok {
		return
	}
	// PUT supplies every editable field; omitted optional fields become empty.
	input, ok := a.readInput(w, r)
	if !ok {
		return
	}
	conditions, ok := a.readConditions(w, r)
	if !ok {
		return
	}
	value, err := a.store.ReplaceIf(r.Context(), id, input, conditions)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	// PUT normalizes input and constructs a representation. HTTP forbids a
	// success validator after such a transformation; GET supplies the new ETag.
	a.writeJSON(w, http.StatusOK, resource(value))
}

func (a *api) patch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Accept-Patch", MergePatchMediaType)
	id, ok := a.readID(w, r)
	if !ok {
		return
	}
	// PATCH carries changes: omitted fields keep their current values.
	changes, ok := a.readChanges(w, r)
	if !ok {
		return
	}
	conditions, ok := a.readConditions(w, r)
	if !ok {
		return
	}
	value, err := a.store.PatchIf(r.Context(), id, changes, conditions)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	w.Header().Set("ETag", contactTag(value))
	a.writeJSON(w, http.StatusOK, resource(value))
}

func (a *api) options(w http.ResponseWriter, r *http.Request) {
	if a.rejectConditions(w, r) {
		return
	}
	if _, ok := a.readID(w, r); !ok {
		return
	}
	// Describe this route's capabilities without reading or changing a contact.
	w.Header().Set("Allow", contactMethods)
	w.Header().Set("Accept-Patch", MergePatchMediaType)
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := a.readID(w, r)
	if !ok {
		return
	}
	conditions, ok := a.readConditions(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteIf(r.Context(), id, conditions); err != nil {
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
