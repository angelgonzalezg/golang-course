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

func runClient(ctx context.Context, target string, out io.Writer) (err error) {
	// This loopback lesson uses plaintext. TLS belongs to the infrastructure
	// stage. NewClient creates a reusable channel; connection I/O is lazy.
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
	updated, err := call(ctx, out, "UpdateContact: clear email", codes.OK, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.UpdateContact(ctx, &contactsv1.UpdateContactRequest{
			Id: ana.GetId(), Input: &contactsv1.ContactInput{},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"email"}}, ExpectedVersion: proto.String(ana.GetVersion()),
		})
	})
	if err != nil {
		return err
	}
	if _, err := call(ctx, out, "Stale update", codes.Aborted, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.UpdateContact(ctx, &contactsv1.UpdateContactRequest{
			Id: ana.GetId(), Input: &contactsv1.ContactInput{Phone: "stale"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"phone"}}, ExpectedVersion: proto.String(ana.GetVersion()),
		})
	}); err != nil {
		return err
	}
	replaced, err := call(ctx, out, "ReplaceContact: replace all fields", codes.OK, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.ReplaceContact(ctx, &contactsv1.ReplaceContactRequest{
			Id: ana.GetId(), Input: &contactsv1.ContactInput{Name: "Ana Updated" + suffix}, ExpectedVersion: proto.String(updated.GetVersion()),
		})
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "\n== Bounded pages and validation details ==")
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

func call[T proto.Message](parent context.Context, out io.Writer, label string, expected codes.Code, invoke func(context.Context) (T, error)) (T, error) {
	// Budgets belong to calls, not to channel construction. Cancel promptly
	// releases each call's timer; gRPC propagates its deadline to the server.
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
		for _, detail := range st.Details() {
			if bad, ok := detail.(*errdetails.BadRequest); ok {
				for _, field := range bad.GetFieldViolations() {
					fmt.Fprintf(out, "  %s: %s\n", field.GetField(), field.GetDescription())
				}
			}
		}
		return zero, nil // This demonstration expected the failure.
	}
	data, err := protojson.Marshal(response)
	if err != nil {
		return zero, err
	}
	fmt.Fprintln(out, string(data)) // Display only; the RPC used protobuf binary.
	return response, nil
}
