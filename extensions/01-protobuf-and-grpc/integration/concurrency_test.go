//go:build integration

package integration_test

import (
	"database/sql"
	"sync"
	"testing"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/rpcpostgres"
	"example.com/course/sql-lab/contactpg"
	"example.com/course/sql-lab/contactsql"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type createOutcome struct {
	contact *contactsv1.Contact
	trailer metadata.MD
	err     error
}

// Independent servers/pools must share one database result. Conditional writers
// must also share one row lock, and independent partial edits must both survive.
func testConcurrentRPCs(t *testing.T, db *sql.DB, dsn string) {
	other, err := contactsql.Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Registered before endpoints, so LIFO cleanup stops their handlers first.
	t.Cleanup(func() { other.Close() })
	servers := []*endpoint{newEndpoint(t, contactpg.NewStore(db), false, nil), newEndpoint(t, contactpg.NewStore(other), false, nil)}
	start := make(chan struct{})
	results := make(chan createOutcome, 12)
	var workers sync.WaitGroup
	for i := range 12 {
		workers.Go(func() {
			<-start
			reply, trailer, err := invokeCreate(t.Context(), servers[i%2].api, "shared", &contactsv1.ContactInput{Name: "Ana"})
			results <- createOutcome{reply, trailer, err}
		})
	}
	close(start)
	workers.Wait()
	close(results)
	fresh := 0
	var original *contactsv1.Contact
	var outcomes []createOutcome
	for got := range results {
		if got.err != nil {
			t.Fatal(got.err)
		}
		outcomes = append(outcomes, got)
		if values := got.trailer.Get(rpcpostgres.ReplayedTrailer); len(values) == 1 && values[0] == "false" {
			fresh++
			original = got.contact
		}
	}
	if fresh != 1 {
		t.Fatalf("fresh creations=%d,want 1", fresh)
	}
	for _, got := range outcomes {
		if !proto.Equal(got.contact, original) {
			t.Fatalf("different concurrent result: %v", got.contact)
		}
	}
	assertCounts(t, db, 1, 1)
	// One original version can authorize only one changing writer.
	for round, inputs := range [][]*contactsv1.ContactInput{{{Name: "Writer A"}, {Name: "Writer B"}}, {{Email: "mail"}, {Phone: "123"}}} {
		start := make(chan struct{})
		results := make(chan error, 2)
		for i, input := range inputs {
			workers.Go(func() {
				<-start
				path := "name"
				var version *string
				if round == 0 {
					version = proto.String(original.Version)
				} else if i == 0 {
					path = "email"
				} else {
					path = "phone"
				}
				_, err := servers[i].api.UpdateContact(t.Context(), &contactsv1.UpdateContactRequest{Id: original.Id, Input: input, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{path}}, ExpectedVersion: version})
				results <- err
			})
		}
		close(start)
		workers.Wait()
		close(results)
		codesSeen := map[codes.Code]int{}
		for err := range results {
			codesSeen[status.Code(err)]++
		}
		if round == 0 && (codesSeen[codes.OK] != 1 || codesSeen[codes.Aborted] != 1) || round == 1 && codesSeen[codes.OK] != 2 {
			t.Fatalf("writer round %d=%v", round, codesSeen)
		}
	}
	current, err := servers[0].api.GetContact(t.Context(), &contactsv1.GetContactRequest{Id: original.Id})
	if err != nil || current.Email != "mail" || current.Phone != "123" {
		t.Fatalf("partial update lost fields: %v,%v", current, err)
	}
	// Different valid payloads racing under a fresh key bind only one request.
	start = make(chan struct{})
	conflicts := make(chan error, 2)
	for i, name := range []string{"Bea", "Cara"} {
		workers.Go(func() {
			<-start
			_, _, err := invokeCreate(t.Context(), servers[i].api, "conflict", &contactsv1.ContactInput{Name: name})
			conflicts <- err
		})
	}
	close(start)
	workers.Wait()
	close(conflicts)
	codesSeen := map[codes.Code]int{}
	for err := range conflicts {
		codesSeen[status.Code(err)]++
	}
	if codesSeen[codes.OK] != 1 || codesSeen[codes.FailedPrecondition] != 1 {
		t.Fatalf("key race=%v", codesSeen)
	}
	assertCounts(t, db, 2, 2)
}
