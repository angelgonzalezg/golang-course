//go:build integration

package integration_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"example.com/course/sql-lab/contactsql"
)

type restOutcome struct {
	reply wireReply
	err   error
}

// Independent servers use independent pools. Twelve retries must share one
// committed result; different payloads racing under another key must conflict.
func testRESTConcurrentCreates(t *testing.T, db *sql.DB, dsn string) {
	second, err := contactsql.Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	servers := []*httptest.Server{httptest.NewServer(persistentREST(db, "user")), httptest.NewServer(persistentREST(second, "user"))}
	for _, server := range servers {
		defer server.Close()
	}
	ctx, cancel := restWorkerContext(t)
	defer cancel()
	start := make(chan struct{})
	outcomes := make(chan restOutcome, 12)
	var workers sync.WaitGroup
	for i := range 12 {
		workers.Go(func() {
			<-start
			server := servers[i%2]
			reply, err := sendREST(ctx, server.Client(), server.URL, "POST", "/contacts", `{"name":"Ana"}`, http.Header{"Idempotency-Key": {"key"}})
			outcomes <- restOutcome{reply, err}
		})
	}
	close(start)
	workers.Wait()
	close(outcomes)
	fresh := 0
	var original wireReply
	var results []wireReply
	for outcome := range outcomes {
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if outcome.reply.Status != 201 {
			t.Fatalf("concurrent POST = %+v", outcome.reply)
		}
		if outcome.reply.Headers["Idempotency-Replayed"] == "false" {
			fresh++
			original = outcome.reply
		}
		results = append(results, outcome.reply)
	}
	if fresh != 1 {
		t.Fatalf("fresh creations = %d, want 1", fresh)
	}
	for _, result := range results {
		if result.Headers["Idempotency-Replayed"] != "false" {
			assertRESTReplay(t, original, result)
		}
	}
	assertServiceCounts(t, db, 1, 1)

	// Reuse a new key concurrently for two different operations: whichever wins
	// binds that key, and the other cannot create a second contact under it.
	start = make(chan struct{})
	outcomes = make(chan restOutcome, 2)
	for i, body := range []string{`{"name":"Bea"}`, `{"name":"Cara"}`} {
		workers.Go(func() {
			<-start
			server := servers[i]
			reply, err := sendREST(ctx, server.Client(), server.URL, "POST", "/contacts", body, http.Header{"Idempotency-Key": {"conflicting"}})
			outcomes <- restOutcome{reply, err}
		})
	}
	close(start)
	workers.Wait()
	close(outcomes)
	statuses := map[int]int{}
	for outcome := range outcomes {
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		statuses[outcome.reply.Status]++
	}
	if statuses[201] != 1 || statuses[409] != 1 {
		t.Fatalf("competing payloads = %v", statuses)
	}
	assertServiceCounts(t, db, 2, 2)
}

// Two conditional writers share one ETag: only one may succeed. Unconditional
// PATCHes of independent fields must then both survive without a lost update.
func testRESTConcurrentMutations(t *testing.T, db *sql.DB, dsn string) {
	second, err := contactsql.Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	servers := []*httptest.Server{httptest.NewServer(persistentREST(db, "user")), httptest.NewServer(persistentREST(second, "user"))}
	for _, server := range servers {
		defer server.Close()
	}
	created := mustREST(t, servers[0], "POST", "/contacts", `{"name":"Ana"}`, nil, 201)
	ctx, cancel := restWorkerContext(t)
	defer cancel()
	for round, bodies := range [][]string{{`{"name":"Writer A"}`, `{"name":"Writer B"}`}, {`{"email":"mail"}`, `{"phone":"123"}`}} {
		start := make(chan struct{})
		outcomes := make(chan restOutcome, 2)
		var workers sync.WaitGroup
		for i, body := range bodies {
			workers.Go(func() {
				<-start
				var headers http.Header
				if round == 0 {
					headers = http.Header{"If-Match": {created.Headers["ETag"]}}
				}
				server := servers[i]
				reply, err := sendREST(ctx, server.Client(), server.URL, "PATCH", created.Headers["Location"], body, headers)
				outcomes <- restOutcome{reply, err}
			})
		}
		close(start)
		workers.Wait()
		close(outcomes)
		statuses := map[int]int{}
		for outcome := range outcomes {
			if outcome.err != nil {
				t.Fatal(outcome.err)
			}
			statuses[outcome.reply.Status]++
		}
		if round == 0 && (statuses[200] != 1 || statuses[412] != 1) || round == 1 && statuses[200] != 2 {
			t.Fatalf("mutation round %d = %v", round, statuses)
		}
	}
	fresh := restContact(t, mustREST(t, servers[0], "GET", created.Headers["Location"], "", nil, 200))
	if fresh.Email != "mail" || fresh.Phone != "123" {
		t.Fatalf("PATCH lost an independent edit: %+v", fresh)
	}
}
