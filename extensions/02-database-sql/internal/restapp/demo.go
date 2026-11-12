package restapp

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"example.com/course/sql-lab/contactpg"
	"example.com/course/sql-lab/contactsql"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/httpapi"
)

// RunDemo makes real loopback HTTP requests, with PostgreSQL behind the shared
// handler. Only this run's random subject and its contact/result are cleaned up.
func RunDemo(ctx context.Context, dsn string, out io.Writer) (result error) {
	if _, err := contactpg.Migrate(dsn); err != nil {
		return err
	}
	db, err := contactsql.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	subject := "demo-" + rand.Text()[:12]
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		result = errors.Join(result, cleanupDemo(cleanup, db, subject))
	}()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	handler := httpapi.NewHandlerWithIdempotency(contactpg.NewStore(db), func(*http.Request) string { return subject }, nil)
	server, served := start(listener, handler)
	defer func() { result = errors.Join(result, stop(server, served)) }()
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 4 * time.Second}
	base := "http://" + listener.Addr().String()
	name := "REST " + subject
	body, err := json.Marshal(map[string]string{"name": " " + name + " ", "email": " original@example.com ", "phone": " 555-0100 "})
	if err != nil {
		return err
	}
	key := http.Header{"Idempotency-Key": {"create-1"}}

	fmt.Fprintln(out, "== 1. Durable keyed POST ==")
	first, err := call(ctx, client, base, "POST", "/contacts", string(body), key, 201)
	if err != nil {
		return err
	}
	again, err := call(ctx, client, base, "POST", "/contacts", string(body), key, 201)
	if err != nil {
		return err
	}
	if first.header.Get("Idempotency-Replayed") != "false" || again.header.Get("Idempotency-Replayed") != "true" || !bytes.Equal(first.body, again.body) || first.header.Get("ETag") != again.header.Get("ETag") {
		return errors.New("POST did not replay the original result")
	}
	path := first.header.Get("Location")
	fmt.Fprintln(out, "Repeated POST -> same 201, Location, ETag, and contact; replay=true.")

	fmt.Fprintln(out, "\n== 2. Bind the key to its original fields ==")
	if _, err := call(ctx, client, base, "POST", "/contacts", `{"name":"Different"}`, key, 409); err != nil {
		return err
	}
	fmt.Fprintln(out, "Changed fields under the same key -> 409; no extra contact.")

	fmt.Fprintln(out, "\n== 3. Keep PATCH, PUT, paging, and ETag behavior ==")
	patched, err := call(ctx, client, base, "PATCH", path, `{"email":null}`, http.Header{"If-Match": {first.header.Get("ETag")}}, 200)
	if err != nil {
		return err
	}
	if _, err := call(ctx, client, base, "PATCH", path, `{"phone":"stale"}`, http.Header{"If-Match": {first.header.Get("ETag")}}, 412); err != nil {
		return err
	}
	putBody, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return err
	}
	put, err := call(ctx, client, base, "PUT", path, string(putBody), http.Header{"If-Match": {patched.header.Get("ETag")}}, 200)
	if err != nil {
		return err
	}
	if put.header.Get("ETag") != "" {
		return errors.New("normalized PUT emitted a success validator")
	}
	fresh, err := call(ctx, client, base, "GET", path, "", nil, 200)
	if err != nil {
		return err
	}
	var value httpapi.ContactResource
	if err := json.Unmarshal(fresh.body, &value); err != nil {
		return err
	}
	if value.Name != name || value.Email != "" || value.Phone != "" {
		return errors.New("PUT did not replace all editable fields")
	}
	if _, err := call(ctx, client, base, "GET", "/contacts?limit=2&sort=name&name_contains="+subject, "", nil, 200); err != nil {
		return err
	}
	fmt.Fprintln(out, "PATCH clears one field; PUT clears omitted fields; stale ETag -> 412.")

	fmt.Fprintln(out, "\n== 4. Replay after the live contact is deleted ==")
	if _, err := call(ctx, client, base, "DELETE", path, "", http.Header{"If-Match": {fresh.header.Get("ETag")}}, 204); err != nil {
		return err
	}
	historical, err := call(ctx, client, base, "POST", "/contacts", string(body), key, 201)
	if err != nil {
		return err
	}
	if !bytes.Equal(historical.body, first.body) || historical.header.Get("ETag") != first.header.Get("ETag") || historical.header.Get("Idempotency-Replayed") != "true" {
		return errors.New("historical HTTP creation result changed")
	}
	if _, err := call(ctx, client, base, "GET", path, "", nil, 404); err != nil {
		return err
	}
	fmt.Fprintln(out, "Original creation replays from PostgreSQL; deleted contact stays absent.")
	return nil
}

type reply struct {
	header http.Header
	body   []byte
}

// call closes each response and checks its status. It keeps the demo focused on
// the protocol while using ordinary net/http requests and caller cancellation.
func call(ctx context.Context, client *http.Client, base, method, path, body string, headers http.Header, status int) (reply, error) {
	req, err := http.NewRequestWithContext(ctx, method, base+path, strings.NewReader(body))
	if err != nil {
		return reply{}, err
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
		return reply{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return reply{}, err
	}
	if response.StatusCode != status {
		return reply{}, fmt.Errorf("%s %s: status %d, want %d", method, path, response.StatusCode, status)
	}
	return reply{response.Header.Clone(), data}, nil
}

// cleanupDemo is for this finite example only. Interactive services retain
// creation records: deleting them deliberately forgets their replay guarantee.
func cleanupDemo(ctx context.Context, db *sql.DB, subject string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sql_service_contacts WHERE id IN
		(SELECT response_id FROM sql_service_create_requests WHERE subject = $1)`, subject); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sql_service_create_requests WHERE subject = $1", subject); err != nil {
		return err
	}
	return tx.Commit()
}
