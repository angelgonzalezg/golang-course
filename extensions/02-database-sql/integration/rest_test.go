//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	json "encoding/json/v2"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"example.com/course/sql-lab/contactpg"
	"example.com/course/sql-lab/internal/restapp"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/httpapi"
)

// Exercise real HTTP and PostgreSQL together. Each named case owns empty service
// tables inside this suite's disposable database; earlier lessons stay separate.
func TestRESTPostgreSQL(t *testing.T) {
	db, dsn := newTestDatabase(t)
	if _, err := contactpg.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		run  func(*testing.T, *sql.DB, string)
	}{
		{"memory and PostgreSQL HTTP contracts", testRESTHTTPContracts},
		{"durable replay and request binding", testRESTReplay},
		{"concurrent retries across HTTP servers", testRESTConcurrentCreates},
		{"conditional and partial concurrent writes", testRESTConcurrentMutations},
		{"saved result failure rolls back POST", testRESTResultFailure},
		{"canceled writes and waiting duplicates", testRESTCancellation},
		{"lost HTTP reply and new process", testRESTProcessRestart},
		{"finite walkthrough preserves other data", testRESTDemo},
		{"independent repeatable migrations", testRESTMigrations},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := db.ExecContext(t.Context(), "TRUNCATE sql_service_create_requests, sql_service_contacts RESTART IDENTITY"); err != nil {
				t.Fatal(err)
			}
			test.run(t, db, dsn)
		})
	}
}

// Supply identity from application configuration; client identity headers are
// deliberately irrelevant to the key namespace in this single-user test setup.
func persistentREST(db *sql.DB, subject string) http.Handler {
	return httpapi.NewHandlerWithIdempotency(contactpg.NewStore(db), func(*http.Request) string { return subject }, nil)
}

// wireReply retains semantic headers only: Date and connection details vary
// independently of the contact API. It can also cross a child-process boundary.
type wireReply struct {
	Status  int
	Body    string
	Headers map[string]string
}

// sendREST closes every response and passes the caller's context. Returning an
// error, rather than calling testing.T, also makes it safe for owned workers.
func sendREST(ctx context.Context, client *http.Client, base, method, path, body string, headers http.Header) (wireReply, error) {
	req, err := http.NewRequestWithContext(ctx, method, base+path, strings.NewReader(body))
	if err != nil {
		return wireReply{}, err
	}
	if method == "POST" || method == "PUT" {
		req.Header.Set("Content-Type", "application/json")
	}
	if method == "PATCH" {
		req.Header.Set("Content-Type", httpapi.MergePatchMediaType)
	}
	for key, values := range headers {
		req.Header[key] = append([]string(nil), values...)
	}
	response, err := client.Do(req)
	if err != nil {
		return wireReply{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return wireReply{}, err
	}
	result := wireReply{Status: response.StatusCode, Body: string(data), Headers: map[string]string{}}
	for _, key := range []string{"ETag", "Location", "Content-Type", "Cache-Control", "X-Content-Type-Options", "Accept-Patch", "Allow", "Idempotency-Replayed"} {
		if value := response.Header.Get(key); value != "" {
			result.Headers[key] = value
		}
	}
	return result, nil
}

// mustREST adds a status assertion for synchronous scenario steps.
func mustREST(t *testing.T, server *httptest.Server, method, path, body string, headers http.Header, status int) wireReply {
	t.Helper()
	result, err := sendREST(t.Context(), server.Client(), server.URL, method, path, body, headers)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != status {
		t.Fatalf("%s %s: %d, want %d; %s", method, path, result.Status, status, result.Body)
	}
	return result
}

// Run identical requests through each backend. Successful creates precede
// duplicate inserts because PostgreSQL identities can have gaps after failures.
func testRESTHTTPContracts(t *testing.T, db *sql.DB, _ string) {
	var baseline []wireReply
	for _, backend := range []struct {
		name    string
		handler http.Handler
	}{
		{"memory", httpapi.NewHandler(contacts.NewStore(), nil)},
		{"PostgreSQL", persistentREST(db, "user")},
	} {
		t.Run(backend.name, func(t *testing.T) {
			server := httptest.NewServer(backend.handler)
			defer server.Close()
			var replies []wireReply
			call := func(method, path, body string, headers http.Header, status int) wireReply {
				t.Helper()
				reply := mustREST(t, server, method, path, body, headers, status)
				replies = append(replies, reply)
				return reply
			}
			call("GET", "/contacts", "", nil, 200)
			created := call("POST", "/contacts", `{"name":" Ana ","email":" mail ","phone":" 123 "}`, nil, 201)
			path, tag := created.Headers["Location"], created.Headers["ETag"]
			for _, body := range []string{`{"name":"Bob"}`, `{"name":"Zoë"}`, `{"name":"ana"}`, `{"name":"NUL\u0000name","email":"a\u0000b"}`, `{"name":"100%_literal"}`} {
				call("POST", "/contacts", body, nil, 201)
			}
			call("POST", "/contacts", `{"name":"Ana"}`, nil, 409)
			call("GET", path, "", nil, 200)
			call("HEAD", path, "", nil, 200)
			call("GET", path, "", http.Header{"If-None-Match": {tag}}, 304)
			noop := call("PATCH", path, `{}`, http.Header{"If-Match": {tag}}, 200)
			if noop.Headers["ETag"] != tag {
				t.Fatal("no-op PATCH changed content ETag")
			}
			call("PATCH", path, `{"name":null}`, nil, 400)
			call("GET", path, "", nil, 200)
			patched := call("PATCH", path, `{"phone":null}`, http.Header{"If-Match": {tag}}, 200)
			put := call("PUT", path, `{"name":" Ana Updated "}`, http.Header{"If-Match": {patched.Headers["ETag"]}}, 200)
			if put.Headers["ETag"] != "" {
				t.Fatal("transformed PUT emitted a success ETag")
			}
			fresh := call("GET", path, "", nil, 200)
			call("PATCH", path, `{"email":"stale"}`, http.Header{"If-Match": {tag}}, 412)
			call("PUT", path, `{"name":"Bob"}`, http.Header{"If-Match": {tag}}, 409)
			for _, query := range []string{"limit=2&offset=1&sort=name", "limit=20&sort=-name&name_contains=AN", "limit=20&name_contains=%C3%8B", "limit=20&name_contains=%25_", "limit=20&offset=" + strconv.Itoa(math.MaxInt), "limit=0"} {
				call("GET", "/contacts?"+query, "", nil, map[bool]int{true: 400, false: 200}[query == "limit=0"])
			}
			call("OPTIONS", path, "", nil, 204)
			call("DELETE", path, "", http.Header{"If-Match": {fresh.Headers["ETag"]}}, 204)
			call("DELETE", path, "", nil, 404)
			call("GET", path, "", nil, 404)
			if baseline == nil {
				baseline = replies
			} else if !reflect.DeepEqual(baseline, replies) {
				for i := range replies {
					if !reflect.DeepEqual(baseline[i], replies[i]) {
						t.Errorf("response %d differs:\nmemory: %+v\nPostgreSQL: %+v", i, baseline[i], replies[i])
					}
				}
			}
		})
	}
}

// Recover the same HTTP snapshot after updates/deletion. Decoded fields, rather
// than JSON member order, bind the key; a changed field must conflict.
func testRESTReplay(t *testing.T, db *sql.DB, _ string) {
	server := httptest.NewServer(persistentREST(db, "user"))
	defer server.Close()
	headers := http.Header{"Idempotency-Key": {"key"}, "X-Subject": {"spoofed"}}
	first := mustREST(t, server, "POST", "/contacts", `{"name":" Ana ","email":null}`, headers, 201)
	if first.Headers["Idempotency-Replayed"] != "false" {
		t.Fatal("first POST was not fresh")
	}
	// null, omitted, and empty optional fields decode to the same domain values.
	again := mustREST(t, server, "POST", "/contacts", "{\n\"email\":\"\", \"name\":\" Ana \"}", headers, 201)
	assertRESTReplay(t, first, again)
	mustREST(t, server, "POST", "/contacts", `{"name":"Ana"}`, headers, 409)
	// Known rollback does not bind a failed key, so corrected input can use it.
	otherKey := http.Header{"Idempotency-Key": {"correctable"}}
	mustREST(t, server, "POST", "/contacts", `{"name":"Ana"}`, otherKey, 409)
	mustREST(t, server, "POST", "/contacts", `{"name":"Bea"}`, otherKey, 201)
	path := first.Headers["Location"]
	mustREST(t, server, "PATCH", path, `{"email":"new"}`, nil, 200)
	assertRESTReplay(t, first, mustREST(t, server, "POST", "/contacts", `{"name":" Ana "}`, headers, 201))
	mustREST(t, server, "DELETE", path, "", nil, 204)
	assertRESTReplay(t, first, mustREST(t, server, "POST", "/contacts", `{"name":" Ana "}`, headers, 201))
	mustREST(t, server, "GET", path, "", nil, 404)
	// Another trusted subject can reuse the same key for a distinct operation.
	other := httptest.NewServer(persistentREST(db, "other-user"))
	defer other.Close()
	mustREST(t, other, "POST", "/contacts", `{"name":"Other caller"}`, headers, 201)
	assertServiceCounts(t, db, 2, 3)
}

// Compare status, representation, Location, and ETag while allowing only the
// invocation's Idempotency-Replayed header to differ from the original.
func assertRESTReplay(t *testing.T, original, replay wireReply) {
	t.Helper()
	if replay.Status != 201 || replay.Headers["Idempotency-Replayed"] != "true" || replay.Body != original.Body || replay.Headers["Location"] != original.Headers["Location"] || replay.Headers["ETag"] != original.Headers["ETag"] {
		t.Fatalf("replay changed original response: %+v, want %+v", replay, original)
	}
}

// Check committed rows independently of the HTTP result, detecting partial or
// duplicate effects that checking a status code alone would miss.
func assertServiceCounts(t *testing.T, db *sql.DB, wantContacts, wantResults int) {
	t.Helper()
	var contacts, results int
	if err := db.QueryRowContext(t.Context(), `SELECT (SELECT count(*) FROM sql_service_contacts), (SELECT count(*) FROM sql_service_create_requests)`).Scan(&contacts, &results); err != nil {
		t.Fatal(err)
	}
	if contacts != wantContacts || results != wantResults {
		t.Fatalf("committed contacts/results = %d/%d, want %d/%d", contacts, results, wantContacts, wantResults)
	}
}

// Inject failure after the contact INSERT. HTTP must expose a generic problem,
// both rows must roll back, and removing the constraint must permit a fresh retry.
func testRESTResultFailure(t *testing.T, db *sql.DB, _ string) {
	if _, err := db.ExecContext(t.Context(), "ALTER TABLE sql_service_create_requests ADD CONSTRAINT test_reject_http_result CHECK (false)"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(persistentREST(db, "user"))
	defer server.Close()
	key := http.Header{"Idempotency-Key": {"key"}}
	failed := mustREST(t, server, "POST", "/contacts", `{"name":"Ana"}`, key, 500)
	if strings.Contains(failed.Body, "sql_service") || strings.Contains(failed.Body, "test_reject") {
		t.Fatal("HTTP exposed database internals")
	}
	assertServiceCounts(t, db, 0, 0)
	if _, err := db.ExecContext(t.Context(), "ALTER TABLE sql_service_create_requests DROP CONSTRAINT test_reject_http_result"); err != nil {
		t.Fatal(err)
	}
	result := mustREST(t, server, "POST", "/contacts", `{"name":"Ana"}`, key, 201)
	if result.Headers["Idempotency-Replayed"] != "false" {
		t.Fatal("rolled-back request remained reserved")
	}
	assertServiceCounts(t, db, 1, 1)
}

// The finite walkthrough must finish its HTTP goroutine and remove only its
// random subject, preserving an unrelated caller's committed contact/result.
func testRESTDemo(t *testing.T, db *sql.DB, dsn string) {
	store := contactpg.NewStore(db)
	first, err := store.CreateOnce(t.Context(), "retained", "key", contacts.Input{Name: "Keep this row"})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := restapp.RunDemo(t.Context(), dsn, &output); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"replay=true", "409", "stale ETag -> 412", "deleted contact stays absent"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("demo missing %q", text)
		}
	}
	again, err := store.CreateOnce(t.Context(), "retained", "key", contacts.Input{Name: "Keep this row"})
	if err != nil || !again.Replayed || again.Contact != first.Contact {
		t.Fatalf("unrelated result changed: %+v, %v", again, err)
	}
	assertServiceCounts(t, db, 1, 1)
}

// Decode a successful resource in assertions, keeping JSON wire fields separate
// from the storage model used by the adapter.
func restContact(t *testing.T, result wireReply) httpapi.ContactResource {
	t.Helper()
	var value httpapi.ContactResource
	if err := json.Unmarshal([]byte(result.Body), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

// A short deadline bounds worker failures; this is not a timing assumption about
// when another request reaches a database lock.
func restWorkerContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), 10*time.Second)
}
