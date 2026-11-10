package rpcapi

import (
	"context"
	"errors"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type searchStore struct {
	Store
	search func(context.Context, contacts.Query) (contacts.Page, error)
}

func (s searchStore) Search(ctx context.Context, query contacts.Query) (contacts.Page, error) {
	return s.search(ctx, query)
}

type createStore struct {
	Store
	create func(context.Context, contacts.Input) (contacts.Contact, error)
}

func (s createStore) Create(ctx context.Context, input contacts.Input) (contacts.Contact, error) {
	return s.create(ctx, input)
}

func TestStreamingInFlightCancellationAndDeadlines(t *testing.T) {
	for _, method := range []string{"server streaming", "client streaming", "bidirectional"} {
		for _, deadline := range []bool{false, true} {
			suffix := "/cancel"
			if deadline {
				suffix = "/deadline"
			}
			t.Run(method+suffix, func(t *testing.T) {
				base := contacts.NewStore()
				if _, err := base.Create(t.Context(), contacts.Input{Name: "Warm connection"}); err != nil {
					t.Fatal(err)
				}
				entered := make(chan bool, 1)
				finished := make(chan error, 1)
				wait := func(ctx context.Context) error {
					_, hasDeadline := ctx.Deadline()
					entered <- hasDeadline
					<-ctx.Done()
					finished <- ctx.Err()
					return ctx.Err()
				}
				var store Store
				var invoke func(context.Context, contactsv1.ContactServiceClient) error
				switch method {
				case "server streaming":
					store = searchStore{Store: base, search: func(ctx context.Context, _ contacts.Query) (contacts.Page, error) {
						return contacts.Page{}, wait(ctx)
					}}
					invoke = func(ctx context.Context, client contactsv1.ContactServiceClient) error {
						stream, err := client.StreamContacts(ctx, &contactsv1.ListContactsRequest{})
						if err != nil {
							return err
						}
						_, err = stream.Recv()
						return err
					}
				case "client streaming":
					store = createStore{Store: base, create: func(ctx context.Context, input contacts.Input) (contacts.Contact, error) {
						if input.Name == "Committed before cancellation" {
							return base.Create(ctx, input)
						}
						return contacts.Contact{}, wait(ctx)
					}}
					invoke = func(ctx context.Context, client contactsv1.ContactServiceClient) error {
						stream, err := client.ImportContacts(ctx)
						if err != nil {
							return err
						}
						if err := stream.Send(&contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "Committed before cancellation"}}); err != nil {
							return err
						}
						if err := stream.Send(&contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "Blocked"}}); err != nil {
							return err
						}
						_, err = stream.CloseAndRecv()
						return err
					}
				case "bidirectional":
					store = getStore{Store: base, get: func(ctx context.Context, id int64) (contacts.Contact, error) {
						if id == 99 {
							return contacts.Contact{}, wait(ctx)
						}
						return base.Get(ctx, id)
					}}
					invoke = func(ctx context.Context, client contactsv1.ContactServiceClient) error {
						stream, err := client.LookupContacts(ctx)
						if err != nil {
							return err
						}
						if err := stream.Send(&contactsv1.LookupContactsRequest{Id: 99}); err != nil {
							return err
						}
						_, err = stream.Recv()
						return err
					}
				}
				client := testClient(t, store, nil)
				warm, stopWarm := context.WithTimeout(t.Context(), 5*time.Second)
				_, err := client.GetContact(warm, &contactsv1.GetContactRequest{Id: 1})
				stopWarm()
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				if deadline {
					cancel()
					ctx, cancel = context.WithTimeout(t.Context(), 500*time.Millisecond)
				}
				defer cancel()
				result := make(chan error, 1)
				go func() { result <- invoke(ctx, client) }()
				select {
				case hasDeadline := <-entered:
					if !hasDeadline {
						t.Fatal("stream deadline did not reach the store")
					}
				case err := <-result:
					t.Fatalf("stream ended before entering the store: %v", err)
				case <-time.After(5 * time.Second):
					t.Fatal("stream did not enter the store")
				}
				want := codes.Canceled
				if deadline {
					want = codes.DeadlineExceeded
				} else {
					cancel()
				}
				select {
				case err := <-result:
					if status.Code(err) != want {
						t.Fatalf("stream result = %v, want %s", err, want)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("client did not return")
				}
				select {
				case err := <-finished:
					if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("store context = %v", err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("stream did not release store work")
				}
				if method == "client streaming" {
					page, err := base.Search(t.Context(), contacts.Query{Limit: 100})
					if err != nil || page.Total != 2 {
						t.Fatalf("cancellation must preserve the warm-up and first imported contact: %+v, %v", page, err)
					}
				}
			})
		}
	}
}
