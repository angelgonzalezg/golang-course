package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// Embed ordinary operations and replace only keyed creation for transport tests.
// Real durability belongs to the PostgreSQL integration suite.
type keyedStore struct {
	ContactStore
	createOnce func(context.Context, string, string, contacts.Input) (contacts.CreationResult, error)
}

func (s keyedStore) CreateOnce(ctx context.Context, subject, key string, input contacts.Input) (contacts.CreationResult, error) {
	return s.createOnce(ctx, subject, key, input)
}

// The adapter passes the original decoded fields and request context, while
// identity comes from the configured resolver rather than a spoofed header.
func TestKeyedCreatePassesTrustedIdentityAndOriginalInput(t *testing.T) {
	ctx := t.Context()
	calls := 0
	store := keyedStore{ContactStore: contacts.NewStore(), createOnce: func(got context.Context, subject, key string, input contacts.Input) (contacts.CreationResult, error) {
		calls++
		if got != ctx || subject != "trusted-user" || key != "create-1" || input != (contacts.Input{Name: " Ana ", Email: " mail ", Phone: " 123 "}) {
			t.Fatalf("wrong keyed request: subject=%q key=%q input=%+v", subject, key, input)
		}
		return contacts.CreationResult{Contact: contacts.Contact{ID: 7, Name: "Ana", Email: "mail", Phone: "123"}, Replayed: calls > 1}, nil
	}}
	handler := NewHandlerWithIdempotency(store, func(*http.Request) string { return "trusted-user" }, nil)
	var original string
	for _, replayed := range []string{"false", "true"} {
		req := httptest.NewRequest("POST", "/contacts", strings.NewReader(`{"name":" Ana ","email":" mail ","phone":" 123 "}`)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "create-1")
		req.Header.Set("X-Subject", "untrusted-user")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != 201 || response.Header().Get("Idempotency-Replayed") != replayed || response.Header().Get("Location") != "/contacts/7" || response.Header().Get("ETag") == "" || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("keyed response: %d %v %s", response.Code, response.Header(), response.Body.String())
		}
		if original == "" {
			original = response.Body.String()
		} else if response.Body.String() != original {
			t.Fatal("replay representation changed")
		}
	}
}

// Bad, repeated, or unsupported keys must never fall back to an ordinary POST.
// Empty/misconfigured identities and key conflicts have explicit HTTP responses.
func TestKeyedCreateRejectsInvalidConfigurationAndHeaders(t *testing.T) {
	calls := 0
	store := keyedStore{ContactStore: contacts.NewStore(), createOnce: func(context.Context, string, string, contacts.Input) (contacts.CreationResult, error) {
		calls++
		return contacts.CreationResult{}, contacts.ErrKeyReuse
	}}
	for _, test := range []struct {
		name    string
		keys    []string
		subject string
		basic   bool
		status  int
	}{
		{"empty key", []string{""}, "user", false, 400},
		{"long key", []string{strings.Repeat("k", 65)}, "user", false, 400},
		{"spaces", []string{"two keys"}, "user", false, 400},
		{"non-ASCII", []string{"clé"}, "user", false, 400},
		{"combined values", []string{"a,b"}, "user", false, 400},
		{"repeated key", []string{"a", "a"}, "user", false, 400},
		{"unsupported", []string{"key"}, "user", true, 400},
		{"caller unavailable", []string{"key"}, "", false, 403},
		{"bad trusted subject", []string{"key"}, "bad subject", false, 500},
		{"key conflict", []string{"key"}, "user", false, 409},
	} {
		t.Run(test.name, func(t *testing.T) {
			var handler http.Handler = NewHandler(store, nil)
			if !test.basic {
				handler = NewHandlerWithIdempotency(store, func(*http.Request) string { return test.subject }, nil)
			}
			response := conditionalRequest(handler, "POST", "/contacts", `{"name":"Ana"}`, http.Header{"Idempotency-Key": test.keys})
			expectProblem(t, response, test.status)
			if response.Header().Get("Idempotency-Replayed") != "" {
				t.Fatal("error carries replay success header")
			}
		})
	}
	if calls != 1 {
		t.Fatalf("keyed store called %d times, want only the valid conflict case", calls)
	}
	page, err := store.Search(t.Context(), contacts.Query{Limit: 20})
	if err != nil || page.Total != 0 {
		t.Fatalf("rejected keys created ordinary rows: %+v, %v", page, err)
	}
}
