package httpapi

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

func request(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func decodeResponse[T any](t *testing.T, response *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, status, response.Body.String())
	}
	var value T
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode response: %v; body = %s", err, response.Body.String())
	}
	return value
}

func expectProblem(t *testing.T, response *httptest.ResponseRecorder, status int) Problem {
	t.Helper()
	problem := decodeResponse[Problem](t, response, status)
	if response.Header().Get("Content-Type") != "application/problem+json" ||
		problem.Type != "about:blank" || problem.Status != status ||
		problem.Title != http.StatusText(status) || problem.Detail == "" {
		t.Fatalf("invalid problem representation: headers %v, body %+v", response.Header(), problem)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("problem response lacks cache policy")
	}
	return problem
}

func TestContactResourceLifecycle(t *testing.T) {
	handler := NewHandler(contacts.NewStore(), nil)
	index := decodeResponse[struct {
		Links map[string]string `json:"links"`
	}](t, request(handler, "GET", "/", ""), 200)
	if index.Links["contacts"] != "/contacts" {
		t.Fatalf("discovery links = %v", index.Links)
	}
	empty := decodeResponse[Collection](t, request(handler, "GET", "/contacts", ""), 200)
	if empty.Contacts == nil || len(empty.Contacts) != 0 || empty.Links.Self != "/contacts" {
		t.Fatalf("empty collection = %+v", empty)
	}
	response := request(handler, "POST", "/contacts", `{"name":" Ana ","email":" ana@example.com ","phone":" 555-0100 "}`)
	created := decodeResponse[ContactResource](t, response, 201)
	path := response.Header().Get("Location")
	if created.ID != 1 || path != "/contacts/1" || created.Links.Self != path || created.Links.Collection != "/contacts" || created.Name != "Ana" {
		t.Fatalf("creation = %+v, Location = %q", created, path)
	}
	if response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("representation headers = %v", response.Header())
	}
	got := decodeResponse[ContactResource](t, request(handler, "GET", path, ""), 200)
	if got != created || got.Email != "ana@example.com" || got.Phone != "555-0100" {
		t.Fatalf("get = %+v", got)
	}
	expectProblem(t, request(handler, "POST", "/contacts", `{"name":"Ana"}`), 409)
	updated := decodeResponse[ContactResource](t, request(handler, "PUT", path, `{"name":"Ana Torres"}`), 200)
	if updated.ID != created.ID || updated.Name != "Ana Torres" || updated.Email != "" || updated.Phone != "" {
		t.Fatalf("full replacement = %+v", updated)
	}
	again := decodeResponse[ContactResource](t, request(handler, "PUT", path, `{"name":"Ana Torres"}`), 200)
	if again != updated {
		t.Fatalf("repeated replacement = %+v, want %+v", again, updated)
	}
	deleted := request(handler, "DELETE", path, "")
	if deleted.Code != 204 || deleted.Body.Len() != 0 || deleted.Header().Get("Content-Type") != "" {
		t.Fatalf("delete response = %+v, %s", deleted.Result(), deleted.Body.String())
	}
	expectProblem(t, request(handler, "DELETE", path, ""), 404)
	expectProblem(t, request(handler, "GET", path, ""), 404)
	final := decodeResponse[Collection](t, request(handler, "GET", "/contacts", ""), 200)
	if len(final.Contacts) != 0 {
		t.Fatalf("deleted contact remains: %+v", final)
	}
}

func TestRejectedReplacementLeavesContactIntact(t *testing.T) {
	handler := NewHandler(contacts.NewStore(), nil)
	original := decodeResponse[ContactResource](t, request(handler, "POST", "/contacts", `{"name":"Ana","email":"ana@example.com"}`), 201)
	decodeResponse[ContactResource](t, request(handler, "POST", "/contacts", `{"name":"Bob"}`), 201)
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"name":"Bob"}`, 409},
		{`{"name":" "}`, 400},
		{`{"email":"new@example.com"}`, 400},
		{`{"name":"Changed","id":99}`, 400},
	} {
		expectProblem(t, request(handler, "PUT", original.Links.Self, tc.body), tc.status)
		got := decodeResponse[ContactResource](t, request(handler, "GET", original.Links.Self, ""), 200)
		if got != original {
			t.Fatalf("rejected replacement changed contact: %+v", got)
		}
	}
	expectProblem(t, request(handler, "PUT", "/contacts/99", `{"name":"Missing"}`), 404)
	list := decodeResponse[Collection](t, request(handler, "GET", "/contacts", ""), 200)
	if len(list.Contacts) != 2 {
		t.Fatalf("replacement created a missing resource: %+v", list)
	}
}

func TestInputContract(t *testing.T) {
	for _, tc := range []struct {
		name, body, mediaType, encoding string
		status                          int
	}{
		{"missing media type", `{"name":"Ana"}`, "", "", 415},
		{"wrong media type", `{"name":"Ana"}`, "text/plain", "", 415},
		{"malformed media type", `{}`, `application/json; charset="`, "", 415},
		{"compressed", `{}`, "application/json", "gzip", 415},
		{"empty", "", "application/json", "", 400},
		{"root null", "null", "application/json", "", 400},
		{"array", `[{"name":"Ana"}]`, "application/json", "", 400},
		{"scalar", `"Ana"`, "application/json", "", 400},
		{"multiple documents", `{"name":"Ana"} {}`, "application/json", "", 400},
		{"unknown member", `{"name":"Ana","id":1}`, "application/json", "", 400},
		{"duplicate member", `{"name":"Ana","name":"Bob"}`, "application/json", "", 400},
		{"wrong type", `{"name":123}`, "application/json", "", 400},
		{"blank name", `{"name":null}`, "application/json", "", 400},
		{"invalid UTF-8", "{\"name\":\"\xff\"}", "application/json", "", 400},
		{"long name", `{"name":"` + strings.Repeat("界", 81) + `"}`, "application/json", "", 400},
		{"parameters", `{"name":"Ana"}`, "application/json; charset=utf-8", "identity", 201},
		{"optional nulls", `{"name":"Ana","email":null,"phone":null}`, "application/json", "", 201},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := contacts.NewStore()
			handler := NewHandler(store, nil)
			req := httptest.NewRequest("POST", "/contacts", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.mediaType)
			req.Header.Set("Content-Encoding", tc.encoding)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if tc.status == 201 {
				got := decodeResponse[ContactResource](t, response, tc.status)
				if got.Email != "" || got.Phone != "" {
					t.Fatalf("optional fields = %+v", got)
				}
			} else {
				expectProblem(t, response, tc.status)
				list, _ := store.List(t.Context())
				if len(list) != 0 {
					t.Fatalf("invalid request wrote data: %v", list)
				}
			}
		})
	}
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("upload failed") }
func (brokenBody) Close() error             { return nil }

func TestBodyLimitAndReadFailure(t *testing.T) {
	for _, size := range []int{MaxRequestBytes, MaxRequestBytes + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			body := `{"name":"Ana"}`
			body += strings.Repeat(" ", size-len(body))
			req := httptest.NewRequest("POST", "/contacts", strings.NewReader(body))
			req.ContentLength = -1 // The bound must not depend on Content-Length.
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			NewHandler(contacts.NewStore(), nil).ServeHTTP(response, req)
			if size == MaxRequestBytes {
				decodeResponse[ContactResource](t, response, 201)
			} else {
				expectProblem(t, response, 413)
			}
		})
	}
	req := httptest.NewRequest("POST", "/contacts", nil)
	req.Body = brokenBody{}
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	NewHandler(contacts.NewStore(), nil).ServeHTTP(response, req)
	expectProblem(t, response, 400)
}

func TestRoutingErrors(t *testing.T) {
	handler := NewHandler(contacts.NewStore(), nil)
	for _, text := range []string{"0", "-1", "01", "+1", "abc", "9223372036854775808"} {
		for _, method := range []string{"GET", "PUT", "DELETE"} {
			expectProblem(t, request(handler, method, "/contacts/"+text, `{"name":"Ana"}`), 400)
		}
	}
	for _, path := range []string{"/missing", "/contacts/1/extra", "/contacts/"} {
		expectProblem(t, request(handler, "GET", path, ""), 404)
	}
	for _, tc := range []struct{ method, path, allow string }{
		{"POST", "/", "GET, HEAD"},
		{"DELETE", "/contacts", "GET, HEAD, POST"},
		{"PATCH", "/contacts/1", "DELETE, GET, HEAD, PUT"},
	} {
		response := request(handler, tc.method, tc.path, "")
		expectProblem(t, response, 405)
		if response.Header().Get("Allow") != tc.allow {
			t.Fatalf("Allow = %q, want %q", response.Header().Get("Allow"), tc.allow)
		}
	}
}

// Embed the unused methods so each test supplies only the behavior it exercises.
type getStore struct {
	ContactStore
	get func(context.Context, int64) (contacts.Contact, error)
}

func (s getStore) Get(ctx context.Context, id int64) (contacts.Contact, error) {
	return s.get(ctx, id)
}

func TestStoreErrorMappingAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{fmt.Errorf("wrapped: %w", contacts.ErrNotFound), 404},
		{contacts.ErrNameTaken, 409},
		{&contacts.ValidationError{Field: "id", Message: "must be positive"}, 400},
		{contacts.ErrIDExhausted, 503},
		{context.Canceled, 503},
		{context.DeadlineExceeded, 503},
		{errors.New("private backend detail"), 500},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			var logs bytes.Buffer
			store := getStore{get: func(context.Context, int64) (contacts.Contact, error) { return contacts.Contact{}, tc.err }}
			handler := NewHandler(store, slog.New(slog.NewTextHandler(&logs, nil)))
			response := request(handler, "GET", "/contacts/1", "")
			expectProblem(t, response, tc.status)
			if tc.status == 500 && (strings.Contains(response.Body.String(), "private backend detail") || !strings.Contains(logs.String(), "private backend detail")) {
				t.Fatalf("unexpected error must be logged but hidden: body %s, log %s", response.Body.String(), logs.String())
			}
		})
	}
}

func TestRequestContextReachesStore(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	store := getStore{get: func(got context.Context, id int64) (contacts.Contact, error) {
		if got != ctx || id != 7 {
			t.Errorf("store received context %v, id %d", got, id)
		}
		return contacts.Contact{}, got.Err()
	}}
	response := httptest.NewRecorder()
	NewHandler(store, nil).ServeHTTP(response, httptest.NewRequest("GET", "/contacts/7", nil).WithContext(ctx))
	expectProblem(t, response, 503)
}

func TestEncodingFailureBeforeSuccessIsCommitted(t *testing.T) {
	store := getStore{get: func(context.Context, int64) (contacts.Contact, error) {
		return contacts.Contact{ID: 1, Name: "\xff"}, nil
	}}
	response := request(NewHandler(store, nil), "GET", "/contacts/1", "")
	expectProblem(t, response, 500)
}

func TestConcurrentHTTPRequestsAndHEAD(t *testing.T) {
	server := httptest.NewServer(NewHandler(contacts.NewStore(), nil))
	defer server.Close()
	client := server.Client()
	client.Timeout = 3 * time.Second
	const workers = 16
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Go(func() {
			body := fmt.Sprintf(`{"name":"Contact %d"}`, worker)
			req, err := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/contacts", strings.NewReader(body))
			if err != nil {
				t.Error(err)
				return
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			_, readErr := io.Copy(io.Discard, resp.Body)
			if err := errors.Join(readErr, resp.Body.Close()); err != nil {
				t.Error(err)
			}
			if resp.StatusCode != 201 {
				t.Errorf("concurrent POST status = %d", resp.StatusCode)
			}
		})
	}
	wg.Wait()
	for _, method := range []string{"GET", "HEAD"} {
		req, err := http.NewRequestWithContext(t.Context(), method, server.URL+"/contacts", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(resp.Body)
		if err := errors.Join(readErr, resp.Body.Close()); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("%s response headers = %v, status %d", method, resp.Header, resp.StatusCode)
		}
		if method == "HEAD" {
			if len(data) != 0 {
				t.Fatalf("HEAD returned a body: %s", data)
			}
			continue
		}
		var list Collection
		if err := json.Unmarshal(data, &list); err != nil || len(list.Contacts) != workers {
			t.Fatalf("concurrent collection = %s, %v", data, err)
		}
		for i, contact := range list.Contacts {
			if contact.ID != int64(i+1) {
				t.Errorf("ordered IDs = %+v", list.Contacts)
			}
		}
	}
}
