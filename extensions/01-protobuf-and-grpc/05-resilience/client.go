package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/rpcobserve"
	"example.com/course/grpc-lab/internal/rpcresilience"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// expected checks an intentional outcome, including planned RPC failures. A
// simulated failure is a passed scenario only when it has the expected status.
func expected(label string, code codes.Code, err error, out io.Writer) error {
	if status.Code(err) != code {
		if err != nil {
			return fmt.Errorf("%s: want %s: %w", label, code, err)
		}
		return fmt.Errorf("%s: got OK, want %s", label, code)
	}
	fmt.Fprintf(out, "%s -> %s\n", label, code)
	return nil
}

// runClient compares safe reads, keyed creation, and four message boundaries.
// store/faults are local observation hooks for this in-process experiment; an
// ordinary remote client would not inspect the server's store or failure counters.
func runClient(parent context.Context, target string, transport *tls.Config, token string, telemetry *rpcobserve.Telemetry, store *contacts.Store, faults *lessonFaults, out io.Writer) (err error) {
	// This parent span groups the operation's RPCs. Passing its context lets
	// the configured instrumentation connect client and server observations.
	ctx, span := telemetry.Tracer.Tracer("course/resilience").Start(parent, "resilience-demo")
	defer span.End()
	connection, err := openConnection(target, transport, token, telemetry)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, connection.Close()) }()
	api := contactsv1.NewContactServiceClient(connection)
	// Use the wrapper for repeatable operations. Keep the raw generated client
	// for scenarios that intentionally bypass retries or change one call's limit.
	client := rpcresilience.NewClient(api)
	fmt.Fprintln(out, "== Bounded retries: only Unavailable, at most 3 application attempts ==")
	if _, err := client.GetContact(ctx, &contactsv1.GetContactRequest{Id: 1}); err != nil {
		return err
	}
	// The controlled server rejects two reads; the third must succeed. Inspect
	// the counter rather than assuming that a final OK proves retries happened.
	if faults.readCalls.Load() != 3 {
		return fmt.Errorf("read recovery used %d attempts", faults.readCalls.Load())
	}
	fmt.Fprintln(out, "Transient read -> OK after 3 attempts")
	_, err = client.GetContact(ctx, &contactsv1.GetContactRequest{})
	if err := expected("Invalid ID (one attempt)", codes.InvalidArgument, err, out); err != nil {
		return err
	}

	fmt.Fprintln(out, "\n== A lost create reply and an idempotent replay ==")
	// Reuse one key and unchanged request after a lost reply. The interceptor
	// remembers the original result, so two attempts produce only one contact.
	request := &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "Retry Contact"}}
	created, err := client.CreateOnce(ctx, "create-1", request)
	if err != nil {
		return err
	}
	if faults.createCalls.Load() != 2 {
		return fmt.Errorf("lost reply used %d create attempts", faults.createCalls.Load())
	}
	// The server began with 12 sample contacts. Check actual stored state to
	// prove the retry added one contact, rather than accepting duplicate work.
	page, err := store.Search(ctx, contacts.Query{Limit: 100})
	if err != nil || page.Total != 13 {
		return fmt.Errorf("idempotent create must add exactly one contact: total=%d, error=%v", page.Total, err)
	}
	fmt.Fprintf(out, "Lost create reply -> OK after 2 attempts; one contact added, id=%d\n", created.GetId())
	replayed, err := client.CreateOnce(ctx, "create-1", request)
	if err != nil || !proto.Equal(created, replayed) {
		return fmt.Errorf("replay changed the original result: %v", err)
	}
	fmt.Fprintln(out, "Same key and payload -> original result")
	// A key identifies one intended request, not permission to send different
	// data under the same label. That mismatch must be rejected without a write.
	_, err = client.CreateOnce(ctx, "create-1", &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "Different payload"}})
	if err := expected("Same key, different payload", codes.FailedPrecondition, err, out); err != nil {
		return err
	}
	_, err = api.CreateContact(ctx, request)
	if err := expected("Create without key", codes.InvalidArgument, err, out); err != nil {
		return err
	}

	fmt.Fprintln(out, "\n== Message limits apply to each Protobuf message ==")
	// Deliberately exceed the byte budget. The first boundary is client-side;
	// raising only that limit lets the next call reach the server's receive check.
	oversized := &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: strings.Repeat("x", 2*rpcresilience.MaxMessageBytes)}}
	keyed := metadata.AppendToOutgoingContext(ctx, rpcresilience.KeyHeader, "oversized")
	_, err = api.CreateContact(keyed, oversized)
	if err := expected("Client send limit", codes.ResourceExhausted, err, out); err != nil {
		return err
	}
	// Raise only the client's send limit, so this time the server rejects the
	// encoded request before the application's name validation can run.
	_, err = api.CreateContact(keyed, oversized, grpc.MaxCallSendMsgSize(4*rpcresilience.MaxMessageBytes))
	if err := expected("Server receive limit", codes.ResourceExhausted, err, out); err != nil {
		return err
	}
	// Reverse direction: one collection can exceed the server's send budget.
	// A single otherwise-valid contact can also exceed a smaller client budget.
	_, err = client.ListContacts(ctx, &contactsv1.ListContactsRequest{})
	if err := expected("Server send limit: large unary page", codes.ResourceExhausted, err, out); err != nil {
		return err
	}
	_, err = api.GetContact(ctx, &contactsv1.GetContactRequest{Id: 1}, grpc.MaxCallRecvMsgSize(64))
	if err := expected("Client receive limit", codes.ResourceExhausted, err, out); err != nil {
		return err
	}
	// Paging reduces one response's size. Streaming instead sends each contact
	// as its own bounded message; the limit is not a total budget for the stream.
	small, err := client.ListContacts(ctx, &contactsv1.ListContactsRequest{Limit: 2})
	if err != nil || len(small.GetContacts()) != 2 || small.NextOffset == nil {
		return fmt.Errorf("small page failed: %v", err)
	}
	fmt.Fprintln(out, "Smaller unary page -> OK with next_offset")
	stream, err := api.StreamContacts(ctx, &contactsv1.ListContactsRequest{Limit: 100})
	if err != nil {
		return err
	}
	count := 0
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		count++
	}
	if count != 13 {
		return fmt.Errorf("stream returned %d contacts, want 13", count)
	}
	fmt.Fprintln(out, "Stream -> EOF / OK: 13 small messages; total bytes may exceed one-message limit")

	fmt.Fprintln(out, "\n== Exhaustion and cancellation do not extend the call budget ==")
	// Keep failing longer than the retry cap. The wrapper must stop after three
	// application attempts instead of waiting for the server to eventually recover.
	before := faults.readCalls.Load()
	faults.readFailures.Store(99)
	_, err = client.GetContact(ctx, &contactsv1.GetContactRequest{Id: 1})
	if err := expected("Persistent outage", codes.Unavailable, err, out); err != nil {
		return err
	}
	if faults.readCalls.Load()-before != rpcresilience.MaxAttempts {
		return fmt.Errorf("retry cap was not respected")
	}
	faults.readFailures.Store(0)
	// An already-expired caller budget must end the operation before another
	// attempt begins. It does not receive a fresh two-second retry budget.
	expired, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	_, err = client.GetContact(expired, &contactsv1.GetContactRequest{Id: 1})
	cancel()
	return expected("Expired overall deadline", codes.DeadlineExceeded, err, out)
}
