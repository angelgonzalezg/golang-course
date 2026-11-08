package persistence_test

import (
	"context"
	"errors"
	"testing"

	"golangcourse/chapters/15-testing/01-unit-tests"
	"golangcourse/chapters/15-testing/02-test-doubles"
)

// A spy records observable interactions. Each test owns one; it is not shared
// by concurrent callers and does not pretend to implement durable storage.
type recordingStore struct {
	calls int
	label string
	ctx   context.Context
}

func (s *recordingStore) Put(ctx context.Context, label string) error {
	s.calls++
	s.label, s.ctx = label, ctx
	return nil
}

// A function adapter supplies a stubbed response without a mocking framework.
type storeFunc func(context.Context, string) error

func (f storeFunc) Put(ctx context.Context, label string) error {
	return f(ctx, label)
}

func TestSaveLabel(t *testing.T) {
	type requestKey struct{}
	ctx := context.WithValue(t.Context(), requestKey{}, "request-42")
	store := &recordingStore{}
	if err := persistence.SaveLabel(ctx, store, " GO "); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || store.label != "go" {
		t.Fatalf("store calls=%d label=%q, want one call with go", store.calls, store.label)
	}
	if store.ctx == nil || store.ctx.Value(requestKey{}) != "request-42" {
		t.Fatal("the store did not receive the caller's request metadata")
	}
}

func TestSaveLabelRejectsBeforeStorage(t *testing.T) {
	t.Run("invalid input", func(t *testing.T) {
		store := &recordingStore{}
		err := persistence.SaveLabel(t.Context(), store, " \t")
		if !errors.Is(err, labels.ErrInvalidLabel) || store.calls != 0 {
			t.Fatalf("error=%v calls=%d, want invalid label and no storage call", err, store.calls)
		}
	})
	t.Run("canceled request", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		store := &recordingStore{}
		err := persistence.SaveLabel(ctx, store, "go")
		if !errors.Is(err, context.Canceled) || store.calls != 0 {
			t.Fatalf("error=%v calls=%d, want cancellation and no storage call", err, store.calls)
		}
	})
}

func TestSaveLabelPreservesStoreError(t *testing.T) {
	want := errors.New("storage unavailable")
	store := storeFunc(func(context.Context, string) error { return want })
	if err := persistence.SaveLabel(t.Context(), store, "go"); !errors.Is(err, want) {
		t.Fatalf("error=%v, want the original storage cause", err)
	}
}
