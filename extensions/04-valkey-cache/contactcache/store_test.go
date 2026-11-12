package contactcache

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/course/cache-lab/cache"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// This small double models only the cache boundary. Expiration, networking,
// and real SQL behavior belong to the integration suite, not this map.
type testCache struct {
	values                    map[string]string
	getErr, setErr, deleteErr error
	gets, sets, deletes       int
	beforeDelete              func()
}

func (c *testCache) Get(_ context.Context, key string) (string, error) {
	c.gets++
	if c.getErr != nil {
		return "", c.getErr
	}
	value, ok := c.values[key]
	if !ok {
		return "", cache.ErrMiss
	}
	return value, nil
}

func (c *testCache) Set(_ context.Context, key, value string, _ time.Duration) error {
	c.sets++
	if c.setErr != nil {
		return c.setErr
	}
	c.values[key] = value
	return nil
}

func (c *testCache) Delete(_ context.Context, key string) (bool, error) {
	c.deletes++
	if c.beforeDelete != nil {
		c.beforeDelete()
	}
	if c.deleteErr != nil {
		return false, c.deleteErr
	}
	_, existed := c.values[key]
	delete(c.values, key)
	return existed, nil
}

type testSource struct {
	*contacts.Store
	reads   int
	readErr error
}

func (s *testSource) Get(ctx context.Context, id int64) (contacts.Contact, error) {
	s.reads++
	if s.readErr != nil {
		return contacts.Contact{}, s.readErr
	}
	return s.Store.Get(ctx, id)
}

func fixture(t *testing.T) (*Store, *testSource, *testCache, contacts.Contact) {
	t.Helper()
	source := &testSource{Store: contacts.NewStore()}
	remote := &testCache{values: make(map[string]string)}
	store, err := NewStore(source, remote, "unit", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	contact, err := store.Create(t.Context(), contacts.Input{Name: "Ana", Email: "ana@example.com", Phone: "555"})
	if err != nil {
		t.Fatal(err)
	}
	return store, source, remote, contact
}

// One cold load fills the cache; a hit succeeds even if later source reads fail.
// This proves routing independently of SQL or Valkey availability.
func TestReadRoutes(t *testing.T) {
	store, source, remote, contact := fixture(t)
	if len(remote.values) != 0 || source.reads != 0 {
		t.Fatal("Create populated the cache or performed Get")
	}
	got, err := store.Get(t.Context(), contact.ID)
	if err != nil || got != contact || source.reads != 1 || remote.sets != 1 {
		t.Fatalf("cold read = (%+v, %v), reads=%d fills=%d", got, err, source.reads, remote.sets)
	}
	source.readErr = errors.New("source unavailable")
	got, err = store.Get(t.Context(), contact.ID)
	if err != nil || got != contact || source.reads != 1 || remote.sets != 1 {
		t.Fatalf("warm read = (%+v, %v), reads=%d fills=%d", got, err, source.reads, remote.sets)
	}
}

// Repeated not-found results remain SQL/source lookups. A source failure also
// propagates without inventing either a contact or a negative-cache entry.
func TestMissingAndFailedSourceReads(t *testing.T) {
	store, source, remote, _ := fixture(t)
	for range 2 {
		if _, err := store.Get(t.Context(), 99); !errors.Is(err, contacts.ErrNotFound) {
			t.Fatal(err)
		}
	}
	if source.reads != 2 || remote.sets != 0 {
		t.Fatalf("missing reads=%d fills=%d", source.reads, remote.sets)
	}
	failure := errors.New("database unavailable")
	source.readErr = failure
	if _, err := store.Get(t.Context(), 99); !errors.Is(err, failure) || remote.sets != 0 {
		t.Fatalf("source failure = %v, fills=%d", err, remote.sets)
	}
}

// Inspect the authoritative state at DEL time: replacement/patch/deletion must
// already be visible. Each next read then gets the new state, not an old copy.
func TestWritesInvalidateAfterSource(t *testing.T) {
	store, source, remote, contact := fixture(t)
	ctx := t.Context()
	if _, err := store.Get(ctx, contact.ID); err != nil {
		t.Fatal(err)
	}
	remote.beforeDelete = func() {
		got, err := source.Store.Get(ctx, contact.ID)
		if err != nil || got.Email != "" || got.Phone != "new" {
			t.Fatalf("DEL before replacement completed: %+v, %v", got, err)
		}
	}
	replaced, err := store.Replace(ctx, contact.ID, contacts.Input{Name: "Ana", Phone: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(ctx, contact.ID); err != nil || got != replaced {
		t.Fatalf("after replace = (%+v, %v)", got, err)
	}
	email := "new@example.com"
	remote.beforeDelete = func() {
		got, err := source.Store.Get(ctx, contact.ID)
		if err != nil || got.Email != email || got.Phone != "new" {
			t.Fatalf("DEL before patch completed: %+v, %v", got, err)
		}
	}
	patched, err := store.Patch(ctx, contact.ID, contacts.Changes{Email: &email})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(ctx, contact.ID); err != nil || got != patched {
		t.Fatalf("after patch = (%+v, %v)", got, err)
	}
	remote.beforeDelete = func() {
		if _, err := source.Store.Get(ctx, contact.ID); !errors.Is(err, contacts.ErrNotFound) {
			t.Fatalf("DEL before source deletion: %v", err)
		}
	}
	if err := store.Delete(ctx, contact.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, contact.ID); !errors.Is(err, contacts.ErrNotFound) {
		t.Fatal(err)
	}
	if remote.deletes != 3 || source.reads != 4 {
		t.Fatalf("invalidations=%d reads=%d", remote.deletes, source.reads)
	}
}

// A rejected write must not evict an otherwise valid cached contact. Input,
// uniqueness, and missing-row errors retain the source's domain classification.
func TestRejectedWritesPreserveCache(t *testing.T) {
	store, _, remote, contact := fixture(t)
	ctx := t.Context()
	if _, err := store.Get(ctx, contact.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, contacts.Input{Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(ctx, contact.ID, contacts.Input{Name: "Other"}); !errors.Is(err, contacts.ErrNameTaken) {
		t.Fatal(err)
	}
	blank := " "
	if _, err := store.Patch(ctx, contact.ID, contacts.Changes{Name: &blank}); !errors.Is(err, contacts.ErrInvalidInput) {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, 99); !errors.Is(err, contacts.ErrNotFound) {
		t.Fatal(err)
	}
	if remote.deletes != 0 || len(remote.values) != 1 {
		t.Fatalf("rejected writes changed cache: deletes=%d entries=%d", remote.deletes, len(remote.values))
	}
}

// Cache errors stay visible in this first version. InvalidationError identifies
// the especially important partial success: source committed, cache still old.
func TestCacheFailures(t *testing.T) {
	failure := errors.New("cache disconnected")
	t.Run("lookup error is not a miss", func(t *testing.T) {
		store, source, remote, contact := fixture(t)
		remote.getErr = failure
		if _, err := store.Get(t.Context(), contact.ID); !errors.Is(err, failure) || source.reads != 0 {
			t.Fatalf("lookup error=%v reads=%d", err, source.reads)
		}
	})
	t.Run("fill error after successful source read", func(t *testing.T) {
		store, source, remote, contact := fixture(t)
		remote.setErr = failure
		if _, err := store.Get(t.Context(), contact.ID); !errors.Is(err, failure) || source.reads != 1 {
			t.Fatalf("fill error=%v reads=%d", err, source.reads)
		}
	})
	t.Run("invalidation error after confirmed write", func(t *testing.T) {
		store, source, remote, contact := fixture(t)
		if _, err := store.Get(t.Context(), contact.ID); err != nil {
			t.Fatal(err)
		}
		remote.deleteErr = failure
		updated, err := store.Replace(t.Context(), contact.ID, contacts.Input{Name: "Updated"})
		var committed *InvalidationError
		if !errors.As(err, &committed) || !errors.Is(err, failure) || committed.ID != contact.ID || updated.Name != "Updated" {
			t.Fatalf("write outcome=%+v error=%v", updated, err)
		}
		got, err := source.Store.Get(t.Context(), contact.ID)
		if err != nil || got != updated {
			t.Fatalf("confirmed source state=%+v error=%v", got, err)
		}
	})
}

// Early cancellation and invalid IDs must not reach either dependency. In
// particular, a cached hit must not accidentally ignore an already canceled call.
func TestCanceledAndInvalidCalls(t *testing.T) {
	store, source, remote, contact := fixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, getErr := store.Get(ctx, contact.ID)
	_, createErr := store.Create(ctx, contacts.Input{Name: "Never"})
	_, replaceErr := store.Replace(ctx, contact.ID, contacts.Input{Name: "Never"})
	_, patchErr := store.Patch(ctx, contact.ID, contacts.Changes{})
	deleteErr := store.Delete(ctx, contact.ID)
	for _, err := range []error{getErr, createErr, replaceErr, patchErr, deleteErr} {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled call = %v", err)
		}
	}
	if _, err := store.Get(t.Context(), 0); !errors.Is(err, contacts.ErrInvalidInput) {
		t.Fatal(err)
	}
	if source.reads != 0 || remote.gets != 0 || remote.sets != 0 || remote.deletes != 0 {
		t.Fatal("invalid/canceled calls reached dependencies")
	}
}

// Configuration bounds keep keys readable and TTL conversion lossless.
func TestConfiguration(t *testing.T) {
	for _, namespace := range []string{"", "has space", "has:colon", "á"} {
		if _, err := NewStore(contacts.NewStore(), &testCache{}, namespace, time.Second); !errors.Is(err, cache.ErrInvalidInput) {
			t.Fatalf("namespace %q: %v", namespace, err)
		}
	}
	for _, ttl := range []time.Duration{0, -time.Second, time.Microsecond, time.Millisecond + 1} {
		if _, err := NewStore(contacts.NewStore(), &testCache{}, "valid", ttl); !errors.Is(err, cache.ErrInvalidInput) {
			t.Fatalf("TTL %v: %v", ttl, err)
		}
	}
	if _, err := NewStore(nil, &testCache{}, "valid", time.Second); !errors.Is(err, cache.ErrInvalidInput) {
		t.Fatal(err)
	}
	if _, err := NewStore(contacts.NewStore(), nil, "valid", time.Second); !errors.Is(err, cache.ErrInvalidInput) {
		t.Fatal(err)
	}
}
