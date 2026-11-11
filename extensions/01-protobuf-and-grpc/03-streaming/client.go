package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// runClient imports sample data, receives a page, then exchanges lookups in both
// directions. The import runs first so the later read examples have data.
func runClient(parent context.Context, target string, out io.Writer) (err error) {
	// Bound the complete finite run, including connection work and every stream.
	// The channel is shared across calls; each helper also owns its stream cancel.
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	connection, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, connection.Close()) }()
	client := contactsv1.NewContactServiceClient(connection)
	suffix := " " + strconv.FormatInt(time.Now().UnixNano(), 36)
	inputs := []*contactsv1.CreateContactRequest{
		{Input: &contactsv1.ContactInput{Name: "Ana" + suffix}},
		{Input: &contactsv1.ContactInput{Name: "Bea" + suffix}},
		{Input: &contactsv1.ContactInput{Name: "Carla" + suffix}},
	}
	fmt.Fprintln(out, "== Client streaming: send a bounded batch, then receive its summary ==")
	summary, err := importBatch(ctx, client, inputs)
	if err != nil {
		return err
	}
	if summary.GetCreatedCount() != int32(len(inputs)) || len(summary.GetIds()) != len(inputs) {
		return fmt.Errorf("unexpected import summary: %v", summary)
	}
	fmt.Fprintf(out, "ImportContacts -> OK: created=%d, ids=%v\n", summary.GetCreatedCount(), summary.GetIds())
	fmt.Fprintln(out, "Each create commits immediately; failed imports can leave earlier items saved.")

	fmt.Fprintln(out, "\n== Server streaming: receive one ordered page until final EOF ==")
	count, err := receiveContacts(ctx, client, &contactsv1.ListContactsRequest{
		NameContains: suffix, Sort: contactsv1.SortOrder_SORT_ORDER_NAME_DESC, Limit: 2,
	}, out)
	if err != nil {
		return err
	}
	if count != 2 {
		return fmt.Errorf("streamed page has %d contacts, want 2", count)
	}
	_, err = receiveContacts(ctx, client, &contactsv1.ListContactsRequest{Limit: -1}, io.Discard)
	if status.Code(err) != codes.InvalidArgument {
		return fmt.Errorf("invalid stream query: got %v, want InvalidArgument", err)
	}
	fmt.Fprintln(out, "Invalid stream query -> InvalidArgument (observed by Recv)")

	fmt.Fprintln(out, "\n== Bidirectional streaming: send and receive concurrently ==")
	// Mix existing, missing, and invalid IDs. Missing/invalid lookups are reply
	// data in this contract, so they do not stop the whole bidirectional RPC.
	requests := []*contactsv1.LookupContactsRequest{
		{RequestId: 1, Id: summary.Ids[0]},
		{RequestId: 2, Id: math.MaxInt64},
		{RequestId: 3, Id: 0},
		{RequestId: 4, Id: summary.Ids[2]},
	}
	if err := lookupContacts(ctx, client, requests, out); err != nil {
		return err
	}
	if err := cancelLookup(ctx, client, summary.Ids[0], out); err != nil {
		return err
	}
	// Successful completion removes only the contacts imported by this run.
	for _, id := range summary.GetIds() {
		if _, err := client.DeleteContact(ctx, &contactsv1.DeleteContactRequest{Id: id}); err != nil {
			return err
		}
	}
	return nil
}

// importBatch sends many requests and receives one summary. It does not make
// the server's per-contact creates into a database-style atomic transaction.
func importBatch(parent context.Context, client contactsv1.ContactServiceClient, requests []*contactsv1.CreateContactRequest) (*contactsv1.ImportContactsResponse, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	stream, err := client.ImportContacts(ctx)
	if err != nil {
		return nil, err
	}
	for _, request := range requests {
		if err := stream.Send(request); err != nil {
			if err == io.EOF {
				break // The server ended early. Read its actual status below.
			}
			return nil, err
		}
	}
	// "Done sending" leaves the response unread. CloseAndRecv ends only our
	// request direction, then waits for the server's summary and final status.
	// Send returning nil alone cannot confirm that any create succeeded.
	return stream.CloseAndRecv()
}

// receiveContacts sends one query and drains responses through final EOF/error.
// Even if opening the stream succeeds, Recv can report query rejection later.
func receiveContacts(parent context.Context, client contactsv1.ContactServiceClient, query *contactsv1.ListContactsRequest, out io.Writer) (int, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	stream, err := client.StreamContacts(ctx, query)
	if err != nil {
		return 0, err
	}
	count := 0
	for {
		contact, err := stream.Recv()
		if err == io.EOF {
			// The server has no more responses and the RPC ended OK. A contact
			// received earlier could still have been followed by a different error.
			fmt.Fprintf(out, "StreamContacts -> EOF / OK: received=%d\n", count)
			return count, nil
		}
		if err != nil {
			return count, fmt.Errorf("receive contacts: %w", err)
		}
		count++
		fmt.Fprintf(out, "Contact id=%d, name=%q\n", contact.GetId(), contact.GetName())
	}
}

// lookupContacts gives one goroutine the send direction while this caller owns
// receiving. Every return path cancels the stream and joins that sender.
func lookupContacts(parent context.Context, client contactsv1.ContactServiceClient, requests []*contactsv1.LookupContactsRequest, out io.Writer) (err error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	stream, err := client.LookupContacts(ctx)
	if err != nil {
		return err
	}
	// The goroutine below sends requests; this function receives replies at the
	// same time. Each direction has one owner. This lets replies keep moving
	// even when sending must wait for transport capacity.
	sent := make(chan error, 1)
	go func() {
		for _, request := range requests {
			if err := stream.Send(request); err != nil {
				sent <- err
				return
			}
		}
		sent <- stream.CloseSend() // Done sending; keep receiving until EOF/error.
	}()
	defer func() {
		cancel()          // Release a blocked sender on any early receive/processing error.
		sendErr := <-sent // Always join the goroutine before returning.
		if err == nil && sendErr != nil && sendErr != io.EOF {
			err = fmt.Errorf("send lookups: %w", sendErr)
		}
	}()
	count := 0
	for {
		response, err := stream.Recv()
		if err == io.EOF {
			if count != len(requests) {
				return fmt.Errorf("lookup returned %d replies for %d requests", count, len(requests))
			}
			fmt.Fprintf(out, "LookupContacts -> EOF / OK: replies=%d\n", count)
			return nil
		}
		if err != nil {
			return fmt.Errorf("receive lookups: %w", err)
		}
		if count >= len(requests) || response.GetRequestId() != requests[count].GetRequestId() {
			return fmt.Errorf("unexpected lookup correlation: %v", response)
		}
		count++
		// A oneof has at most one selected branch; check the actual branch type.
		switch result := response.Result.(type) {
		case *contactsv1.LookupContactsResponse_Contact:
			fmt.Fprintf(out, "request=%d -> contact=%q\n", response.GetRequestId(), result.Contact.GetName())
		case *contactsv1.LookupContactsResponse_Failure:
			fmt.Fprintf(out, "request=%d -> item failure: %s\n", response.GetRequestId(), result.Failure.GetCode())
		default:
			return fmt.Errorf("lookup reply has no outcome: %v", response)
		}
	}
}

// cancelLookup demonstrates cancellation after a request has reached the handler,
// rather than starting a call with an already-canceled context.
func cancelLookup(parent context.Context, client contactsv1.ContactServiceClient, id int64, out io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	stream, err := client.LookupContacts(ctx)
	if err != nil {
		return err
	}
	// One bounded request/reply proves the handler is active. Leave the request
	// direction open so the server waits for more work, then cancel the RPC.
	if err := stream.Send(&contactsv1.LookupContactsRequest{RequestId: 1, Id: id}); err != nil {
		return err
	}
	if _, err := stream.Recv(); err != nil {
		return err
	}
	cancel()
	_, err = stream.Recv()
	if status.Code(err) != codes.Canceled {
		return fmt.Errorf("cancel active lookup: got %v, want Canceled", err)
	}
	fmt.Fprintln(out, "Active lookup cancellation -> Canceled")
	return nil
}
