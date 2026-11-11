package rpcresilience

import (
	"context"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// scriptedClient substitutes deterministic RPC outcomes for transport. Each
// test controls the relevant method so it can inspect retries and deadlines.
type scriptedClient struct {
	contactsv1.ContactServiceClient
	get    func(context.Context) (*contactsv1.Contact, error)
	create func(context.Context) (*contactsv1.Contact, error)
}

func (s scriptedClient) GetContact(ctx context.Context, _ *contactsv1.GetContactRequest, _ ...grpc.CallOption) (*contactsv1.Contact, error) {
	return s.get(ctx)
}
func (s scriptedClient) CreateContact(ctx context.Context, _ *contactsv1.CreateContactRequest, _ ...grpc.CallOption) (*contactsv1.Contact, error) {
	return s.create(ctx)
}

// Fail two reads, then succeed. Exactly three attempts must share one deadline;
// retrying must not grant each attempt a new full time budget.
func TestRetryRecoveryAndOneDeadlineAcrossAttempts(t *testing.T) {
	calls := 0
	var deadline time.Time
	client := NewClient(scriptedClient{get: func(ctx context.Context) (*contactsv1.Contact, error) {
		calls++
		current, ok := ctx.Deadline()
		if !ok {
			t.Fatal("attempt has no deadline")
		}
		if calls == 1 {
			deadline = current
		} else if !current.Equal(deadline) {
			t.Fatal("retry extended the call deadline")
		}
		if calls < 3 {
			return nil, status.Error(codes.Unavailable, "temporary")
		}
		return &contactsv1.Contact{Id: 1}, nil
	}})
	response, err := client.GetContact(t.Context(), &contactsv1.GetContactRequest{Id: 1})
	if err != nil || response.GetId() != 1 || calls != MaxAttempts {
		t.Fatalf("retry recovery = %v, %v; calls=%d", response, err, calls)
	}
}

// Return each listed failure status consistently. Only Unavailable is retried
// up to the cap; every other status must return after a single invocation.
func TestRetryExhaustionAndNonRetryableStatuses(t *testing.T) {
	for _, code := range []codes.Code{codes.Unavailable, codes.InvalidArgument, codes.PermissionDenied, codes.Unauthenticated, codes.ResourceExhausted, codes.Aborted, codes.FailedPrecondition, codes.Internal, codes.Canceled, codes.DeadlineExceeded} {
		t.Run(code.String(), func(t *testing.T) {
			calls := 0
			client := NewClient(scriptedClient{get: func(context.Context) (*contactsv1.Contact, error) {
				calls++
				return nil, status.Error(code, "failure")
			}})
			_, err := client.GetContact(t.Context(), &contactsv1.GetContactRequest{Id: 1})
			want := 1
			if code == codes.Unavailable {
				want = MaxAttempts
			}
			if status.Code(err) != code || calls != want {
				t.Fatalf("status=%v, calls=%d, want %d", err, calls, want)
			}
		})
	}
}

// Cancel as the first attempt returns a temporary failure, then try an expired
// call. Neither case may start another transport invocation.
func TestCancellationInterruptsBackoffAndExpiredBudgetNeverCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	client := NewClient(scriptedClient{get: func(context.Context) (*contactsv1.Contact, error) {
		calls++
		cancel() // The failure is returned after the parent canceled.
		return nil, status.Error(codes.Unavailable, "temporary")
	}})
	if _, err := client.GetContact(ctx, &contactsv1.GetContactRequest{Id: 1}); status.Code(err) != codes.Canceled || calls != 1 {
		t.Fatalf("cancellation: %v; calls=%d", err, calls)
	}
	expired, stop := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer stop()
	if _, err := client.GetContact(expired, &contactsv1.GetContactRequest{Id: 1}); status.Code(err) != codes.DeadlineExceeded || calls != 1 {
		t.Fatalf("expired budget called transport: %v; calls=%d", err, calls)
	}
}

// Reject an unkeyed create before transport, then retry a keyed create. Every
// attempt must retain the chosen key/request ID without editing the caller's metadata.
func TestCreateRetryRequiresAKeyAndDoesNotMutateCallerMetadata(t *testing.T) {
	calls := 0
	client := NewClient(scriptedClient{create: func(ctx context.Context) (*contactsv1.Contact, error) {
		calls++
		md, _ := metadata.FromOutgoingContext(ctx)
		keys := md.Get(KeyHeader)
		if len(keys) != 1 || keys[0] != "stable-key" || md.Get("x-request-id")[0] != "request-1" {
			t.Fatalf("retry metadata changed: %v", md)
		}
		if calls == 1 {
			return nil, status.Error(codes.Unavailable, "lost reply")
		}
		return &contactsv1.Contact{Id: 1}, nil
	}})
	md := metadata.Pairs(KeyHeader, "caller-key", "x-request-id", "request-1")
	ctx := metadata.NewOutgoingContext(t.Context(), md)
	if _, err := client.CreateOnce(ctx, "", createRequest("Ana")); status.Code(err) != codes.InvalidArgument || calls != 0 {
		t.Fatalf("unkeyed retry reached transport: %v", err)
	}
	if _, err := client.CreateOnce(ctx, "stable-key", createRequest("Ana")); err != nil || calls != 2 {
		t.Fatalf("keyed retry = %v; calls=%d", err, calls)
	}
	if md.Get(KeyHeader)[0] != "caller-key" {
		t.Fatal("retry mutated caller-owned metadata")
	}
}
