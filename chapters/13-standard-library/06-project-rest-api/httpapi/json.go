package httpapi

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// MaxRequestBytes bounds each accepted JSON body, including whitespace. Field
// limits count runes after trimming and are validated separately by the store.
const MaxRequestBytes = 4 * 1024

// MergePatchMediaType selects the omission/string/null semantics used by PATCH.
const MergePatchMediaType = "application/merge-patch+json"

type contactRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// Problem uses RFC 9457's about:blank shape; Status matches the HTTP status.
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

// readInput turns accepted JSON into the store's ordinary Go input. On failure
// it writes the problem response itself and returns false; the calling handler
// must return then, rather than attempt to send a second response.
func (a *api) readInput(w http.ResponseWriter, r *http.Request) (contacts.Input, bool) {
	data, ok := a.readBody(w, r, "application/json")
	if !ok {
		return contacts.Input{}, false
	}
	// Fresh storage makes omitted optional fields empty on every PUT.
	// v2 also maps explicit null strings to their zero value, an empty string.
	var input *contactRequest
	if err := json.Unmarshal(data, &input, json.RejectUnknownMembers(true)); err != nil || input == nil {
		a.writeProblem(w, http.StatusBadRequest, "body must be one contact object with only name, email, and phone")
		return contacts.Input{}, false
	}
	return contacts.Input{Name: input.Name, Email: input.Email, Phone: input.Phone}, true
}

func (a *api) readChanges(w http.ResponseWriter, r *http.Request) (contacts.Changes, bool) {
	data, ok := a.readBody(w, r, MergePatchMediaType)
	if !ok {
		return contacts.Changes{}, false
	}
	// A map preserves member presence. With only *string struct fields,
	// both an omitted member and JSON null would decode to nil.
	var members map[string]*string
	if err := json.Unmarshal(data, &members); err != nil || members == nil {
		a.writeProblem(w, http.StatusBadRequest, "patch must be one object with string or null values")
		return contacts.Changes{}, false
	}
	var changes contacts.Changes
	for field, value := range members {
		if value == nil {
			// null requests removal. Empty optional text is the domain's way
			// to represent missing data; removing the required name fails validation.
			empty := ""
			value = &empty
		}
		switch field {
		case "name":
			changes.Name = value
		case "email":
			changes.Email = value
		case "phone":
			changes.Phone = value
		default:
			a.writeProblem(w, http.StatusBadRequest, "patch may change only name, email, and phone")
			return contacts.Changes{}, false
		}
	}
	return changes, true
}

func (a *api) readBody(w http.ResponseWriter, r *http.Request, expectedMediaType string) ([]byte, bool) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	encoding := strings.TrimSpace(r.Header.Get("Content-Encoding"))
	if err != nil || mediaType != expectedMediaType || (encoding != "" && !strings.EqualFold(encoding, "identity")) {
		a.writeProblem(w, http.StatusUnsupportedMediaType, "send an uncompressed "+expectedMediaType+" object")
		return nil, false
	}
	// Bound bytes read from the network before decoding JSON. Field validation
	// later answers whether those values form a valid contact.
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			a.writeProblem(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("body exceeds %d bytes", MaxRequestBytes))
		} else {
			a.writeProblem(w, http.StatusBadRequest, "could not read request body")
		}
		return nil, false
	}
	return data, true
}

// The store reports Go errors without HTTP knowledge. This adapter translates
// them to statuses such as 400/404/409 and a consistent JSON problem body.
func (a *api) writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, contacts.ErrInvalidInput):
		a.writeProblem(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, contacts.ErrNotFound):
		a.writeProblem(w, http.StatusNotFound, "contact not found")
	case errors.Is(err, contacts.ErrNameTaken):
		a.writeProblem(w, http.StatusConflict, "another contact already uses that name")
	case errors.Is(err, contacts.ErrKeyReuse):
		a.writeProblem(w, http.StatusConflict, "Idempotency-Key already identifies different input; retry the original input")
	case errors.Is(err, contacts.ErrPreconditionFailed):
		a.writeProblem(w, http.StatusPreconditionFailed, "contact version precondition failed; read the current contact before retrying")
	case errors.Is(err, contacts.ErrIDExhausted):
		a.writeProblem(w, http.StatusServiceUnavailable, "no contact IDs are available")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// This origin server uses 503 when its operation cannot complete. A client
		// that canceled the connection usually observes its own context error.
		a.writeProblem(w, http.StatusServiceUnavailable, "request could not be completed")
	default:
		a.logger.Error("contact operation failed", "error", err)
		a.writeProblem(w, http.StatusInternalServerError, "request could not be completed")
	}
}

func (a *api) writeJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		a.logger.Error("encode response", "error", err)
		w.Header().Del("Location")
		a.writeProblem(w, http.StatusInternalServerError, "response could not be encoded")
		return
	}
	a.writeBytes(w, status, "application/json", data)
}

func (a *api) writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Del("ETag")
	w.Header().Set("Cache-Control", "no-store")
	// This representation has only valid constant text, validated domain messages,
	// and an integer status. It is encoded before any response is committed.
	data, err := json.Marshal(Problem{Type: "about:blank", Title: http.StatusText(status), Status: status, Detail: detail})
	if err != nil {
		a.logger.Error("encode problem", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	a.writeBytes(w, status, "application/problem+json", data)
}

func (a *api) writeBytes(w http.ResponseWriter, status int, contentType string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if _, err := w.Write(append(data, '\n')); err != nil {
		a.logger.Error("write response", "error", err)
	}
}
