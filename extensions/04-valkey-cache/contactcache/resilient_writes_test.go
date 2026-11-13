package contactcache_test

import (
	"context"
	"errors"
	"testing"

	"example.com/course/cache-lab/cache"
	"example.com/course/cache-lab/contactcache"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// A writer arriving after an old snapshot was read must wait for that local
// fill, then commit/invalidate it. A later lookup cannot reuse the old flight.
func TestResilientReadWriteOrdering(t *testing.T) {
	f := newResFixture(t)
	hold := f.source.HoldNextGet()
	defer hold.Release()
	reading := asyncGet(t.Context(), f.store, f.contact.ID)
	waitFor(t, hold.Entered)
	writeWait := f.probe.Watch("write_wait", 1)
	updated := make(chan readResult, 1)
	phone := "new"
	go func() {
		value, err := f.store.Patch(t.Context(), f.contact.ID, contacts.Changes{Phone: &phone})
		updated <- readResult{value, err}
	}()
	waitFor(t, writeWait)
	before, err := f.source.Source.Get(t.Context(), f.contact.ID)
	if err != nil || before != f.contact {
		t.Fatalf("writer passed held reader: %+v %v", before, err)
	}
	hold.Release()
	checkRead(t, waitFor(t, reading), f.contact)
	want := f.contact
	want.Phone = phone
	checkRead(t, waitFor(t, updated), want)
	key, _ := f.store.Key(want.ID)
	if _, err := f.remote.Get(t.Context(), key); !errors.Is(err, cache.ErrMiss) {
		t.Fatalf("old fill survived invalidation: %v", err)
	}
	checkRead(t, waitFor(t, asyncGet(t.Context(), f.store, want.ID)), want)
}

// Waiting on the local gate is cancelable. A canceled writer must not mutate
// the source after the reader releases its gate.
func TestResilientCanceledWriter(t *testing.T) {
	f := newResFixture(t)
	hold := f.source.HoldNextGet()
	defer hold.Release()
	reading := asyncGet(t.Context(), f.store, f.contact.ID)
	waitFor(t, hold.Entered)
	waiting := f.probe.Watch("write_wait", 1)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	writing := make(chan error, 1)
	go func() { writing <- f.store.Delete(ctx, f.contact.ID) }()
	waitFor(t, waiting)
	cancel()
	if err := waitFor(t, writing); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	hold.Release()
	checkRead(t, waitFor(t, reading), f.contact)
	if got, err := f.source.Source.Get(t.Context(), f.contact.ID); err != nil || got != f.contact {
		t.Fatalf("canceled deletion changed source: %+v %v", got, err)
	}
}

// Failed DEL is partial success, not a reason to retry SQL. The old cache can
// still be hit; retrying only Invalidate removes it and restores a fresh load.
func TestResilientInvalidationRepair(t *testing.T) {
	f := newResFixture(t)
	checkRead(t, waitFor(t, asyncGet(t.Context(), f.store, f.contact.ID)), f.contact)
	f.faults.FailDelete.Store(true)
	phone := "changed"
	updated, err := f.store.Patch(t.Context(), f.contact.ID, contacts.Changes{Phone: &phone})
	var committed *contactcache.InvalidationError
	if !errors.As(err, &committed) || updated.Phone != phone {
		t.Fatalf("partial success=(%+v,%v)", updated, err)
	}
	if got, err := f.source.Source.Get(t.Context(), f.contact.ID); err != nil || got != updated {
		t.Fatal("source write was not confirmed")
	}
	checkRead(t, waitFor(t, asyncGet(t.Context(), f.store, f.contact.ID)), f.contact)
	f.faults.FailDelete.Store(false)
	if err := f.store.Invalidate(t.Context(), f.contact.ID); err != nil {
		t.Fatal(err)
	}
	checkRead(t, waitFor(t, asyncGet(t.Context(), f.store, f.contact.ID)), updated)
}

type cancelAfterCommit struct {
	contactcache.Source
	cancel context.CancelFunc
}

func (s cancelAfterCommit) Replace(ctx context.Context, id int64, input contacts.Input) (contacts.Contact, error) {
	value, err := s.Source.Replace(ctx, id, input)
	if err == nil {
		s.cancel()
	}
	return value, err
}

// Cancellation after source success must not prevent the bounded post-commit
// DEL. A confirmed write can therefore return success even as its caller cancels.
func TestResilientPostCommitCancellation(t *testing.T) {
	f := newResFixture(t)
	checkRead(t, waitFor(t, asyncGet(t.Context(), f.store, f.contact.ID)), f.contact)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f.source.Source = cancelAfterCommit{Source: f.source.Source, cancel: cancel}
	updated, err := f.store.Replace(ctx, f.contact.ID, contacts.Input{Name: "Updated"})
	if err != nil || updated.Name != "Updated" || !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("post-commit cancellation=(%+v,%v)", updated, err)
	}
	key, _ := f.store.Key(f.contact.ID)
	if _, err := f.remote.Get(t.Context(), key); !errors.Is(err, cache.ErrMiss) {
		t.Fatal("cancellation prevented DEL")
	}
}
