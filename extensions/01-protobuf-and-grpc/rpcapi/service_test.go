package rpcapi

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/rpcserver"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func testClient(t *testing.T, store Store, logger *slog.Logger) contactsv1.ContactServiceClient {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := rpcserver.Start(listener, New(store, logger))
	connection, err := grpc.NewClient("passthrough:///contacts-test",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeErr := connection.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := errors.Join(closeErr, server.Shutdown(ctx)); err != nil {
			t.Error(err)
		}
	})
	return contactsv1.NewContactServiceClient(connection)
}

func TestUnaryLifecycleAndSharedStoreValues(t *testing.T) {
	store := contacts.NewStore()
	client := testClient(t, store, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ana, err := client.CreateContact(ctx, &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: " Ana ", Email: " ana@example.com ", Phone: " 555-0100 "}})
	if err != nil || ana.GetName() != "Ana" || ana.GetEmail() != "ana@example.com" || ana.GetId() != 1 || ana.GetVersion() == "" {
		t.Fatalf("create = %v, %v", ana, err)
	}
	_, err = client.CreateContact(ctx, &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "Ana"}})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("duplicate = %v", err)
	}
	bob, err := client.CreateContact(ctx, &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "Bob"}})
	if err != nil {
		t.Fatal(err)
	}
	// The transport representation is independent of the domain's snapshots.
	stored, err := store.Get(ctx, ana.GetId())
	if err != nil || ana.GetVersion() != contacts.Version(stored) {
		t.Fatalf("domain/transport version mismatch: %+v, %v", stored, err)
	}
	snapshot := proto.Clone(ana).(*contactsv1.Contact)
	ana.Email = "edited client copy"
	fetched, err := client.GetContact(ctx, &contactsv1.GetContactRequest{Id: ana.GetId()})
	if err != nil || !proto.Equal(fetched, snapshot) {
		t.Fatalf("client mutation reached store: %v, %v", fetched, err)
	}
	// Unselected name (even a conflicting one) is ignored; selected empty
	// email clears it while the unselected phone stays unchanged.
	updated, err := client.UpdateContact(ctx, &contactsv1.UpdateContactRequest{
		Id: ana.GetId(), Input: &contactsv1.ContactInput{Name: "Bob", Email: ""},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"email"}}, ExpectedVersion: proto.String(snapshot.GetVersion()),
	})
	if err != nil || updated.GetName() != "Ana" || updated.GetEmail() != "" || updated.GetPhone() != "555-0100" || updated.GetVersion() == snapshot.GetVersion() {
		t.Fatalf("selected clearing = %v, %v", updated, err)
	}
	_, err = client.UpdateContact(ctx, &contactsv1.UpdateContactRequest{
		Id: ana.GetId(), Input: &contactsv1.ContactInput{Name: "Outdated", Phone: "stale"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "phone"}}, ExpectedVersion: proto.String(snapshot.GetVersion()),
	})
	if status.Code(err) != codes.Aborted {
		t.Fatalf("stale update = %v", err)
	}
	for _, tc := range []struct {
		input *contactsv1.ContactInput
		code  codes.Code
	}{
		{&contactsv1.ContactInput{Name: "Bob", Email: "changed@example.com"}, codes.AlreadyExists},
		{&contactsv1.ContactInput{Name: "", Email: "changed@example.com"}, codes.InvalidArgument},
	} {
		_, err := client.UpdateContact(ctx, &contactsv1.UpdateContactRequest{Id: ana.GetId(), Input: tc.input, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "email"}}})
		if status.Code(err) != tc.code {
			t.Fatalf("invalid/conflicting patch = %v", err)
		}
		got, err := client.GetContact(ctx, &contactsv1.GetContactRequest{Id: ana.GetId()})
		if err != nil || !proto.Equal(got, updated) {
			t.Fatalf("rejected multi-field patch was not atomic: %v, %v", got, err)
		}
	}
	replaced, err := client.ReplaceContact(ctx, &contactsv1.ReplaceContactRequest{Id: ana.GetId(), Input: &contactsv1.ContactInput{Name: "Ana Torres"}, ExpectedVersion: proto.String(updated.GetVersion())})
	if err != nil || replaced.GetId() != ana.GetId() || replaced.GetName() != "Ana Torres" || replaced.GetPhone() != "" || replaced.GetEmail() != "" {
		t.Fatalf("full replacement = %v, %v", replaced, err)
	}
	_, err = client.DeleteContact(ctx, &contactsv1.DeleteContactRequest{Id: ana.GetId(), ExpectedVersion: proto.String(updated.GetVersion())})
	if status.Code(err) != codes.Aborted {
		t.Fatalf("stale delete = %v", err)
	}
	if _, err := client.DeleteContact(ctx, &contactsv1.DeleteContactRequest{Id: ana.GetId(), ExpectedVersion: proto.String(replaced.GetVersion())}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetContact(ctx, &contactsv1.GetContactRequest{Id: ana.GetId()}); status.Code(err) != codes.NotFound {
		t.Fatalf("get deleted = %v", err)
	}
	if _, err := client.ReplaceContact(ctx, &contactsv1.ReplaceContactRequest{Id: 999, Input: &contactsv1.ContactInput{Name: "Missing"}}); status.Code(err) != codes.NotFound {
		t.Fatalf("replacement must not create: %v", err)
	}
	if _, err := client.DeleteContact(ctx, &contactsv1.DeleteContactRequest{Id: bob.GetId()}); err != nil {
		t.Fatal(err)
	}
}

func TestListDefaultsFilteringOrderingAndEmptyPages(t *testing.T) {
	store := contacts.NewStore()
	for _, name := range []string{"Zoë", "Ana", "ana", "Bob"} {
		if _, err := store.Create(t.Context(), contacts.Input{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	client := testClient(t, store, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	defaults, err := client.ListContacts(ctx, &contactsv1.ListContactsRequest{})
	if err != nil || defaults.GetLimit() != 20 || defaults.GetTotal() != 4 || defaults.NextOffset != nil || len(defaults.GetContacts()) != 4 || defaults.Contacts[0].GetId() != 1 {
		t.Fatalf("defaults = %v, %v", defaults, err)
	}
	first, err := client.ListContacts(ctx, &contactsv1.ListContactsRequest{Limit: 1, NameContains: " AN ", Sort: contactsv1.SortOrder_SORT_ORDER_NAME_DESC})
	if err != nil || first.GetTotal() != 2 || first.NextOffset == nil || first.GetNextOffset() != 1 || len(first.GetContacts()) != 1 || first.Contacts[0].GetId() != 2 {
		t.Fatalf("first filtered page = %v, %v", first, err)
	}
	last, err := client.ListContacts(ctx, &contactsv1.ListContactsRequest{Limit: 1, Offset: first.GetNextOffset(), NameContains: "AN", Sort: contactsv1.SortOrder_SORT_ORDER_NAME_DESC})
	if err != nil || last.NextOffset != nil || len(last.GetContacts()) != 1 || last.Contacts[0].GetId() != 3 {
		t.Fatalf("last filtered page = %v, %v", last, err)
	}
	for _, request := range []*contactsv1.ListContactsRequest{{Offset: 999}, {NameContains: "not present"}} {
		page, err := client.ListContacts(ctx, request)
		if err != nil || len(page.GetContacts()) != 0 || page.NextOffset != nil {
			t.Fatalf("empty page = %v, %v", page, err)
		}
	}
}

func TestInvalidRequestsHaveStructuredFieldDetails(t *testing.T) {
	store := contacts.NewStore()
	before, err := store.Create(t.Context(), contacts.Input{Name: "Ana", Email: "ana@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	client := testClient(t, store, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for _, tc := range []struct {
		name, field string
		invoke      func() error
	}{
		{"missing input", "input", func() error { _, err := client.CreateContact(ctx, &contactsv1.CreateContactRequest{}); return err }},
		{"blank name", "input.name", func() error {
			_, err := client.CreateContact(ctx, &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: " "}})
			return err
		}},
		{"zero ID", "id", func() error { _, err := client.GetContact(ctx, &contactsv1.GetContactRequest{}); return err }},
		{"missing replace input", "input", func() error {
			_, err := client.ReplaceContact(ctx, &contactsv1.ReplaceContactRequest{Id: 1})
			return err
		}},
		{"missing update input", "input", func() error { _, err := client.UpdateContact(ctx, &contactsv1.UpdateContactRequest{Id: 1}); return err }},
		{"missing mask", "update_mask", func() error {
			_, err := client.UpdateContact(ctx, &contactsv1.UpdateContactRequest{Id: 1, Input: &contactsv1.ContactInput{}})
			return err
		}},
		{"empty version", "expected_version", func() error {
			_, err := client.DeleteContact(ctx, &contactsv1.DeleteContactRequest{Id: 1, ExpectedVersion: proto.String("")})
			return err
		}},
		{"negative offset", "offset", func() error {
			_, err := client.ListContacts(ctx, &contactsv1.ListContactsRequest{Offset: -1})
			return err
		}},
		{"large limit", "limit", func() error {
			_, err := client.ListContacts(ctx, &contactsv1.ListContactsRequest{Limit: 101})
			return err
		}},
		{"unknown enum", "sort", func() error {
			_, err := client.ListContacts(ctx, &contactsv1.ListContactsRequest{Sort: 999})
			return err
		}},
		{"long filter", "name_contains", func() error {
			_, err := client.ListContacts(ctx, &contactsv1.ListContactsRequest{NameContains: strings.Repeat("a", 81)})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.invoke()
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("invalid request = %v", err)
			}
			details := status.Convert(err).Details()
			if len(details) != 1 {
				t.Fatalf("expected one structured detail: %v", details)
			}
			bad, ok := details[0].(*errdetails.BadRequest)
			if !ok || len(bad.GetFieldViolations()) != 1 || bad.FieldViolations[0].GetField() != tc.field {
				t.Fatalf("wrong field detail: %v", details)
			}
		})
	}
	for _, paths := range [][]string{{}, {"id"}, {"email", "email"}, {"*"}, {"email.value"}, {"Name"}} {
		_, err := client.UpdateContact(ctx, &contactsv1.UpdateContactRequest{Id: 1, Input: &contactsv1.ContactInput{Name: "Changed"}, UpdateMask: &fieldmaskpb.FieldMask{Paths: paths}})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("invalid mask %v = %v", paths, err)
		}
	}
	after, err := store.Get(ctx, before.ID)
	if err != nil || after != before {
		t.Fatalf("rejected requests changed contact: %+v, %v", after, err)
	}
}

func TestCompetingRPCUpdatesUseOneAtomicVersionCheck(t *testing.T) {
	store := contacts.NewStore()
	before, err := store.Create(t.Context(), contacts.Input{Name: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	client := testClient(t, store, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan codes.Code, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"Writer A", "Writer B"} {
		wg.Go(func() {
			<-start
			_, err := client.UpdateContact(ctx, &contactsv1.UpdateContactRequest{
				Id: before.ID, Input: &contactsv1.ContactInput{Name: name}, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}}, ExpectedVersion: proto.String(contacts.Version(before)),
			})
			results <- status.Code(err)
		})
	}
	close(start)
	wg.Wait()
	close(results)
	counts := map[codes.Code]int{}
	for code := range results {
		counts[code]++
	}
	if counts[codes.OK] != 1 || counts[codes.Aborted] != 1 {
		t.Fatalf("competing updates = %v", counts)
	}
}
