package rpcresilience

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func createRequest(name string) *contactsv1.CreateContactRequest {
	return &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: name}}
}

func TestConcurrentDuplicatesCreateOnceAndOwnTheirResults(t *testing.T) {
	cache := NewCreateCache(10)
	var calls atomic.Int64
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan *contactsv1.Contact, 16)
	errors := make(chan error, 16)
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			<-start
			result, err := cache.do(ctx, "subject", "key", createRequest("Ana"), func() (*contactsv1.Contact, error) {
				calls.Add(1)
				return &contactsv1.Contact{Id: 1, Name: "Ana"}, nil
			})
			results <- result
			errors <- err
		})
	}
	close(start)
	workers.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for result := range results {
		if result.GetId() != 1 || result.GetName() != "Ana" {
			t.Fatalf("duplicate result = %v", result)
		}
		result.Name = "caller edit" // Must not edit the cache or another caller's copy.
	}
	result, err := cache.do(ctx, "subject", "key", createRequest("Ana"), func() (*contactsv1.Contact, error) {
		t.Fatal("replay called the handler")
		return nil, nil
	})
	if err != nil || result.GetName() != "Ana" || calls.Load() != 1 {
		t.Fatalf("replay = %v, %v; handler calls=%d", result, err, calls.Load())
	}
}

func TestKeysAreScopedAndBoundToPayloadWithoutEviction(t *testing.T) {
	cache := NewCreateCache(2)
	var calls int64
	create := func() (*contactsv1.Contact, error) { calls++; return &contactsv1.Contact{Id: calls, Name: "Ana"}, nil }
	first, err := cache.do(t.Context(), "subject-A", "same-key", createRequest("Ana"), create)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cache.do(t.Context(), "subject-B", "same-key", createRequest("Ana"), create)
	if err != nil || first.GetId() == second.GetId() {
		t.Fatalf("subject scopes were merged: %v, %v", second, err)
	}
	_, err = cache.do(t.Context(), "subject-A", "same-key", createRequest("Different"), create)
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("changed payload = %v", err)
	}
	_, err = cache.do(t.Context(), "subject-A", "new-key", createRequest("Ana"), create)
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("full cache = %v", err)
	}
	replay, err := cache.do(t.Context(), "subject-A", "same-key", createRequest("Ana"), create)
	if err != nil || replay.GetId() != first.GetId() || calls != 2 {
		t.Fatalf("full cache evicted a result: %v, %v; calls=%d", replay, err, calls)
	}
}

func TestTerminalErrorsAreReplayedWithoutRepeatingUnknownEffects(t *testing.T) {
	cache := NewCreateCache(1)
	calls := 0
	create := func() (*contactsv1.Contact, error) {
		calls++
		return nil, status.Error(codes.Unavailable, "ambiguous backend failure")
	}
	for range 2 {
		_, err := cache.do(t.Context(), "subject", "key", createRequest("Ana"), create)
		if status.Code(err) != codes.Unavailable {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("replayed an ambiguous handler effect %d times", calls)
	}
}

type waitingContext struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func (c *waitingContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Done()
}

func TestCanceledDuplicateWaitDoesNotCancelOriginalCreate(t *testing.T) {
	cache := NewCreateCache(1)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	leader := make(chan error, 1)
	go func() {
		_, err := cache.do(t.Context(), "subject", "key", createRequest("Ana"), func() (*contactsv1.Contact, error) {
			close(entered)
			<-release
			return &contactsv1.Contact{Id: 1, Name: "Ana"}, nil
		})
		leader <- err
	}()
	<-entered
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	waiting := &waitingContext{Context: ctx, entered: make(chan struct{})}
	duplicate := make(chan error, 1)
	go func() {
		_, err := cache.do(waiting, "subject", "key", createRequest("Ana"), func() (*contactsv1.Contact, error) {
			return nil, status.Error(codes.Internal, "duplicate must not run")
		})
		duplicate <- err
	}()
	<-waiting.entered // The duplicate reached the in-flight entry's wait.
	cancel()
	if err := <-duplicate; status.Code(err) != codes.Canceled {
		t.Fatalf("waiting duplicate = %v", err)
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-leader; err != nil {
		t.Fatalf("duplicate cancellation reached leader: %v", err)
	}
}
