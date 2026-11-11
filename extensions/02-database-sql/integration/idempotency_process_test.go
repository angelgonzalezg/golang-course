//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	json "encoding/json/v2"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"example.com/course/sql-lab/contactidem"
	"example.com/course/sql-lab/contactsql"
)

// Discard a successful child process's reply, then retry in a second process.
// The new process must recover the same committed result without any shared Go cache.
func testCreationProcessRestart(t *testing.T, db *sql.DB, dsn string) {
	idempotencyStore(t, db, dsn)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	run := func(output io.Writer) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "-test.run=^TestIdempotencyProcessHelper$")
		command.Env = append(os.Environ(), "SQL_LAB_IDEMPOTENCY_CHILD=1", "SQL_LAB_CHILD_DATABASE_URL="+dsn)
		var stderr bytes.Buffer
		command.Stdout, command.Stderr = output, &stderr
		if err := command.Run(); err != nil {
			t.Fatalf("idempotency child failed: %v\n%s", err, stderr.String())
		}
	}
	// This is a controlled lost-reply model, not simulated packet loss. The
	// first child completes its transaction, but the caller keeps no response.
	run(io.Discard)
	var originalID int64
	if err := db.QueryRowContext(t.Context(), "SELECT id FROM sql_idem_contacts WHERE name = 'Process contact'").Scan(&originalID); err != nil {
		t.Fatal(err)
	}
	var response bytes.Buffer
	run(&response)
	var recovered contactidem.Result
	if err := json.Unmarshal(response.Bytes(), &recovered); err != nil {
		t.Fatal(err)
	}
	if !recovered.Replayed || recovered.Contact.ID != originalID || recovered.Contact.Name != "Process contact" {
		t.Fatalf("new process recovered %+v, want original ID %d", recovered, originalID)
	}
	assertIdempotencyCounts(t, db, 1, 1)
}

// This test-binary entry point runs only when the parent fixture requests a
// child. Each child opens/closes its own pool, creates/replays once, and emits
// its result. Normal integration runs simply return from this helper entry.
func TestIdempotencyProcessHelper(t *testing.T) {
	if os.Getenv("SQL_LAB_IDEMPOTENCY_CHILD") != "1" {
		return
	}
	if err := runIdempotencyChild(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// runIdempotencyChild has already completed its defers. Exit avoids adding
	// testing's PASS text to the JSON that the parent decodes.
	os.Exit(0)
}

// The child owns only its connection pool, not test rows or schema. Leaving
// committed rows behind is essential for the second process's durability check.
func runIdempotencyChild() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := contactsql.Open(ctx, os.Getenv("SQL_LAB_CHILD_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := contactidem.NewStore(db).CreateOnce(ctx, "process-user", "process-key", contactsql.Input{Name: "Process contact"})
	if err != nil {
		return err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}
