package main

import (
	"context"
	"sync"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/rpcresilience"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func request(name string) *contactsv1.CreateContactRequest {
	return &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: name}}
}

func TestLostReplyAndConcurrentRetriesCreateOnlyOneContact(t *testing.T) {
	faults := new(lessonFaults)
	faults.dropCreate.Store(true)
	env := setup(t, faults, 16)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var workers sync.WaitGroup
	errors := make(chan error, 12)
	ids := make(chan int64, 12)
	for range 12 {
		workers.Go(func() {
			result, err := env.client.CreateOnce(ctx, "shared-key", request("Ana"))
			errors <- err
			ids <- result.GetId()
		})
	}
	workers.Wait()
	close(errors)
	close(ids)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for id := range ids {
		if id != 2 {
			t.Fatalf("duplicate got ID %d, want 2", id)
		}
	}
	if env.store.creates.Load() != 1 {
		t.Fatalf("wire retries reached Create %d times", env.store.creates.Load())
	}
	_, err := env.client.CreateOnce(ctx, "shared-key", request("Different payload"))
	if status.Code(err) != codes.FailedPrecondition || env.store.creates.Load() != 1 {
		t.Fatalf("payload conflict = %v; creates=%d", err, env.store.creates.Load())
	}
}

func TestCacheCapacityAndKeyValidationOverTLS(t *testing.T) {
	env := setup(t, nil, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for _, keys := range [][]string{nil, {""}, {"first", "second"}, {"has space"}} {
		md := metadata.MD{rpcresilience.KeyHeader: keys}
		_, err := env.api.CreateContact(metadata.NewOutgoingContext(ctx, md), request("Ana"))
		if status.Code(err) != codes.InvalidArgument || env.store.creates.Load() != 0 {
			t.Fatalf("invalid key reached storage: %v", err)
		}
	}
	first, err := env.client.CreateOnce(ctx, "first", request("Ana"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.client.CreateOnce(ctx, "second", request("Bea"))
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("full cache = %v", err)
	}
	replay, err := env.client.CreateOnce(ctx, "first", request("Ana"))
	if err != nil || replay.GetId() != first.GetId() || env.store.creates.Load() != 1 {
		t.Fatalf("full-cache replay = %v, %v", replay, err)
	}
}

func TestKeysDoNotAuthorizeCallersOrMakeStreamsReplayable(t *testing.T) {
	env := setup(t, nil, 16)
	readerConnection, err := openConnection(env.target, env.transport, "reader-fixture", env.telemetry)
	if err != nil {
		t.Fatal(err)
	}
	defer readerConnection.Close()
	reader := rpcresilience.NewClient(contactsv1.NewContactServiceClient(readerConnection))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, err := reader.CreateOnce(ctx, "key", request("Ana")); status.Code(err) != codes.PermissionDenied || env.store.creates.Load() != 0 {
		t.Fatalf("key bypassed authorization: %v", err)
	}
	keyed := metadata.AppendToOutgoingContext(ctx, rpcresilience.KeyHeader, "key")
	if _, err := env.api.GetContact(keyed, &contactsv1.GetContactRequest{Id: 1}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("key attached to unsupported unary RPC = %v", err)
	}
	stream, err := env.api.ImportContacts(keyed)
	if err == nil {
		_, err = stream.CloseAndRecv()
	}
	if status.Code(err) != codes.InvalidArgument || env.store.creates.Load() != 0 {
		t.Fatalf("key silently accepted on import: %v", err)
	}
}
