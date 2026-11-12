//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/rpcresilience"
	"example.com/course/sql-lab/contactpg"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// Exercise the durable service through its real RPC transport. Every named
// case starts with empty service tables in this suite's own PostgreSQL container.
func TestPostgreSQL(t *testing.T) {
	db, dsn := newDatabase(t)
	for _, test := range []struct {
		name string
		run  func(*testing.T, *sql.DB, string)
	}{
		{"memory and PostgreSQL unary contracts", testUnaryContracts},
		{"memory and PostgreSQL streaming contracts", testStreamingContracts},
		{"replay binding and authorization", testReplayAndAccess},
		{"lost reply and bounded application retry", testLostReply},
		{"concurrent retries and atomic versions", testConcurrentRPCs},
		{"rollback and redacted database errors", testRollback},
		{"cancellation reaches SQL locks", testCancellation},
		{"cross-transport shared storage and replay", testRESTInterop},
		{"new process recovers committed creation", testProcessRestart},
		{"TLS discovery limits and telemetry", testInfrastructure},
		{"finite demo preserves unrelated rows", testDemo},
	} {
		t.Run(test.name, func(t *testing.T) { resetService(t, db); test.run(t, db, dsn) })
	}
}

// A fresh outgoing metadata copy labels a single intended create. Other RPCs
// use the original context so keys cannot accidentally leak onto streams/reads.
func keyed(ctx context.Context, key string) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(rpcresilience.KeyHeader, key)
	return metadata.NewOutgoingContext(ctx, md)
}

// Compare successful resource messages and expected-version behavior through
// both backends. Valid creates precede any unique violation because SQL IDs may gap.
func testUnaryContracts(t *testing.T, db *sql.DB, _ string) {
	var baseline []*contactsv1.Contact
	for _, memory := range []bool{true, false} {
		t.Run(map[bool]string{true: "memory", false: "PostgreSQL"}[memory], func(t *testing.T) {
			h := newEndpoint(t, contactpg.NewStore(db), memory, nil)
			ctx := t.Context()
			var observed []*contactsv1.Contact
			created, err := h.api.CreateContact(keyed(ctx, "create-1"), &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: " Ana ", Email: " mail ", Phone: " 123 "}})
			if err != nil {
				t.Fatal(err)
			}
			observed = append(observed, created)
			if _, err := h.api.CreateContact(keyed(ctx, "create-2"), &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "Bob\x00UTF-8", Email: "a\x00b"}}); err != nil {
				t.Fatal(err)
			}
			read, err := h.api.GetContact(ctx, &contactsv1.GetContactRequest{Id: created.Id})
			if err != nil || !proto.Equal(read, created) {
				t.Fatalf("get=%v,%v", read, err)
			}
			updated, err := h.api.UpdateContact(ctx, &contactsv1.UpdateContactRequest{
				Id: created.Id, Input: &contactsv1.ContactInput{Phone: "", Name: "ignored"},
				UpdateMask:      &fieldmaskpb.FieldMask{Paths: []string{"phone"}},
				ExpectedVersion: proto.String(created.Version),
			})
			if err != nil || updated.Phone != "" || updated.Email != "mail" {
				t.Fatalf("masked update=%v,%v", updated, err)
			}
			observed = append(observed, updated)
			_, err = h.api.ReplaceContact(ctx, &contactsv1.ReplaceContactRequest{Id: created.Id, Input: &contactsv1.ContactInput{Name: "Stale"}, ExpectedVersion: proto.String(created.Version)})
			if status.Code(err) != codes.Aborted {
				t.Fatalf("stale replacement=%v", err)
			}
			replaced, err := h.api.ReplaceContact(ctx, &contactsv1.ReplaceContactRequest{Id: created.Id, Input: &contactsv1.ContactInput{Name: " Ana Updated "}, ExpectedVersion: proto.String(updated.Version)})
			if err != nil || replaced.Email != "" || replaced.Phone != "" {
				t.Fatalf("full replacement=%v,%v", replaced, err)
			}
			observed = append(observed, replaced)
			page, err := h.api.ListContacts(ctx, &contactsv1.ListContactsRequest{Limit: 1, Sort: contactsv1.SortOrder_SORT_ORDER_NAME_ASC})
			if err != nil || page.Total != 2 || page.NextOffset == nil || *page.NextOffset != 1 || !proto.Equal(page.Contacts[0], replaced) {
				t.Fatalf("page=%v,%v", page, err)
			}
			_, err = h.api.CreateContact(keyed(ctx, "duplicate"), &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "Ana Updated"}})
			if status.Code(err) != codes.AlreadyExists {
				t.Fatalf("duplicate=%v", err)
			}
			if _, err := h.api.DeleteContact(ctx, &contactsv1.DeleteContactRequest{Id: created.Id, ExpectedVersion: proto.String(replaced.Version)}); err != nil {
				t.Fatal(err)
			}
			if _, err := h.api.GetContact(ctx, &contactsv1.GetContactRequest{Id: created.Id}); status.Code(err) != codes.NotFound {
				t.Fatalf("deleted get=%v", err)
			}
			if baseline == nil {
				baseline = observed
			} else {
				for i := range observed {
					if !proto.Equal(baseline[i], observed[i]) {
						t.Fatalf("resource %d differs: memory=%v PostgreSQL=%v", i, baseline[i], observed[i])
					}
				}
			}
		})
	}
}

// Preserve per-item import commits, finite ordered server streams, and bidi
// item failures with a real database. Streams never imply replay of a batch.
func testStreamingContracts(t *testing.T, db *sql.DB, _ string) {
	for _, memory := range []bool{true, false} {
		t.Run(map[bool]string{true: "memory", false: "PostgreSQL"}[memory], func(t *testing.T) {
			h := newEndpoint(t, contactpg.NewStore(db), memory, nil)
			ctx := t.Context()
			importer, err := h.api.ImportContacts(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"Zoë", "Ana", "ana"} {
				if err := importer.Send(&contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: name}}); err != nil {
					t.Fatal(err)
				}
			}
			summary, err := importer.CloseAndRecv()
			if err != nil || summary.CreatedCount != 3 || len(summary.Ids) != 3 {
				t.Fatalf("import=%v,%v", summary, err)
			}
			partial, err := h.api.ImportContacts(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := partial.Send(&contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "Partial"}}); err != nil {
				t.Fatal(err)
			}
			if err := partial.Send(&contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{}}); err != nil {
				t.Fatal(err)
			}
			if _, err := partial.CloseAndRecv(); status.Code(err) != codes.InvalidArgument {
				t.Fatalf("partial import=%v", err)
			}
			stream, err := h.api.StreamContacts(ctx, &contactsv1.ListContactsRequest{NameContains: "AN", Sort: contactsv1.SortOrder_SORT_ORDER_NAME_ASC, Limit: 100})
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for {
				item, err := stream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				names = append(names, item.Name)
			}
			if len(names) != 2 || names[0] != "Ana" || names[1] != "ana" {
				t.Fatalf("ordered stream=%v", names)
			}
			page, err := h.api.ListContacts(ctx, &contactsv1.ListContactsRequest{})
			if err != nil || page.Total != 4 {
				t.Fatalf("partial commit lost: %v,%v", page, err)
			}
			lookup, err := h.api.LookupContacts(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for i, id := range []int64{summary.Ids[0], 0, 999} {
				if err := lookup.Send(&contactsv1.LookupContactsRequest{RequestId: uint64(i), Id: id}); err != nil {
					t.Fatal(err)
				}
				reply, err := lookup.Recv()
				if err != nil || reply.RequestId != uint64(i) {
					t.Fatalf("lookup=%v,%v", reply, err)
				}
				if (i == 0 && reply.GetContact() == nil) ||
					(i == 1 && reply.GetFailure().GetCode() != contactsv1.LookupFailureCode_LOOKUP_FAILURE_CODE_INVALID_ARGUMENT) ||
					(i == 2 && reply.GetFailure().GetCode() != contactsv1.LookupFailureCode_LOOKUP_FAILURE_CODE_NOT_FOUND) {
					t.Fatalf("lookup item outcome=%v", reply)
				}
			}
			if err := lookup.CloseSend(); err != nil {
				t.Fatal(err)
			}
			if _, err := lookup.Recv(); !errors.Is(err, io.EOF) {
				t.Fatalf("lookup completion=%v", err)
			}
		})
	}
}
