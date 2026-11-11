package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// runClient follows create/read, selected updates, replacement, paging, and
// expected failures. Each call helper checks the RPC's outcome before continuing.
func runClient(ctx context.Context, target string, out io.Writer) (err error) {
	// Reuse one channel for the calls below. NewClient prepares that object;
	// the actual connection work starts when a call needs it. This loopback
	// lesson uses plaintext; TLS is introduced in the infrastructure stage.
	connection, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, connection.Close()) }()
	client := contactsv1.NewContactServiceClient(connection)
	// A unique namespace makes repeated -target runs independent. Successful
	// examples delete their contacts; interrupted remote runs can leave data.
	suffix := " " + strconv.FormatInt(time.Now().UnixNano(), 36)
	fmt.Fprintln(out, "== Create, retrieve, and inspect statuses ==")
	ana, err := call(ctx, out, "CreateContact", codes.OK, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.CreateContact(ctx, &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "Ana" + suffix, Email: "ana@example.com", Phone: "555-0100"}})
	})
	if err != nil {
		return err
	}
	bob, err := call(ctx, out, "CreateContact Bob", codes.OK, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.CreateContact(ctx, &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "Bob" + suffix}})
	})
	if err != nil {
		return err
	}
	if _, err := call(ctx, out, "Duplicate name", codes.AlreadyExists, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.CreateContact(ctx, &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: ana.GetName()}})
	}); err != nil {
		return err
	}
	if _, err := call(ctx, out, "GetContact", codes.OK, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.GetContact(ctx, &contactsv1.GetContactRequest{Id: ana.GetId()})
	}); err != nil {
		return err
	}
	fmt.Fprintln(out, "\n== Selected changes and optimistic concurrency ==")
	// Empty input gives email its zero value "". The mask selects that field,
	// so the service clears email while keeping the existing name and phone.
	updated, err := call(ctx, out, "UpdateContact: clear email", codes.OK, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.UpdateContact(ctx, &contactsv1.UpdateContactRequest{
			Id: ana.GetId(), Input: &contactsv1.ContactInput{},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"email"}}, ExpectedVersion: proto.String(ana.GetVersion()),
		})
	})
	if err != nil {
		return err
	}
	// ana still contains the version from before the email change. Reusing it
	// proves the server rejects a stale write instead of overwriting newer data.
	if _, err := call(ctx, out, "Stale update", codes.Aborted, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.UpdateContact(ctx, &contactsv1.UpdateContactRequest{
			Id: ana.GetId(), Input: &contactsv1.ContactInput{Phone: "stale"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"phone"}}, ExpectedVersion: proto.String(ana.GetVersion()),
		})
	}); err != nil {
		return err
	}
	// Replacement uses the updated version and supplies all editable fields.
	// Leaving email/phone at their zero values deliberately clears them.
	replaced, err := call(ctx, out, "ReplaceContact: replace all fields", codes.OK, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.ReplaceContact(ctx, &contactsv1.ReplaceContactRequest{
			Id: ana.GetId(), Input: &contactsv1.ContactInput{Name: "Ana Updated" + suffix}, ExpectedVersion: proto.String(updated.GetVersion()),
		})
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "\n== Bounded pages and validation details ==")
	// Follow server-provided offsets, filtering by this run's suffix so another
	// client's contacts do not change the demonstration's selected collection.
	var offset int64
	for {
		page, err := call(ctx, out, "ListContacts", codes.OK, func(ctx context.Context) (*contactsv1.ListContactsResponse, error) {
			return client.ListContacts(ctx, &contactsv1.ListContactsRequest{Limit: 1, Offset: offset, NameContains: suffix, Sort: contactsv1.SortOrder_SORT_ORDER_NAME_ASC})
		})
		if err != nil {
			return err
		}
		if page.NextOffset == nil {
			break
		}
		offset = page.GetNextOffset() // Presence, not zero, indicates another page.
	}
	if _, err := call(ctx, out, "Invalid ID", codes.InvalidArgument, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.GetContact(ctx, &contactsv1.GetContactRequest{})
	}); err != nil {
		return err
	}
	fmt.Fprintln(out, "\n== Caller cancellation and deadlines ==")
	// These two calls start with an already-ended context. In-flight propagation
	// is a separate scenario exercised by the service's context tests.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := call(canceled, out, "Pre-canceled call", codes.Canceled, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.GetContact(ctx, &contactsv1.GetContactRequest{Id: ana.GetId()})
	}); err != nil {
		return err
	}
	expired, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancel()
	if _, err := call(expired, out, "Expired deadline", codes.DeadlineExceeded, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.GetContact(ctx, &contactsv1.GetContactRequest{Id: ana.GetId()})
	}); err != nil {
		return err
	}
	fmt.Fprintln(out, "\n== Delete and close the shared channel ==")
	for _, contact := range []*contactsv1.Contact{replaced, bob} {
		if _, err := call(ctx, out, "DeleteContact", codes.OK, func(ctx context.Context) (*contactsv1.DeleteContactResponse, error) {
			return client.DeleteContact(ctx, &contactsv1.DeleteContactRequest{Id: contact.GetId(), ExpectedVersion: proto.String(contact.GetVersion())})
		}); err != nil {
			return err
		}
	}
	_, err = call(ctx, out, "Deleted contact", codes.NotFound, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.GetContact(ctx, &contactsv1.GetContactRequest{Id: ana.GetId()})
	})
	return err
}

// call is a teaching helper: T preserves each generated RPC's response type.
// expected makes intentional rejection cases part of a successful demo run.
func call[T proto.Message](parent context.Context, out io.Writer, label string, expected codes.Code, invoke func(context.Context) (T, error)) (T, error) {
	// Give this call up to two seconds, including any connection setup. The
	// server receives that deadline through context. cancel releases the timer
	// when we finish sooner; it does not undo a completed server-side change.
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	response, err := invoke(ctx)
	code := status.Code(err)
	fmt.Fprintf(out, "%s -> %s\n", label, code)
	var zero T
	if code != expected {
		if err != nil {
			return zero, fmt.Errorf("%s: want %s: %w", label, expected, err)
		}
		return zero, fmt.Errorf("%s: got %s, want %s", label, code, expected)
	}
	if err != nil {
		st := status.Convert(err)
		fmt.Fprintln(out, st.Message())
		// Status details carry structured field errors; callers need not parse
		// the wording of the human-readable status message to identify a field.
		for _, detail := range st.Details() {
			if bad, ok := detail.(*errdetails.BadRequest); ok {
				for _, field := range bad.GetFieldViolations() {
					fmt.Fprintf(out, "  %s: %s\n", field.GetField(), field.GetDescription())
				}
			}
		}
		// The generated call failed as expected. Return nil from this demo
		// helper so later examples still run; this does not turn the RPC into OK.
		return zero, nil
	}
	data, err := protojson.Marshal(response)
	if err != nil {
		return zero, err
	}
	fmt.Fprintln(out, string(data)) // Display only; the RPC used protobuf binary.
	return response, nil
}
