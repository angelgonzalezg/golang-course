package rpcresilience

import (
	"context"
	"math/rand/v2"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	MaxAttempts     = 3
	MaxMessageBytes = 1024 // Deliberately small to make transport limits visible.
	CallBudget      = 2 * time.Second
)

// Client exposes only reads and keyed creation as retryable operations. Other
// mutations and every stream use the generated client directly, without replay.
// The connection should disable native configured retries to avoid stacking
// application retries on another retry policy. Transparent retries still exist.
type Client struct {
	api contactsv1.ContactServiceClient
}

func NewClient(api contactsv1.ContactServiceClient) *Client { return &Client{api: api} }

func (c *Client) GetContact(ctx context.Context, request *contactsv1.GetContactRequest) (*contactsv1.Contact, error) {
	return retryUnary(ctx, func(ctx context.Context) (*contactsv1.Contact, error) { return c.api.GetContact(ctx, request) })
}

func (c *Client) ListContacts(ctx context.Context, request *contactsv1.ListContactsRequest) (*contactsv1.ListContactsResponse, error) {
	return retryUnary(ctx, func(ctx context.Context) (*contactsv1.ListContactsResponse, error) {
		return c.api.ListContacts(ctx, request)
	})
}

func (c *Client) CreateOnce(ctx context.Context, key string, request *contactsv1.CreateContactRequest) (*contactsv1.Contact, error) {
	if !validKey(key) {
		return nil, status.Error(codes.InvalidArgument, "a valid idempotency key is required before retrying creation")
	}
	// Copy outgoing metadata: preserve credentials/tracing and replace, rather
	// than append, the key. The caller must keep key AND payload across retries.
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(KeyHeader, key)
	ctx = metadata.NewOutgoingContext(ctx, md)
	return retryUnary(ctx, func(ctx context.Context) (*contactsv1.Contact, error) { return c.api.CreateContact(ctx, request) })
}

func retryUnary[T any](parent context.Context, invoke func(context.Context) (T, error)) (T, error) {
	// One deadline covers connection work, every attempt, and every backoff.
	// A fresh full budget per attempt could multiply the user's latency limit.
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
		// Exponential backoff: about 20ms, then 40ms, with +/-20% jitter so
		// many callers do not all retry together. Backoff is context-aware.
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
