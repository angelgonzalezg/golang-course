package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/rpcresilience"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestClientAndServerReceiveRejectOversizedRequestsBeforeMutation(t *testing.T) {
	env := setup(t, nil, 16)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, rpcresilience.KeyHeader, "oversized")
	input := request(strings.Repeat("x", 2*rpcresilience.MaxMessageBytes))
	for _, options := range [][]grpc.CallOption{nil, {grpc.MaxCallSendMsgSize(4 * rpcresilience.MaxMessageBytes)}} {
		_, err := env.api.CreateContact(ctx, input, options...)
		if status.Code(err) != codes.ResourceExhausted || env.store.creates.Load() != 0 {
			t.Fatalf("oversized request = %v; creates=%d", err, env.store.creates.Load())
		}
	}
	// Rejection before dispatch must not consume even the key reservation.
	if _, err := env.client.CreateOnce(t.Context(), "oversized", request("Ana")); err != nil {
		t.Fatalf("transport rejection bound the idempotency key: %v", err)
	}
}

func TestUnaryResponseLimitsAndPerMessageStreaming(t *testing.T) {
	env := setup(t, nil, 16)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for i := range 11 {
		_, err := env.store.Store.Create(ctx, contacts.Input{Name: fmt.Sprintf("Contact %02d", i), Email: strings.Repeat("e", 110) + "@example.com"})
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", "large-page")
	if _, err := env.client.ListContacts(ctx, &contactsv1.ListContactsRequest{}); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("server send limit = %v", err)
	}
	if _, err := env.api.GetContact(ctx, &contactsv1.GetContactRequest{Id: 1}, grpc.MaxCallRecvMsgSize(64)); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("client receive limit = %v", err)
	}
	page, err := env.client.ListContacts(ctx, &contactsv1.ListContactsRequest{Limit: 2})
	if err != nil || len(page.GetContacts()) != 2 || page.NextOffset == nil {
		t.Fatalf("small page = %v, %v", page, err)
	}
	stream, err := env.api.StreamContacts(ctx, &contactsv1.ListContactsRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count != 12 {
		t.Fatalf("per-message limit incorrectly capped total stream bytes: count=%d", count)
	}
}

func TestStreamSizeFailureKeepsEarlierImportCommitsAndDeliveredReplies(t *testing.T) {
	env := setup(t, nil, 16)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stream, err := env.api.ImportContacts(ctx, grpc.MaxCallSendMsgSize(4*rpcresilience.MaxMessageBytes))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []*contactsv1.CreateContactRequest{request("Imported first"), request(strings.Repeat("x", 2*rpcresilience.MaxMessageBytes))} {
		if err := stream.Send(input); err != nil && err != io.EOF {
			t.Fatal(err)
		}
	}
	if _, err := stream.CloseAndRecv(); status.Code(err) != codes.ResourceExhausted || env.store.creates.Load() != 1 {
		t.Fatalf("failed import = %v; earlier creates=%d", err, env.store.creates.Load())
	}
	// A domain-valid Unicode contact can still exceed the byte limit. Seed it
	// directly to isolate response limits from RPC input limits in this fixture.
	if _, err := env.store.Store.Create(ctx, contacts.Input{Name: strings.Repeat("😀", 80), Email: strings.Repeat("😀", 254), Phone: strings.Repeat("😀", 40)}); err != nil {
		t.Fatal(err)
	}
	responses, err := env.api.StreamContacts(ctx, &contactsv1.ListContactsRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for {
		_, err := responses.Recv()
		if err != nil {
			if status.Code(err) != codes.ResourceExhausted || count != 2 {
				t.Fatalf("partial response stream = %v after %d replies", err, count)
			}
			break
		}
		count++
	}
}
