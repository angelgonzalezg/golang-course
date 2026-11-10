package main

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func TestReaderCannotMutateThroughUnaryOrStreaming(t *testing.T) {
	env := setup(t, true, nil)
	client := contactsv1.NewContactServiceClient(env.connection(t, env.secrets.reader))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	input := &contactsv1.ContactInput{Name: "Must not be written"}
	for _, tc := range []struct {
		name string
		call func(context.Context) error
	}{
		{"create", func(ctx context.Context) error {
			_, err := client.CreateContact(ctx, &contactsv1.CreateContactRequest{Input: input})
			return err
		}},
		{"replace", func(ctx context.Context) error {
			_, err := client.ReplaceContact(ctx, &contactsv1.ReplaceContactRequest{Id: 1, Input: input})
			return err
		}},
		{"update", func(ctx context.Context) error {
			_, err := client.UpdateContact(ctx, &contactsv1.UpdateContactRequest{Id: 1, Input: input, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}}})
			return err
		}},
		{"delete", func(ctx context.Context) error {
			_, err := client.DeleteContact(ctx, &contactsv1.DeleteContactRequest{Id: 1})
			return err
		}},
		{"import", func(ctx context.Context) error {
			stream, err := client.ImportContacts(ctx)
			if err != nil {
				return err
			}
			if err := stream.Send(&contactsv1.CreateContactRequest{Input: input}); err != nil && err != io.EOF {
				return err
			}
			_, err = stream.CloseAndRecv()
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Supplying a role header must not elevate an authenticated reader.
			ctx := metadata.AppendToOutgoingContext(ctx, "x-role", "writer")
			if err := tc.call(ctx); status.Code(err) != codes.PermissionDenied {
				t.Fatalf("reader mutation = %v", err)
			}
		})
	}
	page, err := env.store.Search(ctx, contacts.Query{Limit: 100})
	if err != nil || page.Total != 1 || page.Contacts[0].Name != "Private contact" {
		t.Fatalf("denied calls reached mutation: %+v, %v", page, err)
	}
}

func TestAuthenticationFailuresAndMetadataOnDeniedCalls(t *testing.T) {
	env := setup(t, false, nil)
	connection := env.connection(t, "")
	client := contactsv1.NewContactServiceClient(connection)
	for _, authorization := range [][]string{nil, {"Bearer unknown-secret"}, {"Basic reader-test-secret"}, {"Bearer "}, {"Bearer reader-test-secret", "Bearer writer-test-secret"}} {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		md := metadata.Pairs("x-request-id", "denied-call")
		md["authorization"] = authorization
		ctx = metadata.NewOutgoingContext(ctx, md)
		var header, trailer metadata.MD
		_, err := client.GetContact(ctx, &contactsv1.GetContactRequest{Id: 1}, grpc.Header(&header), grpc.Trailer(&trailer))
		if status.Code(err) != codes.Unauthenticated || strings.Contains(err.Error(), "test-secret") || strings.Contains(err.Error(), "unknown-secret") {
			cancel()
			t.Fatalf("authentication response leaked or accepted a credential: %v", err)
		}
		if err := printMetadata("denied", header, trailer, "denied-call", "unary", io.Discard); err != nil {
			cancel()
			t.Fatal(err)
		}
		stream, err := client.StreamContacts(ctx, &contactsv1.ListContactsRequest{})
		if err == nil {
			_, err = stream.Recv()
		}
		cancel()
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("stream bypassed authentication: %v", err)
		}
	}
	// Completion logs include denied calls, without credentials or payloads.
	logs := env.logs.String()
	if !strings.Contains(logs, `"code":"Unauthenticated"`) || !strings.Contains(logs, `"request_id":"denied-call"`) {
		t.Fatalf("outer observer omitted denied calls: %s", logs)
	}
	for _, private := range []string{env.secrets.reader, env.secrets.writer, "unknown-secret", "Private contact", "private@example.com", "authorization"} {
		if strings.Contains(logs, private) {
			t.Fatalf("completion log contains private data %q", private)
		}
	}
}

func TestMetadataValidationAndGeneratedRequestIDs(t *testing.T) {
	env := setup(t, false, nil)
	client := contactsv1.NewContactServiceClient(env.connection(t, env.secrets.reader))
	for _, values := range [][]string{{""}, {"two", "ids"}, {strings.Repeat("x", 65)}, {"contains spaces"}} {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		ctx = metadata.NewOutgoingContext(ctx, metadata.MD{"x-request-id": values})
		_, err := client.GetContact(ctx, &contactsv1.GetContactRequest{Id: 1})
		if status.Code(err) != codes.InvalidArgument {
			cancel()
			t.Fatalf("invalid request ID = %v", err)
		}
		stream, err := client.StreamContacts(ctx, &contactsv1.ListContactsRequest{})
		if err == nil {
			_, err = stream.Recv()
		}
		cancel()
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("stream accepted invalid request ID: %v", err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var header metadata.MD
	if _, err := client.GetContact(ctx, &contactsv1.GetContactRequest{Id: 1}, grpc.Header(&header)); err != nil {
		t.Fatal(err)
	}
	ids := header.Get("x-request-id")
	if len(ids) != 1 || ids[0] == "" {
		t.Fatalf("missing generated request ID: %v", header)
	}
}
