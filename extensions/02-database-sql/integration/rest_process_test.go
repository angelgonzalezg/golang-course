//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"example.com/course/sql-lab/contactsql"
)

// Discard one child's successful HTTP reply, then retry in a new OS process
// with a new pool/server. The saved creation must survive without any Go cache.
func testRESTProcessRestart(t *testing.T, db *sql.DB, dsn string) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	run := func(output io.Writer) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "-test.run=^TestRESTProcessHelper$")
		command.Env = append(os.Environ(), "SQL_LAB_REST_CHILD=1", "SQL_LAB_REST_DATABASE_URL="+dsn)
		var stderr bytes.Buffer
		command.Stdout, command.Stderr = output, &stderr
		if err := command.Run(); err != nil {
			t.Fatalf("REST child failed: %v\n%s", err, stderr.String())
		}
	}
	// Controlled lost-response model: work commits, but the parent retains no
	// reply. This does not claim to simulate packet loss on a real network.
	run(io.Discard)
	var id int64
	if err := db.QueryRowContext(t.Context(), "SELECT id FROM sql_service_contacts WHERE name = $1", []byte("Process contact")).Scan(&id); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	run(&output)
	var reply wireReply
	if err := json.Unmarshal(output.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	resource := restContact(t, reply)
	if reply.Status != 201 || reply.Headers["Idempotency-Replayed"] != "true" || resource.ID != id || resource.Name != "Process contact" || resource.Phone != "123" {
		t.Fatalf("new process recovered wrong HTTP result: %+v", reply)
	}
	assertServiceCounts(t, db, 1, 1)
}

// The parent invokes this test-binary entry only with its child environment.
// Ordinary integration runs return here without opening an extra fixture.
func TestRESTProcessHelper(t *testing.T) {
	if os.Getenv("SQL_LAB_REST_CHILD") != "1" {
		return
	}
	if err := runRESTChild(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// All child defers have completed; avoid appending testing's PASS to JSON.
	os.Exit(0)
}

// A child owns its pool, server, and response encoding. It intentionally leaves
// committed rows behind so another process can replay the HTTP creation.
func runRESTChild() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := contactsql.Open(ctx, os.Getenv("SQL_LAB_REST_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()
	server := httptest.NewServer(persistentREST(db, "process-user"))
	defer server.Close()
	reply, err := sendREST(ctx, server.Client(), server.URL, "POST", "/contacts", `{"name":"Process contact","phone":"123"}`, http.Header{"Idempotency-Key": {"process-key"}})
	if err != nil {
		return err
	}
	if reply.Status != 201 {
		return fmt.Errorf("child POST status: %d", reply.Status)
	}
	data, err := json.Marshal(reply)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}
