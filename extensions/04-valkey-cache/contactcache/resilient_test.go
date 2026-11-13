package contactcache_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"example.com/course/cache-lab/cache"
	"example.com/course/cache-lab/contactcache"
	"example.com/course/cache-lab/internal/cachecontrol"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// The unit cache models thread-safe string storage, not network/TTL behavior.
// Those contracts run against real Valkey in the tagged integration suite.
type memoryRemote struct {
	mu     sync.Mutex
	values map[string]string
}

func (m *memoryRemote) Get(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.values[key]
	if !ok {
		return "", cache.ErrMiss
	}
	return value, nil
}
func (m *memoryRemote) Set(ctx context.Context, key, value string, _ time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = value
	return nil
}
func (m *memoryRemote) Delete(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.values[key]
	delete(m.values, key)
	return ok, nil
}

type resFixture struct {
	store   *contactcache.ResilientStore
	source  *cachecontrol.HeldSource
	remote  *memoryRemote
	faults  *cachecontrol.FaultCache
	probe   *cachecontrol.Probe
	contact contacts.Contact
}

func newResFixture(t *testing.T) resFixture {
	t.Helper()
	source := &cachecontrol.HeldSource{Source: contacts.NewStore()}
	remote := &memoryRemote{values: make(map[string]string)}
	faults := &cachecontrol.FaultCache{Cache: remote}
	probe := &cachecontrol.Probe{}
	store, err := contactcache.NewResilientStore(t.Context(), source, faults, "unit-resilient", contactcache.ResilienceOptions{
		TTL: time.Minute, CacheTimeout: 20 * time.Millisecond, LoadTimeout: time.Second, Observe: probe.Observe,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	contact, err := store.Create(t.Context(), contacts.Input{Name: "Ana", Email: "ana@example.com", Phone: "old"})
	if err != nil {
		t.Fatal(err)
	}
	return resFixture{store, source, remote, faults, probe, contact}
}

type readResult struct {
	contact contacts.Contact
	err     error
}

func asyncGet(ctx context.Context, store *contactcache.ResilientStore, id int64) <-chan readResult {
	result := make(chan readResult, 1)
	go func() { value, err := store.Get(ctx, id); result <- readResult{value, err} }()
	return result
}

func waitFor[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	value, err := cachecontrol.Wait(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func checkRead(t *testing.T, result readResult, want contacts.Contact) {
	t.Helper()
	if result.err != nil || result.contact != want {
		t.Fatalf("read=(%+v,%v), want %+v", result.contact, result.err, want)
	}
}

// A cold lookup reads the source once. A later hit, including all optional
// fields, leaves the source count unchanged and does not renew the entry.
func TestResilientColdAndWarm(t *testing.T) {
	f := newResFixture(t)
	checkRead(t, waitFor(t, asyncGet(t.Context(), f.store, f.contact.ID)), f.contact)
	checkRead(t, waitFor(t, asyncGet(t.Context(), f.store, f.contact.ID)), f.contact)
	if f.source.Reads.Load() != 1 || f.probe.Count("cache_hit") != 1 || f.probe.Count("cache_fill") != 1 {
		t.Fatal("warm lookup reached source or refilled")
	}
}

// Suppressed GET/SET/payload failures remain observable while a valid SQL/source
// result is returned. Source failures and missing contacts still reach callers.
func TestResilientFallback(t *testing.T) {
	for _, failure := range []string{"read", "fill", "payload"} {
		t.Run(failure, func(t *testing.T) {
			f := newResFixture(t)
			key, _ := f.store.Key(f.contact.ID)
			switch failure {
			case "read":
				f.faults.FailGet.Store(true)
			case "fill":
				f.faults.FailSet.Store(true)
			case "payload":
				if err := f.remote.Set(t.Context(), key, "{", time.Minute); err != nil {
					t.Fatal(err)
				}
			}
			checkRead(t, waitFor(t, asyncGet(t.Context(), f.store, f.contact.ID)), f.contact)
			operation := map[string]string{"read": "cache_read_error", "fill": "cache_fill_error", "payload": "cache_payload_error"}[failure]
			if f.probe.Count(operation) != 1 || f.source.Reads.Load() != 1 {
				t.Fatalf("missing diagnostic %s", operation)
			}
		})
	}
	t.Run("source not found is not cached", func(t *testing.T) {
		f := newResFixture(t)
		for range 2 {
			if _, err := f.store.Get(t.Context(), 99); !errors.Is(err, contacts.ErrNotFound) {
				t.Fatal(err)
			}
		}
		if f.source.Reads.Load() != 2 || f.probe.Count("cache_fill") != 0 {
			t.Fatal("negative result was cached")
		}
	})
}

// Hold the first snapshot until every caller has joined. Cancel the initiating
// caller; the remaining callers still share exactly one authoritative read.
func TestResilientSingleflightAndIndependentCancellation(t *testing.T) {
	f := newResFixture(t)
	hold := f.source.HoldNextGet()
	defer hold.Release()
	queued := f.probe.Watch("lookup_queued", 6)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first := asyncGet(ctx, f.store, f.contact.ID)
	waitFor(t, hold.Entered)
	others := make([]<-chan readResult, 5)
	for i := range others {
		others[i] = asyncGet(t.Context(), f.store, f.contact.ID)
	}
	for range 6 {
		waitFor(t, queued)
	}
	cancel()
	if result := waitFor(t, first); !errors.Is(result.err, context.Canceled) {
		t.Fatal(result.err)
	}
	if f.source.Reads.Load() != 1 {
		t.Fatal("duplicate SQL/source loads")
	}
	hold.Release()
	for _, result := range others {
		checkRead(t, waitFor(t, result), f.contact)
	}
	if f.source.Reads.Load() != 1 {
		t.Fatal("canceling one waiter restarted the shared load")
	}
}

// waitingCache holds GET until its command context expires.
type waitingCache struct{ contactcache.Cache }

func (c waitingCache) Get(ctx context.Context, _ string) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

// CacheTimeout is a sub-budget: it can expire while the overall shared load
// still has time for SQL. The test blocks on context, not a fixed sleep.
func TestResilientCacheTimeout(t *testing.T) {
	f := newResFixture(t)
	probe := &cachecontrol.Probe{}
	store, err := contactcache.NewResilientStore(t.Context(), f.source, waitingCache{f.remote}, "timeout", contactcache.ResilienceOptions{TTL: time.Minute, CacheTimeout: 10 * time.Millisecond, LoadTimeout: time.Second, Observe: probe.Observe})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	checkRead(t, waitFor(t, asyncGet(t.Context(), store, f.contact.ID)), f.contact)
	if probe.Count("cache_read_error") != 1 {
		t.Fatal("timeout was not observed")
	}
}

// A total load timeout must not become a successful fallback beyond its budget.
// The held source result observes cancellation and leaves no cache entry.
func TestResilientLoadTimeout(t *testing.T) {
	f := newResFixture(t)
	store, err := contactcache.NewResilientStore(t.Context(), f.source, f.remote, "load-timeout", contactcache.ResilienceOptions{TTL: time.Minute, CacheTimeout: 5 * time.Millisecond, LoadTimeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hold := f.source.HoldNextGet()
	defer hold.Release()
	result := waitFor(t, asyncGet(t.Context(), store, f.contact.ID))
	if !errors.Is(result.err, context.DeadlineExceeded) {
		t.Fatal(result.err)
	}
	key, _ := store.Key(f.contact.ID)
	if _, err := f.remote.Get(t.Context(), key); !errors.Is(err, cache.ErrMiss) {
		t.Fatal(err)
	}
}

// Shutdown cancels a live load, waits for it, and rejects subsequent operations.
// Canceling callers is not used as a substitute for owning shared-work cleanup.
func TestResilientClose(t *testing.T) {
	f := newResFixture(t)
	hold := f.source.HoldNextGet()
	defer hold.Release()
	result := asyncGet(t.Context(), f.store, f.contact.ID)
	waitFor(t, hold.Entered)
	f.store.Close()
	if got := waitFor(t, result); !errors.Is(got.err, contactcache.ErrClosed) {
		t.Fatal(got.err)
	}
	if _, err := f.store.Get(t.Context(), f.contact.ID); !errors.Is(err, contactcache.ErrClosed) {
		t.Fatal(err)
	}
	f.store.Close()
}

type failingRead struct {
	contactcache.Source
	failure error
}

func (s failingRead) Get(context.Context, int64) (contacts.Contact, error) {
	return contacts.Contact{}, s.failure
}

// Fallback must not hide a failed authoritative read behind a cache warning.
// Rejected writes must likewise keep their domain error without attempting DEL.
func TestResilientAuthoritativeErrors(t *testing.T) {
	t.Run("SQL cause survives cache failure", func(t *testing.T) {
		f := newResFixture(t)
		failure := errors.New("source unavailable")
		f.source.Source = failingRead{Source: f.source.Source, failure: failure}
		f.faults.FailGet.Store(true)
		if _, err := f.store.Get(t.Context(), f.contact.ID); !errors.Is(err, failure) {
			t.Fatal(err)
		}
		if f.probe.Count("cache_fill") != 0 {
			t.Fatal("failed source result was cached")
		}
	})
	t.Run("rejected write does not invalidate", func(t *testing.T) {
		f := newResFixture(t)
		checkRead(t, waitFor(t, asyncGet(t.Context(), f.store, f.contact.ID)), f.contact)
		f.faults.FailDelete.Store(true)
		blank := " "
		if _, err := f.store.Patch(t.Context(), f.contact.ID, contacts.Changes{Name: &blank}); !errors.Is(err, contacts.ErrInvalidInput) {
			t.Fatal(err)
		}
		if f.probe.Count("invalidation_error") != 0 {
			t.Fatal("rejected mutation attempted DEL")
		}
		checkRead(t, waitFor(t, asyncGet(t.Context(), f.store, f.contact.ID)), f.contact)
	})
}
