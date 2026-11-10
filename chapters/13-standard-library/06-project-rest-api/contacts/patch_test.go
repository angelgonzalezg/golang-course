package contacts

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func text(value string) *string { return &value }

func TestPatchSelectsFieldsAndKeepsID(t *testing.T) {
	store := NewStore()
	before, err := store.Create(t.Context(), Input{"Ana", "ana@example.com", "555-0100"})
	if err != nil {
		t.Fatal(err)
	}
	changes := Changes{Email: text(" new@example.com ")}
	updated, err := store.Patch(t.Context(), before.ID, changes)
	want := before
	want.Email = "new@example.com"
	if err != nil || updated != want {
		t.Fatalf("partial update = %+v, %v; want %+v", updated, err, want)
	}
	*changes.Email = "edited input"
	updated.Name = "edited snapshot"
	stored, err := store.Get(t.Context(), before.ID)
	if err != nil || stored != want {
		t.Fatalf("caller edited stored data: %+v, %v", stored, err)
	}
	for _, changes := range []Changes{{Email: text("new@example.com")}, {}} {
		got, err := store.Patch(t.Context(), before.ID, changes)
		if err != nil || got != want {
			t.Fatalf("repeated or empty patch = %+v, %v", got, err)
		}
	}
	cleared, err := store.Patch(t.Context(), before.ID, Changes{Phone: text("")})
	if err != nil || cleared.Phone != "" || cleared.Name != before.Name || cleared.Email != want.Email {
		t.Fatalf("clear optional field = %+v, %v", cleared, err)
	}
	renamed, err := store.Patch(t.Context(), before.ID, Changes{Name: text(" Ana Torres ")})
	if err != nil || renamed.ID != before.ID || renamed.Name != "Ana Torres" {
		t.Fatalf("rename = %+v, %v", renamed, err)
	}
	if _, err := store.Create(t.Context(), Input{Name: "Ana Torres"}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("new name was not indexed: %v", err)
	}
	if _, err := store.Create(t.Context(), Input{Name: "Ana"}); err != nil {
		t.Fatalf("old name was not released: %v", err)
	}
}

func TestRejectedPatchIsAtomic(t *testing.T) {
	store := NewStore()
	before, err := store.Create(t.Context(), Input{"Ana", "ana@example.com", "555-0100"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(t.Context(), Input{Name: "Bob"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		changes Changes
		err     error
	}{
		{"conflict", Changes{Name: text("Bob"), Email: text("changed@example.com")}, ErrNameTaken},
		{"blank name", Changes{Name: text(""), Phone: text("")}, ErrInvalidInput},
		{"long email", Changes{Name: text("Renamed"), Email: text(strings.Repeat("a", 255))}, ErrInvalidInput},
		{"invalid UTF-8", Changes{Name: text("Renamed"), Phone: text("\xff")}, ErrInvalidInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := store.Patch(t.Context(), before.ID, tc.changes); !errors.Is(err, tc.err) {
				t.Fatalf("patch error = %v, want %v", err, tc.err)
			}
			got, err := store.Get(t.Context(), before.ID)
			if err != nil || got != before {
				t.Fatalf("rejected patch changed contact: %+v, %v", got, err)
			}
		})
	}
	// A failed rename must also leave the original name index intact.
	if _, err := store.Create(t.Context(), Input{Name: before.Name}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("rejected patch damaged the name index: %v", err)
	}
	for _, id := range []int64{0, -1, 99} {
		_, err := store.Patch(t.Context(), id, Changes{Email: text("changed@example.com")})
		want := ErrInvalidInput
		if id == 99 {
			want = ErrNotFound
		}
		if !errors.Is(err, want) {
			t.Errorf("patch ID %d = %v, want %v", id, err, want)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.Patch(ctx, before.ID, Changes{Name: text("Changed")}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled patch = %v", err)
	}
	got, err := store.Get(t.Context(), before.ID)
	if err != nil || got != before {
		t.Fatalf("canceled patch changed contact: %+v, %v", got, err)
	}
}

func TestPatchCancellationWhileWaiting(t *testing.T) {
	store := NewStore()
	before, err := store.Create(t.Context(), Input{Name: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	observed := &observedContext{Context: ctx, checked: make(chan struct{})}
	store.mu.Lock()
	done := make(chan error, 1)
	go func() {
		_, err := store.Patch(observed, before.ID, Changes{Name: text("Changed")})
		done <- err
	}()
	<-observed.checked
	cancel()
	store.mu.Unlock()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting patch = %v", err)
	}
	got, err := store.Get(t.Context(), before.ID)
	if err != nil || got != before {
		t.Fatalf("waiting patch changed contact: %+v, %v", got, err)
	}
}

func TestConcurrentPatchesKeepUnselectedFields(t *testing.T) {
	store := NewStore()
	// Exercise simultaneous updates to different fields of the same contact.
	// Each pair must preserve both edits, regardless of lock acquisition order.
	for range 24 {
		before, err := store.Create(t.Context(), Input{Name: "Ana"})
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		for _, changes := range []Changes{{Email: text("new@example.com")}, {Phone: text("555-9999")}} {
			wg.Go(func() {
				<-start
				if _, err := store.Patch(t.Context(), before.ID, changes); err != nil {
					t.Error(err)
				}
			})
		}
		close(start)
		wg.Wait()
		got, err := store.Get(t.Context(), before.ID)
		if err != nil || got.Name != before.Name || got.Email != "new@example.com" || got.Phone != "555-9999" {
			t.Fatalf("concurrent patch lost an edit: %+v, %v", got, err)
		}
		if err := store.Delete(t.Context(), before.ID); err != nil {
			t.Fatal(err)
		}
	}
}
