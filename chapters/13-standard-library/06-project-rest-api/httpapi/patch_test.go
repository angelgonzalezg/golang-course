package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

func TestPUTReplacesWhilePATCHPreservesOmittedFields(t *testing.T) {
	handler := NewHandler(contacts.NewStore(), nil)
	original := decodeResponse[ContactResource](t, request(handler, "POST", "/contacts",
		`{"name":"Ana","email":"ana@example.com","phone":"555-0100"}`), 201)
	path := original.Links.Self
	// Both requests supply only a name. PATCH keeps the other two fields.
	patched := decodeResponse[ContactResource](t, request(handler, "PATCH", path, `{"name":"Ana Torres"}`), 200)
	if patched.ID != original.ID || patched.Name != "Ana Torres" || patched.Email != original.Email || patched.Phone != original.Phone {
		t.Fatalf("PATCH did not preserve omitted fields: %+v", patched)
	}
	for _, body := range []string{`{"name":"Ana Torres"}`, `{}`} {
		again := decodeResponse[ContactResource](t, request(handler, "PATCH", path, body), 200)
		if again != patched {
			t.Fatalf("repeated or empty patch changed state: %+v", again)
		}
	}
	// PUT replaces all editable fields. Its omitted optional fields become empty.
	response := request(handler, "PUT", path, `{"name":"Ana Torres"}`)
	replaced := decodeResponse[ContactResource](t, response, 200)
	if replaced.ID != original.ID || replaced.Name != patched.Name || replaced.Email != "" || replaced.Phone != "" {
		t.Fatalf("PUT did not replace all editable fields: %+v", replaced)
	}
	representation := decodeResponse[map[string]any](t, response, 200)
	for _, field := range []string{"email", "phone"} {
		if _, present := representation[field]; present {
			t.Errorf("empty optional %s must be absent from JSON: %v", field, representation)
		}
	}
}

func TestPatchDocumentContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		status int
		want   contacts.Input
	}{
		{name: "one field", body: `{"email":" new@example.com "}`, status: 200, want: contacts.Input{Name: "Ana", Email: "new@example.com", Phone: "555-0100"}},
		{name: "multiple fields", body: `{"name":"Ana Torres","phone":"555-9999"}`, status: 200, want: contacts.Input{Name: "Ana Torres", Email: "ana@example.com", Phone: "555-9999"}},
		{name: "remove optional", body: `{"email":null}`, status: 200, want: contacts.Input{Name: "Ana", Phone: "555-0100"}},
		{name: "empty optional", body: `{"email":"","phone":"   "}`, status: 200, want: contacts.Input{Name: "Ana"}},
		{name: "empty object", body: `{}`, status: 200, want: contacts.Input{Name: "Ana", Email: "ana@example.com", Phone: "555-0100"}},
		{name: "root null", body: `null`, status: 400},
		{name: "root array", body: `[]`, status: 400},
		{name: "root scalar", body: `"Ana"`, status: 400},
		{name: "empty body", body: ``, status: 400},
		{name: "extra document", body: `{} {}`, status: 400},
		{name: "duplicate member", body: `{"email":"first@example.com","email":"second@example.com"}`, status: 400},
		{name: "server ID", body: `{"id":1}`, status: 400},
		{name: "server links", body: `{"links":null}`, status: 400},
		{name: "unknown member", body: `{"email":"new@example.com","nickname":null}`, status: 400},
		{name: "wrong case", body: `{"Email":"new@example.com"}`, status: 400},
		{name: "wrong type", body: `{"email":42}`, status: 400},
		{name: "nested object", body: `{"email":{"value":"new@example.com"}}`, status: 400},
		{name: "invalid UTF-8", body: "{\"email\":\"\xff\"}", status: 400},
		{name: "remove required name", body: `{"name":null,"email":"new@example.com"}`, status: 400},
		{name: "blank name", body: `{"name":" ","phone":null}`, status: 400},
		{name: "long phone", body: `{"email":null,"phone":"` + strings.Repeat("1", 41) + `"}`, status: 400},
		{name: "duplicate name", body: `{"name":"Bob","email":"new@example.com"}`, status: 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewHandler(contacts.NewStore(), nil)
			before := decodeResponse[ContactResource](t, request(handler, "POST", "/contacts",
				`{"name":"Ana","email":"ana@example.com","phone":"555-0100"}`), 201)
			decodeResponse[ContactResource](t, request(handler, "POST", "/contacts", `{"name":"Bob"}`), 201)
			response := request(handler, "PATCH", before.Links.Self, tc.body)
			if response.Header().Get("Accept-Patch") != MergePatchMediaType {
				t.Fatal("PATCH response does not advertise its document format")
			}
			if tc.status != 200 {
				expectProblem(t, response, tc.status)
				got := decodeResponse[ContactResource](t, request(handler, "GET", before.Links.Self, ""), 200)
				if got != before {
					t.Fatalf("rejected patch changed contact: %+v", got)
				}
				return
			}
			got := decodeResponse[ContactResource](t, response, 200)
			if got.ID != before.ID || got.Links != before.Links {
				t.Fatalf("patch changed resource identity: %+v", got)
			}
			if got.Name != tc.want.Name || got.Email != tc.want.Email || got.Phone != tc.want.Phone {
				t.Errorf("patched fields = %+v, want %+v", got, tc.want)
			}
			if tc.name == "remove optional" {
				body := decodeResponse[map[string]any](t, response, 200)
				if _, present := body["email"]; present {
					t.Fatalf("null must remove the optional JSON member: %v", body)
				}
			}
		})
	}
}

func TestPatchMissingContactAndRemovalIsIdempotent(t *testing.T) {
	handler := NewHandler(contacts.NewStore(), nil)
	expectProblem(t, request(handler, "PATCH", "/contacts/99", `{"email":"new@example.com"}`), 404)
	expectProblem(t, request(handler, "PATCH", "/contacts/99", `{}`), 404)
	created := decodeResponse[ContactResource](t, request(handler, "POST", "/contacts", `{"name":"Ana","email":"ana@example.com"}`), 201)
	for range 2 {
		removed := decodeResponse[ContactResource](t, request(handler, "PATCH", created.Links.Self, `{"email":null}`), 200)
		if removed.Name != created.Name || removed.Email != "" {
			t.Fatalf("repeated removal = %+v", removed)
		}
	}
}

func TestPatchMediaTypeAndBodyLimits(t *testing.T) {
	for _, tc := range []struct {
		name, mediaType, encoding, body string
		status                          int
	}{
		{"missing type", "", "", `{}`, 415},
		{"ordinary JSON type", "application/json", "", `{}`, 415},
		{"JSON Patch type", "application/json-patch+json", "", `[]`, 415},
		{"malformed type", `application/merge-patch+json; charset="`, "", `{}`, 415},
		{"compressed", MergePatchMediaType, "gzip", `{}`, 415},
		{"parameters", MergePatchMediaType + "; charset=utf-8", "identity", `{}`, 200},
		{"at limit", MergePatchMediaType, "", `{}` + strings.Repeat(" ", MaxRequestBytes-2), 200},
		{"over limit", MergePatchMediaType, "", `{}` + strings.Repeat(" ", MaxRequestBytes-1), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewHandler(contacts.NewStore(), nil)
			before := decodeResponse[ContactResource](t, request(handler, "POST", "/contacts", `{"name":"Ana"}`), 201)
			req := httptest.NewRequest("PATCH", before.Links.Self, strings.NewReader(tc.body))
			req.ContentLength = -1
			req.Header.Set("Content-Type", tc.mediaType)
			req.Header.Set("Content-Encoding", tc.encoding)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if tc.status == 200 {
				decodeResponse[ContactResource](t, response, 200)
			} else {
				expectProblem(t, response, tc.status)
			}
			got := decodeResponse[ContactResource](t, request(handler, "GET", before.Links.Self, ""), 200)
			if got != before {
				t.Fatalf("body check changed contact: %+v", got)
			}
		})
	}
	req := httptest.NewRequest("PATCH", "/contacts/1", nil)
	req.Header.Set("Content-Type", MergePatchMediaType)
	req.Body = brokenBody{}
	response := httptest.NewRecorder()
	NewHandler(contacts.NewStore(), nil).ServeHTTP(response, req)
	expectProblem(t, response, 400)
}

func TestOptionsAdvertisesPatch(t *testing.T) {
	response := request(NewHandler(contacts.NewStore(), nil), "OPTIONS", "/contacts/1", "")
	if response.Code != 204 || response.Body.Len() != 0 || response.Header().Get("Content-Type") != "" {
		t.Fatalf("OPTIONS = %d, %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Allow") != "DELETE, GET, HEAD, OPTIONS, PATCH, PUT" || response.Header().Get("Accept-Patch") != MergePatchMediaType {
		t.Fatalf("OPTIONS capabilities = %v", response.Header())
	}
}

type patchStore struct {
	ContactStore
	patch func(context.Context, int64, contacts.Changes) (contacts.Contact, error)
}

func (s patchStore) Patch(ctx context.Context, id int64, changes contacts.Changes) (contacts.Contact, error) {
	return s.patch(ctx, id, changes)
}

func TestPatchPassesContextAndSelectedFields(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	store := patchStore{patch: func(got context.Context, id int64, changes contacts.Changes) (contacts.Contact, error) {
		if got != ctx || id != 7 || changes.Name != nil || changes.Email == nil || *changes.Email != "" || changes.Phone == nil || *changes.Phone != "555-9999" {
			t.Errorf("unexpected store call: context %v, id %d, changes %+v", got, id, changes)
		}
		return contacts.Contact{}, got.Err()
	}}
	req := httptest.NewRequest("PATCH", "/contacts/7", strings.NewReader(`{"email":null,"phone":"555-9999"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", MergePatchMediaType)
	response := httptest.NewRecorder()
	NewHandler(store, nil).ServeHTTP(response, req)
	expectProblem(t, response, 503)
}
