package rpcresilience

import (
	"context"
	"math/rand/v2"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	// MaxAttempts includes the first RPC; it counts application attempts.
	MaxAttempts = 3
	// MaxMessageBytes is the small encoded-message budget, not a whole-stream limit.
	MaxMessageBytes = 1024
	// CallBudget covers all attempts and backoffs; an earlier caller deadline wins.
	CallBudget = 2 * time.Second
)

// Client exposes only reads and keyed creation as retryable operations. Other
// mutations and every stream use the generated client directly, without replay.
// The connection should disable native configured retries to avoid stacking
// application retries on another retry policy. Transparent retries still exist.
type Client struct {
	api contactsv1.ContactServiceClient
}

// NewClient wraps an existing generated client. It owns no connection; the caller
// must close the underlying channel after its calls finish.
func NewClient(api contactsv1.ContactServiceClient) *Client { return &Client{api: api} }

// GetContact retries Unavailable within one budget; a later read may see newer data.
func (c *Client) GetContact(ctx context.Context, request *contactsv1.GetContactRequest) (*contactsv1.Contact, error) {
	return retryUnary(ctx, func(ctx context.Context) (*contactsv1.Contact, error) { return c.api.GetContact(ctx, request) })
}

// ListContacts retries the same page query, without freezing a snapshot across attempts.
func (c *Client) ListContacts(ctx context.Context, request *contactsv1.ListContactsRequest) (*contactsv1.ListContactsResponse, error) {
	return retryUnary(ctx, func(ctx context.Context) (*contactsv1.ListContactsResponse, error) {
		return c.api.ListContacts(ctx, request)
	})
}

// CreateOnce uses a key to label one intended create operation. If its reply is
// lost, the same key/data asks the server for the saved result. This requires
// a server policy after authorization: stage 5 uses CreateCache, while stage 6
// commits the contact/result together in PostgreSQL. A key alone supplies no guarantee.
func (c *Client) CreateOnce(ctx context.Context, key string, request *contactsv1.CreateContactRequest, options ...grpc.CallOption) (*contactsv1.Contact, error) {
	if !validKey(key) {
		return nil, status.Error(codes.InvalidArgument, "a valid idempotency key is required before retrying creation")
	}
	// Keep the caller's metadata unchanged. The key stays the same for every
	// attempt, and the payload (request fields) must stay the same too.
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(KeyHeader, key)
	ctx = metadata.NewOutgoingContext(ctx, md)
	// Options apply to each sequential attempt, including trailer capture from
	// the final invocation. They do not add another application retry policy.
	return retryUnary(ctx, func(ctx context.Context) (*contactsv1.Contact, error) {
		return c.api.CreateContact(ctx, request, options...)
	})
}

func retryUnary[T any](parent context.Context, invoke func(context.Context) (T, error)) (T, error) {
	// Set one deadline for the whole operation, including waits between tries.
	// Giving every attempt a new full budget could exceed the caller's limit.
	ctx, cancel := context.WithTimeout(parent, CallBudget)
	defer cancel()
	var zero T
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, status.FromContextError(err).Err()
		}
		result, err := invoke(ctx)
		if err == nil || status.Code(err) != codes.Unavailable || attempt == MaxAttempts {
			return result, err
		}
		// Wait before trying again (backoff): about 20ms, then 40ms. The shift
		// doubles the base wait; a small random change (jitter) helps callers
		// avoid retrying together. Cancellation can interrupt the wait.
		delay := time.Duration(float64(20*time.Millisecond<<uint(attempt-1)) * (0.8 + rand.Float64()*0.4))
		if err := wait(ctx, delay); err != nil {
			return zero, status.FromContextError(err).Err()
		}
	}
	panic("unreachable retry loop")
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
