package contacts

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestStoreLifecycleAndSnapshots(t *testing.T) {
	ctx := t.Context()
	var store Store // A zero-value store has the same contract as NewStore().
	empty, err := store.List(ctx)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list = %v, %v; want non-nil empty slice", empty, err)
	}
	ana, err := store.Create(ctx, Input{" Ana ", " ana@example.com ", " 555-0100 "})
	if err != nil || ana != (Contact{1, "Ana", "ana@example.com", "555-0100"}) {
		t.Fatalf("create = %+v, %v", ana, err)
	}
	bob, err := store.Create(ctx, Input{Name: "Bob"})
	if err != nil {
		t.Fatal(err)
	}
	list, err := store.List(ctx)
	if err != nil || !reflect.DeepEqual(list, []Contact{ana, bob}) {
		t.Fatalf("ordered list = %v, %v", list, err)
	}
	list[0].Name = "Edited snapshot"
	ana.Email = "edited@example.com"
	stored, err := store.Get(ctx, 1)
	if err != nil || stored.Name != "Ana" || stored.Email != "ana@example.com" {
		t.Fatalf("snapshot changed store: %+v, %v", stored, err)
	}
	if _, err := store.Create(ctx, Input{Name: " Ana "}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := store.Replace(ctx, bob.ID, Input{Name: "Ana"}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("conflicting replacement = %v", err)
	}
	unchanged, _ := store.Get(ctx, bob.ID)
	if unchanged != bob {
		t.Fatalf("rejected replacement changed contact: %+v", unchanged)
	}
	replaced, err := store.Replace(ctx, 1, Input{Name: "Ana Torres"})
	if err != nil || replaced != (Contact{ID: 1, Name: "Ana Torres"}) {
		t.Fatalf("full replacement = %+v, %v", replaced, err)
	}
	// The old name is available again; IDs continue increasing after deletion.
	newAna, err := store.Create(ctx, Input{Name: "Ana"})
	if err != nil || newAna.ID != 3 {
		t.Fatalf("old name reuse = %+v, %v", newAna, err)
	}
	if err := store.Delete(ctx, bob.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, bob.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
	newBob, err := store.Create(ctx, Input{Name: "Bob"})
	if err != nil || newBob.ID != 4 {
		t.Fatalf("deleted name reuse = %+v, %v", newBob, err)
	}
	for _, id := range []int64{bob.ID, 999} {
		if _, err := store.Get(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("get missing %d = %v", id, err)
		}
		if _, err := store.Replace(ctx, id, Input{Name: "Missing"}); !errors.Is(err, ErrNotFound) {
			t.Errorf("replace missing %d = %v", id, err)
		}
	}
	other := NewStore()
	first, err := other.Create(ctx, Input{Name: "Ana"})
	if err != nil || first.ID != 1 {
		t.Fatalf("independent store = %+v, %v", first, err)
	}
}

func TestValidationRejectsChanges(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input Input
		field string
	}{
		{"blank", Input{Name: " \t\n"}, "name"},
		{"long name", Input{Name: strings.Repeat("界", 81)}, "name"},
		{"long email", Input{Name: "Ana", Email: strings.Repeat("a", 255)}, "email"},
		{"long phone", Input{Name: "Ana", Phone: strings.Repeat("1", 41)}, "phone"},
		{"invalid UTF-8", Input{Name: "Ana", Email: "\xff"}, "email"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := NewStore()
			original, err := store.Create(t.Context(), Input{Name: "Original"})
			if err != nil {
				t.Fatal(err)
			}
			_, createErr := store.Create(t.Context(), tc.input)
			_, replaceErr := store.Replace(t.Context(), original.ID, tc.input)
			for _, err := range []error{createErr, replaceErr} {
				var validation *ValidationError
				if !errors.Is(err, ErrInvalidInput) || !errors.As(err, &validation) || validation.Field != tc.field {
					t.Errorf("validation error = %v; want field %s", err, tc.field)
				}
			}
			list, _ := store.List(t.Context())
			if !reflect.DeepEqual(list, []Contact{original}) {
				t.Fatalf("invalid input changed store: %v", list)
			}
		})
	}
	store := NewStore()
	for _, id := range []int64{0, -1} {
		_, getErr := store.Get(t.Context(), id)
		_, replaceErr := store.Replace(t.Context(), id, Input{Name: "Ana"})
		for _, err := range []error{getErr, replaceErr, store.Delete(t.Context(), id)} {
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("invalid ID %d = %v", id, err)
			}
		}
	}
	// Limits count runes rather than UTF-8 bytes. Name matching is case sensitive.
	if _, err := store.Create(t.Context(), Input{strings.Repeat("界", 80), strings.Repeat("é", 254), strings.Repeat("1", 40)}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Ana", "ana"} {
		if _, err := store.Create(t.Context(), Input{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIDExhaustionDoesNotWrapOrWrite(t *testing.T) {
	store := NewStore()
	store.lastID = math.MaxInt64 - 1
	last, err := store.Create(t.Context(), Input{Name: "Last"})
	if err != nil || last.ID != math.MaxInt64 {
		t.Fatalf("last ID = %+v, %v", last, err)
	}
	if _, err := store.Create(t.Context(), Input{Name: "Overflow"}); !errors.Is(err, ErrIDExhausted) {
		t.Fatalf("overflow = %v", err)
	}
	list, _ := store.List(t.Context())
	if !reflect.DeepEqual(list, []Contact{last}) {
		t.Fatalf("overflow changed store: %v", list)
	}
}

func TestCanceledOperationsDoNotWrite(t *testing.T) {
	store := NewStore()
	original, err := store.Create(t.Context(), Input{Name: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, createErr := store.Create(ctx, Input{Name: "Bob"})
	_, getErr := store.Get(ctx, original.ID)
	_, listErr := store.List(ctx)
	_, replaceErr := store.Replace(ctx, original.ID, Input{Name: "Changed"})
	for _, err := range []error{createErr, getErr, listErr, replaceErr, store.Delete(ctx, original.ID)} {
		if !errors.Is(err, context.Canceled) {
			t.Errorf("canceled operation = %v", err)
		}
	}
	list, _ := store.List(t.Context())
	if !reflect.DeepEqual(list, []Contact{original}) {
		t.Fatalf("canceled operations changed store: %v", list)
	}
}

type observedContext struct {
	context.Context
	once    sync.Once
	checked chan struct{}
}

func (c *observedContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.checked) })
	return err
}

func TestCancellationWhileWaitingForStore(t *testing.T) {
	store := NewStore()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	observed := &observedContext{Context: ctx, checked: make(chan struct{})}
	store.mu.Lock()
	done := make(chan error, 1)
	go func() {
		_, err := store.Create(observed, Input{Name: "Ana"})
		done <- err
	}()
	// The first check captured nil. Hold the lock until cancellation, so the
	// check after acquiring it is what prevents this write. No Sleep is needed.
	<-observed.checked
	cancel()
	store.mu.Unlock()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting create = %v", err)
	}
	list, _ := store.List(t.Context())
	if len(list) != 0 {
		t.Fatalf("waiting create wrote data: %v", list)
	}
}

func TestConcurrentCreatesEnforceUniqueness(t *testing.T) {
	store := NewStore()
	const workers = 24
	start := make(chan struct{})
	results := make(chan error, workers)
	for range workers {
		go func() {
			<-start
			_, err := store.Create(t.Context(), Input{Name: "Ana"})
			results <- err
		}()
	}
	close(start)
	created := 0
	for range workers {
		if err := <-results; err == nil {
			created++
		} else if !errors.Is(err, ErrNameTaken) {
			t.Errorf("concurrent create = %v", err)
		}
	}
	if created != 1 {
		t.Fatalf("created %d contacts; want exactly one", created)
	}
}

func TestConcurrentContactLifecycles(t *testing.T) {
	store := NewStore()
	var wg sync.WaitGroup
	for worker := range 24 {
		wg.Go(func() {
			name := fmt.Sprintf("Contact %d", worker)
			created, err := store.Create(t.Context(), Input{Name: name})
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := store.Replace(t.Context(), created.ID, Input{Name: name + " updated"}); err != nil {
				t.Error(err)
			}
			if _, err := store.Get(t.Context(), created.ID); err != nil {
				t.Error(err)
			}
			list, err := store.List(t.Context())
			if err != nil {
				t.Error(err)
			}
			for i := 1; i < len(list); i++ {
				if list[i-1].ID >= list[i].ID {
					t.Errorf("list is not ordered by unique ID: %v", list)
				}
			}
			if err := store.Delete(t.Context(), created.ID); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	list, _ := store.List(t.Context())
	if len(list) != 0 {
		t.Fatalf("contacts remain after completed lifecycles: %v", list)
	}
}
