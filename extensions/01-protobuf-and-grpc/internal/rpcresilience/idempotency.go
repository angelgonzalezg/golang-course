// Package rpcresilience contains the stage-5 memory cache and bounded retry
// policies. Stage 6 reuses the retry/limit policies with durable SQL creation.
package rpcresilience

import (
	"context"
	"sync"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/rpcinfra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// KeyHeader carries the stage-5 unary-create deduplication key, separately from
// the request ID used for logging and the subject established by authentication.
const KeyHeader = "x-idempotency-key"

// The same key from two verified callers represents two separate operations.
type scopedKey struct{ subject, key string }
type createEntry struct {
	request *contactsv1.CreateContactRequest
	result  *contactsv1.Contact
	err     error
	done    chan struct{} // Closed when the first request finishes; duplicates wait here.
}

// CreateCache remembers the first request/result for each verified caller/key
// pair, including errors. Repeating that same request returns the saved result
// instead of creating another contact. A full cache rejects new keys and keeps
// its old records. All records disappear when this Go process exits.
type CreateCache struct {
	mu       sync.Mutex
	capacity int
	entries  map[scopedKey]*createEntry
}

// NewCreateCache limits the number of subject/key reservations, including
// in-flight requests and cached failures. It panics if capacity is nonpositive.
func NewCreateCache(capacity int) *CreateCache {
	if capacity < 1 {
		panic("idempotency cache capacity must be positive")
	}
	return &CreateCache{capacity: capacity, entries: make(map[scopedKey]*createEntry)}
}

func validKey(key string) bool {
	if len(key) < 1 || len(key) > 64 {
		return false
	}
	for _, c := range key {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// Unary requires a key on CreateContact and rejects keys on other unary methods.
// Install it after authorization so every replay still checks current permissions.
func (c *CreateCache) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		keys := md.Get(KeyHeader)
		if info.FullMethod != contactsv1.ContactService_CreateContact_FullMethodName {
			if len(keys) != 0 {
				return nil, status.Error(codes.InvalidArgument, "idempotency keys apply only to unary CreateContact")
			}
			return next(ctx, request)
		}
		// This stage's endpoint REQUIRES a key for every unary create. Earlier
		// endpoints do not install this interceptor and make no such promise.
		if len(keys) != 1 || !validKey(keys[0]) {
			return nil, status.Error(codes.InvalidArgument, "one 1-64 character ASCII idempotency key is required")
		}
		identity, ok := rpcinfra.IdentityFromContext(ctx)
		if !ok || identity.Subject == "" {
			return nil, status.Error(codes.Unauthenticated, "authenticate before applying idempotency")
		}
		input, ok := request.(*contactsv1.CreateContactRequest)
		if !ok || input == nil {
			return nil, status.Error(codes.InvalidArgument, "create request is required")
		}
		return c.do(ctx, identity.Subject, keys[0], input, func() (*contactsv1.Contact, error) {
			response, err := next(ctx, request)
			if err != nil {
				return nil, err
			}
			contact, ok := response.(*contactsv1.Contact)
			if !ok || contact == nil {
				return nil, status.Error(codes.Internal, "unexpected create response")
			}
			return contact, nil
		})
	}
}

func cloneContact(contact *contactsv1.Contact) *contactsv1.Contact {
	if contact == nil {
		return nil
	}
	return proto.Clone(contact).(*contactsv1.Contact)
}

// do finds or reserves one caller/key operation, runs its create outside the
// mutex, then publishes a saved outcome. Only the reserving caller runs create;
// equivalent callers wait, and every returned message is an independent clone.
func (c *CreateCache) do(ctx context.Context, subject, key string, request *contactsv1.CreateContactRequest, create func() (*contactsv1.Contact, error)) (*contactsv1.Contact, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	c.mu.Lock()
	index := scopedKey{subject, key}
	if existing, ok := c.entries[index]; ok {
		if !proto.Equal(existing.request, request) {
			c.mu.Unlock()
			return nil, status.Error(codes.FailedPrecondition, "idempotency key is already bound to another request")
		}
		c.mu.Unlock()
		// Another caller owns this operation. Wait for its saved result, or let
		// this waiting caller cancel without canceling the original request.
		select {
		case <-existing.done:
			return cloneContact(existing.result), existing.err
		case <-ctx.Done():
			return nil, status.FromContextError(ctx.Err()).Err()
		}
	}
	if len(c.entries) == c.capacity {
		c.mu.Unlock()
		return nil, status.Error(codes.ResourceExhausted, "idempotency cache is full; existing keys can still be replayed")
	}
	entry := &createEntry{request: proto.Clone(request).(*contactsv1.CreateContactRequest), done: make(chan struct{})}
	// Record the reservation before releasing the lock. A simultaneous request
	// with the same caller/key will find it and wait instead of running create.
	c.entries[index] = entry
	c.mu.Unlock()
	// Do not hold the cache mutex during store operations, and do not detach
	// from the caller's context. Cancellation does not undo a completed create.
	result, err := create()
	// Retain terminal errors too: rerunning after an ambiguous backend failure
	// could repeat a mutation that committed before the error was reported.
	entry.result, entry.err = cloneContact(result), err
	// Save the fields before closing done. A waiter that receives that closure
	// can safely read the completed fields; later replays receive it immediately.
	close(entry.done)
	return cloneContact(entry.result), entry.err
}

// RejectStreamingKeys prevents callers from assuming a key makes an import or
// lookup conversation replayable. Stream progress keeps its original semantics.
func RejectStreamingKeys() grpc.StreamServerInterceptor {
	return func(service any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
		md, _ := metadata.FromIncomingContext(stream.Context())
		if len(md.Get(KeyHeader)) != 0 {
			return status.Error(codes.InvalidArgument, "streaming RPCs do not support idempotency keys or replay")
		}
		return next(service, stream)
	}
}
