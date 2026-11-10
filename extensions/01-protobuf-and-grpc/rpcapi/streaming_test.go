package rpcapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestServerStreamPageAndTerminalStatus(t *testing.T) {
	store := contacts.NewStore()
	for _, name := range []string{"Ana", "Bea", "Carla", "David"} {
		if _, err := store.Create(t.Context(), contacts.Input{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	client := testClient(t, store, nil)
	for _, tc := range []struct {
		name  string
		query *contactsv1.ListContactsRequest
		want  []string
		code  codes.Code
	}{
		{"filtered ordered page", &contactsv1.ListContactsRequest{NameContains: "a", Sort: contactsv1.SortOrder_SORT_ORDER_NAME_DESC, Offset: 1, Limit: 2}, []string{"Carla", "Bea"}, codes.OK},
		{"empty page", &contactsv1.ListContactsRequest{Offset: 100}, nil, codes.OK},
		{"invalid limit", &contactsv1.ListContactsRequest{Limit: -1}, nil, codes.InvalidArgument},
		{"invalid enum", &contactsv1.ListContactsRequest{Sort: 99}, nil, codes.InvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			stream, err := client.StreamContacts(ctx, tc.query)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for {
				contact, err := stream.Recv()
				if err != nil {
					if tc.code == codes.OK && err != io.EOF || tc.code != codes.OK && status.Code(err) != tc.code {
						t.Fatalf("terminal status = %v, want %s", err, tc.code)
					}
					break
				}
				names = append(names, contact.GetName())
			}
			if !slices.Equal(names, tc.want) {
				t.Fatalf("page = %v, want %v", names, tc.want)
			}
		})
	}
}

type gatedContactStream struct {
	grpc.ServerStream
	ctx  context.Context
	send func(*contactsv1.Contact) error
}

func (s gatedContactStream) Context() context.Context             { return s.ctx }
func (s gatedContactStream) Send(value *contactsv1.Contact) error { return s.send(value) }

func TestServerStreamSnapshotAndBlockedSend(t *testing.T) {
	store := contacts.NewStore()
	for _, name := range []string{"Ana", "Bea"} {
		if _, err := store.Create(t.Context(), contacts.Input{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	entered := make(chan *contactsv1.Contact, 2)
	permit := make(chan struct{})
	stream := gatedContactStream{ctx: ctx, send: func(value *contactsv1.Contact) error {
		entered <- value
		select {
		case <-permit:
			return nil
		case <-ctx.Done():
			return status.FromContextError(ctx.Err()).Err()
		}
	}}
	done := make(chan struct{})
	var streamErr error
	go func() {
		defer close(done)
		streamErr = New(store, nil).StreamContacts(&contactsv1.ListContactsRequest{}, stream)
	}()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case first := <-entered:
		if first.GetName() != "Ana" {
			t.Fatalf("first = %v", first)
		}
	case <-ctx.Done():
		t.Fatal("first Send did not start")
	}
	// Mutation completes while Send is blocked: no store lock spans network I/O.
	// It must not change the already selected snapshot's second contact.
	if _, err := store.ReplaceIf(ctx, 2, contacts.Input{Name: "Changed"}, contacts.Conditions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case extra := <-entered:
		t.Fatalf("produced another message before Send returned: %v", extra)
	default:
	}
	select {
	case permit <- struct{}{}:
	case <-ctx.Done():
		t.Fatal("could not release first Send")
	}
	select {
	case second := <-entered:
		if second.GetName() != "Bea" {
			t.Fatalf("snapshot changed: %v", second)
		}
	case <-ctx.Done():
		t.Fatal("second Send did not start")
	}
	cancel() // Model a Send blocked by transport flow control, then canceled.
	<-done
	if status.Code(streamErr) != codes.Canceled {
		t.Fatalf("blocked Send result = %v", streamErr)
	}
}

func importRequests(t *testing.T, client contactsv1.ContactServiceClient, requests []*contactsv1.CreateContactRequest) (*contactsv1.ImportContactsResponse, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stream, err := client.ImportContacts(ctx)
	if err != nil {
		return nil, err
	}
	for _, request := range requests {
		if err := stream.Send(request); err != nil {
			if err == io.EOF {
				break // Only CloseAndRecv tells us the server's terminal status.
			}
			return nil, err
		}
	}
	return stream.CloseAndRecv()
}

func TestImportSummaryAndEmptyBatch(t *testing.T) {
	store := contacts.NewStore()
	client := testClient(t, store, nil)
	for _, names := range [][]string{nil, {"Ana", "Bea", "Carla"}} {
		var requests []*contactsv1.CreateContactRequest
		for _, name := range names {
			requests = append(requests, &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: name}})
		}
		summary, err := importRequests(t, client, requests)
		if err != nil || summary.GetCreatedCount() != int32(len(names)) || len(summary.GetIds()) != len(names) {
			t.Fatalf("summary = %v, %v", summary, err)
		}
		for i, id := range summary.GetIds() {
			contact, err := store.Get(t.Context(), id)
			if err != nil || contact.Name != names[i] {
				t.Fatalf("imported item %d = %+v, %v", i, contact, err)
			}
		}
	}
}

func TestImportStopsAtFailureWithoutRollback(t *testing.T) {
	for _, tc := range []struct {
		name string
		bad  *contactsv1.ContactInput
		code codes.Code
	}{
		{"invalid", &contactsv1.ContactInput{Name: " "}, codes.InvalidArgument},
		{"missing input", nil, codes.InvalidArgument},
		{"duplicate", &contactsv1.ContactInput{Name: "Ana"}, codes.AlreadyExists},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := contacts.NewStore()
			client := testClient(t, store, nil)
			summary, err := importRequests(t, client, []*contactsv1.CreateContactRequest{
				{Input: &contactsv1.ContactInput{Name: "Ana"}},
				{Input: tc.bad},
				{Input: &contactsv1.ContactInput{Name: "Must not be created"}},
			})
			if status.Code(err) != tc.code || summary != nil {
				t.Fatalf("failed batch = %v, %v", summary, err)
			}
			page, err := store.Search(t.Context(), contacts.Query{Limit: 100})
			if err != nil || page.Total != 1 || page.Contacts[0].Name != "Ana" {
				t.Fatalf("partial commits = %+v, %v", page, err)
			}
		})
	}
}

func TestImportBatchLimit(t *testing.T) {
	store := contacts.NewStore()
	client := testClient(t, store, nil)
	requests := make([]*contactsv1.CreateContactRequest, MaxImportContacts+1)
	for i := range requests {
		requests[i] = &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: fmt.Sprintf("Contact %03d", i)}}
	}
	summary, err := importRequests(t, client, requests)
	if status.Code(err) != codes.ResourceExhausted || summary != nil {
		t.Fatalf("oversized batch = %v, %v", summary, err)
	}
	page, err := store.Search(t.Context(), contacts.Query{Limit: 100})
	if err != nil || page.Total != MaxImportContacts {
		t.Fatalf("batch cap did not stop before creating item 101: %+v, %v", page, err)
	}
}

func TestBidiRepliesBeforeHalfCloseAndContinuesAfterItemFailure(t *testing.T) {
	store := contacts.NewStore()
	created, err := store.Create(t.Context(), contacts.Input{Name: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	client := testClient(t, store, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stream, err := client.LookupContacts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []int64{created.ID, 99, 0, created.ID} {
		requestID := uint64(i + 10)
		if err := stream.Send(&contactsv1.LookupContactsRequest{RequestId: requestID, Id: id}); err != nil {
			t.Fatal(err)
		}
		// Reading each bounded reply before CloseSend proves this is a live
		// conversation, not a server that buffers requests until client EOF.
		response, err := stream.Recv()
		if err != nil || response.GetRequestId() != requestID {
			t.Fatalf("reply %d = %v, %v", i, response, err)
		}
		switch id {
		case created.ID:
			if response.GetContact().GetName() != "Ana" || response.GetFailure() != nil {
				t.Fatalf("contact outcome = %v", response)
			}
		case 99, 0:
			want := contactsv1.LookupFailureCode_LOOKUP_FAILURE_CODE_NOT_FOUND
			if id == 0 {
				want = contactsv1.LookupFailureCode_LOOKUP_FAILURE_CODE_INVALID_ARGUMENT
			}
			if response.GetFailure().GetCode() != want || response.GetFailure().GetMessage() == "" || response.GetContact() != nil {
				t.Fatalf("failure outcome = %v", response)
			}
		}
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("terminal status = %v, want EOF / OK", err)
	}
	// An empty bidirectional conversation also completes successfully.
	empty, err := client.LookupContacts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := empty.CloseSend(); err != nil {
		t.Fatal(err)
	}
	if _, err := empty.Recv(); err != io.EOF {
		t.Fatalf("empty stream = %v", err)
	}
}

func TestBidiTerminalFailureAfterDeliveredReply(t *testing.T) {
	base := contacts.NewStore()
	if _, err := base.Create(t.Context(), contacts.Input{Name: "Ana"}); err != nil {
		t.Fatal(err)
	}
	store := getStore{Store: base, get: func(ctx context.Context, id int64) (contacts.Contact, error) {
		if id == 99 {
			return contacts.Contact{}, errors.New("private backend detail")
		}
		return base.Get(ctx, id)
	}}
	client := testClient(t, store, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stream, err := client.LookupContacts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&contactsv1.LookupContactsRequest{RequestId: 1, Id: 1}); err != nil {
		t.Fatal(err)
	}
	first, err := stream.Recv()
	if err != nil || first.GetContact().GetName() != "Ana" {
		t.Fatalf("first reply = %v, %v", first, err)
	}
	if err := stream.Send(&contactsv1.LookupContactsRequest{RequestId: 2, Id: 99}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	_, err = stream.Recv()
	if status.Code(err) != codes.Internal || strings.Contains(err.Error(), "private backend detail") {
		t.Fatalf("terminal failure must not become EOF, an item failure, or leak internals: %v", err)
	}
}
