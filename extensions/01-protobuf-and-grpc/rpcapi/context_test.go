package rpcapi

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/httpapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type getStore struct {
	Store
	get func(context.Context, int64) (contacts.Contact, error)
}

func (s getStore) Get(ctx context.Context, id int64) (contacts.Contact, error) { return s.get(ctx, id) }

func TestInFlightContextCancellationAndDeadlineReachStore(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "cancellation"
		if deadline {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			base := contacts.NewStore()
			if _, err := base.Create(t.Context(), contacts.Input{Name: "Ana"}); err != nil {
				t.Fatal(err)
			}
			entered := make(chan bool, 1)
			finished := make(chan error, 1)
			store := getStore{Store: base, get: func(ctx context.Context, id int64) (contacts.Contact, error) {
				if id != 99 {
					return base.Get(ctx, id)
				}
				_, hasDeadline := ctx.Deadline()
				entered <- hasDeadline
				<-ctx.Done()
				finished <- ctx.Err()
				return contacts.Contact{}, ctx.Err()
			}}
			client := testClient(t, store, nil)
			warm, cancelWarm := context.WithTimeout(t.Context(), 5*time.Second)
			_, err := client.GetContact(warm, &contactsv1.GetContactRequest{Id: 1})
			cancelWarm()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(t.Context(), 500*time.Millisecond)
			}
			defer cancel()
			result := make(chan error, 1)
			go func() { _, err := client.GetContact(ctx, &contactsv1.GetContactRequest{Id: 99}); result <- err }()
			select {
			case hasDeadline := <-entered:
				if deadline && !hasDeadline {
					t.Fatal("client deadline did not reach the store")
				}
			case err := <-result:
				t.Fatalf("call ended before entering the store: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("call did not enter store")
			}
			if !deadline {
				cancel() // Explicit ordering, not a Sleep guessing when work started.
			}
			want := codes.Canceled
			if deadline {
				want = codes.DeadlineExceeded
			}
			if err := <-result; status.Code(err) != want {
				t.Fatalf("client result = %v, want %s", err, want)
			}
			select {
			case err := <-finished:
				if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("store context result = %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("store did not finish after caller cancellation/deadline")
			}
		})
	}
}

func TestUnexpectedStoreFailureIsLoggedAndRedacted(t *testing.T) {
	var logs bytes.Buffer
	store := getStore{get: func(context.Context, int64) (contacts.Contact, error) {
		return contacts.Contact{}, errors.New("private backend detail")
	}}
	client := testClient(t, store, slog.New(slog.NewTextHandler(&logs, nil)))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err := client.GetContact(ctx, &contactsv1.GetContactRequest{Id: 1})
	if status.Code(err) != codes.Internal || strings.Contains(err.Error(), "private backend detail") || !strings.Contains(logs.String(), "private backend detail") {
		t.Fatalf("unexpected error handling: error %v, logs %s", err, logs.String())
	}
}

func TestRESTAndRPCAdaptersCanShareOneStore(t *testing.T) {
	store := contacts.NewStore()
	client := testClient(t, store, nil)
	httpServer := httptest.NewServer(httpapi.NewHandler(store, nil))
	defer httpServer.Close()
	httpClient := httpServer.Client()
	httpClient.Timeout = 5 * time.Second
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	created, err := client.CreateContact(ctx, &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "Ana"}})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, "PATCH", httpServer.URL+"/contacts/1", strings.NewReader(`{"email":"rest@example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", httpapi.MergePatchMediaType)
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(resp.Body)
	if err := errors.Join(readErr, resp.Body.Close()); err != nil {
		t.Fatal(err)
	}
	var fromHTTP httpapi.ContactResource
	if resp.StatusCode != 200 || json.Unmarshal(data, &fromHTTP) != nil || fromHTTP.Email != "rest@example.com" {
		t.Fatalf("HTTP adapter response = %d, %s", resp.StatusCode, data)
	}
	fromRPC, err := client.GetContact(ctx, &contactsv1.GetContactRequest{Id: created.GetId()})
	if err != nil || fromRPC.GetEmail() != fromHTTP.Email || fromRPC.GetVersion() == created.GetVersion() {
		t.Fatalf("shared store was not observed: %v, %v", fromRPC, err)
	}
	_, err = client.UpdateContact(ctx, &contactsv1.UpdateContactRequest{
		Id: created.GetId(), Input: &contactsv1.ContactInput{Phone: "stale"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"phone"}}, ExpectedVersion: proto.String(created.GetVersion()),
	})
	if status.Code(err) != codes.Aborted {
		t.Fatalf("REST write must invalidate the earlier RPC version: %v", err)
	}
}

func TestNilRequestsAndAdditionalStatusMappings(t *testing.T) {
	service := New(contacts.NewStore(), nil)
	_, createErr := service.CreateContact(t.Context(), nil)
	_, getErr := service.GetContact(t.Context(), nil)
	_, listErr := service.ListContacts(t.Context(), nil)
	_, replaceErr := service.ReplaceContact(t.Context(), nil)
	_, updateErr := service.UpdateContact(t.Context(), nil)
	_, deleteErr := service.DeleteContact(t.Context(), nil)
	for _, err := range []error{createErr, getErr, listErr, replaceErr, updateErr, deleteErr} {
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("nil direct request = %v", err)
		}
	}
	for _, tc := range []struct {
		err  error
		code codes.Code
	}{
		{contacts.ErrIDExhausted, codes.ResourceExhausted},
		{context.Canceled, codes.Canceled},
		{context.DeadlineExceeded, codes.DeadlineExceeded},
	} {
		if got := status.Code(service.rpcError(t.Context(), tc.err)); got != tc.code {
			t.Errorf("error %v mapped to %s, want %s", tc.err, got, tc.code)
		}
	}
}
