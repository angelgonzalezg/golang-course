//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"strings"
	"sync/atomic"
	"testing"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/rpcpostgres"
	"example.com/course/grpc-lab/internal/rpcresilience"
	"example.com/course/sql-lab/contactpg"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// Capture invocation metadata separately from the saved resource fields.
func invokeCreate(ctx context.Context, api contactsv1.ContactServiceClient, key string, input *contactsv1.ContactInput) (*contactsv1.Contact, metadata.MD, error) {
	var trailer metadata.MD
	contact, err := api.CreateContact(keyed(ctx, key), &contactsv1.CreateContactRequest{Input: input}, grpc.Trailer(&trailer))
	return contact, trailer, err
}

// Successful replay must retain the original ID, editable fields, and content
// version. The trailer alone would not prove that the result stayed unchanged.
func assertReplay(t *testing.T, got, want *contactsv1.Contact, trailer metadata.MD, err error) {
	t.Helper()
	if err != nil || !proto.Equal(got, want) || len(trailer.Get(rpcpostgres.ReplayedTrailer)) != 1 || trailer.Get(rpcpostgres.ReplayedTrailer)[0] != "true" {
		t.Fatalf("replay=%v, metadata=%v, error=%v; want %v", got, trailer, err, want)
	}
}

// Bind the caller/key to known original fields, retain historical results, and
// authenticate/authorize each replay. Unknown protobuf fields have no business
// meaning in this typed policy, unlike stage 5's whole-message cache comparison.
func testReplayAndAccess(t *testing.T, db *sql.DB, _ string) {
	h := newEndpoint(t, contactpg.NewStore(db), false, nil)
	ctx := t.Context()
	input := &contactsv1.ContactInput{Name: " Ana ", Email: " original ", Phone: "123"}
	first, trailer, err := invokeCreate(ctx, h.api, "key", input)
	if err != nil || len(trailer.Get(rpcpostgres.ReplayedTrailer)) != 1 || trailer.Get(rpcpostgres.ReplayedTrailer)[0] != "false" {
		t.Fatalf("first create=%v,%v,%v", first, trailer, err)
	}
	again, md, err := invokeCreate(ctx, h.api, "key", proto.Clone(input).(*contactsv1.ContactInput))
	assertReplay(t, again, first, md, err)
	again.Email = "caller edit"
	unknown := &contactsv1.CreateContactRequest{Input: proto.Clone(input).(*contactsv1.ContactInput)}
	unknown.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01}) // Unknown field 100, varint 1.
	md = nil
	again, err = h.api.CreateContact(keyed(ctx, "key"), unknown, grpc.Trailer(&md))
	assertReplay(t, again, first, md, err)
	if _, _, err := invokeCreate(ctx, h.api, "key", &contactsv1.ContactInput{Name: "Ana", Email: " original ", Phone: "123"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("changed whitespace=%v", err)
	}
	_, reader := h.client(t, h.reader)
	_, anonymous := h.client(t, "")
	_, invalidToken := h.client(t, "wrong-token")
	for _, test := range []struct {
		name string
		api  contactsv1.ContactServiceClient
		code codes.Code
	}{{"reader", reader, codes.PermissionDenied}, {"anonymous", anonymous, codes.Unauthenticated}, {"invalid token", invalidToken, codes.Unauthenticated}} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := invokeCreate(t.Context(), test.api, "key", input); status.Code(err) != test.code {
				t.Fatalf("replay bypassed authorization: %v", err)
			}
		})
	}
	for _, values := range [][]string{nil, {""}, {strings.Repeat("k", 65)}, {"two keys"}, {"a", "a"}} {
		ctx := metadata.NewOutgoingContext(ctx, metadata.MD{rpcresilience.KeyHeader: values})
		if _, err := h.api.CreateContact(ctx, &contactsv1.CreateContactRequest{Input: input}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("bad/missing key=%v", err)
		}
	}
	if _, err := h.api.GetContact(keyed(ctx, "key"), &contactsv1.GetContactRequest{Id: first.Id}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("key leaked to read: %v", err)
	}
	// Unary middleware does not apply per stream item, and streams reject keys.
	stream, err := h.api.ImportContacts(keyed(ctx, "key"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.CloseAndRecv(); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("keyed stream=%v", err)
	}
	updated, err := h.api.UpdateContact(ctx, &contactsv1.UpdateContactRequest{Id: first.Id, Input: &contactsv1.ContactInput{Email: "current"}, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"email"}}})
	if err != nil {
		t.Fatal(err)
	}
	again, md, err = invokeCreate(ctx, h.api, "key", input)
	assertReplay(t, again, first, md, err)
	if _, err := h.api.DeleteContact(ctx, &contactsv1.DeleteContactRequest{Id: first.Id, ExpectedVersion: proto.String(updated.Version)}); err != nil {
		t.Fatal(err)
	}
	again, md, err = invokeCreate(ctx, h.api, "key", input)
	assertReplay(t, again, first, md, err)
	if _, err := h.api.GetContact(ctx, &contactsv1.GetContactRequest{Id: first.Id}); status.Code(err) != codes.NotFound {
		t.Fatalf("replay recreated deleted contact: %v", err)
	}
	assertCounts(t, db, 0, 1)
}

// A local post-commit hook hides one successful reply. The existing bounded
// retry client must recover via a database replay, not repeat the business insert.
func testLostReply(t *testing.T, db *sql.DB, _ string) {
	var calls atomic.Int64
	var drop atomic.Bool
	drop.Store(true)
	faults := func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if info.FullMethod != contactsv1.ContactService_CreateContact_FullMethodName {
			return next(ctx, request)
		}
		calls.Add(1)
		response, err := next(ctx, request)
		if err == nil && drop.CompareAndSwap(true, false) {
			return nil, status.Error(codes.Unavailable, "controlled lost reply")
		}
		return response, err
	}
	h := newEndpoint(t, contactpg.NewStore(db), false, faults)
	var trailer metadata.MD
	result, err := rpcresilience.NewClient(h.api).CreateOnce(t.Context(), "lost-reply", &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "Ana"}}, grpc.Trailer(&trailer))
	if err != nil || result.GetId() <= 0 || calls.Load() != 2 || len(trailer.Get(rpcpostgres.ReplayedTrailer)) != 1 || trailer.Get(rpcpostgres.ReplayedTrailer)[0] != "true" {
		t.Fatalf("retry=%v,%v; calls=%d trailer=%v", result, err, calls.Load(), trailer)
	}
	assertCounts(t, db, 1, 1)
}

// Fail the result INSERT after the contact INSERT. Both must roll back, the RPC
// error must stay redacted, and known failures must leave a key available.
func testRollback(t *testing.T, db *sql.DB, _ string) {
	if _, err := db.ExecContext(t.Context(), "ALTER TABLE sql_service_create_requests ADD CONSTRAINT test_reject_rpc_result CHECK (false)"); err != nil {
		t.Fatal(err)
	}
	h := newEndpoint(t, contactpg.NewStore(db), false, nil)
	input := &contactsv1.ContactInput{Name: "Ana"}
	if _, _, err := invokeCreate(t.Context(), h.api, "key", input); status.Code(err) != codes.Internal || strings.Contains(err.Error(), "sql_service") || strings.Contains(err.Error(), "test_reject") {
		t.Fatalf("database error exposed/incorrectly mapped: %v", err)
	}
	assertCounts(t, db, 0, 0)
	if _, err := db.ExecContext(t.Context(), "ALTER TABLE sql_service_create_requests DROP CONSTRAINT test_reject_rpc_result"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := invokeCreate(t.Context(), h.api, "key", input); err != nil {
		t.Fatal(err)
	}
	if _, _, err := invokeCreate(t.Context(), h.api, "correctable", input); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("name conflict=%v", err)
	}
	if _, _, err := invokeCreate(t.Context(), h.api, "correctable", &contactsv1.ContactInput{Name: "Bea"}); err != nil {
		t.Fatal(err)
	}
	assertCounts(t, db, 2, 2)
}
