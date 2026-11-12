//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"example.com/course/cache-lab/cache"
	"example.com/course/cache-lab/contactcache"
	"example.com/course/cache-lab/internal/asidedemo"
	"example.com/course/sql-lab/contactpg"
	"example.com/course/sql-lab/contactsql"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// Count contact Get calls at the real SQL adapter boundary. Writes still run
// their own SQL transactions; this deliberately does not count every statement.
type countedSQL struct {
	*contactpg.Store
	reads atomic.Int64
}

func (s *countedSQL) Get(ctx context.Context, id int64) (contacts.Contact, error) {
	s.reads.Add(1)
	return s.Store.Get(ctx, id)
}

// This suite owns BOTH real servers and discovers dynamic ports. It never
// reads DATABASE_URL/VALKEY_URL or touches the learner's Compose resources.
func TestCacheAside(t *testing.T) {
	db, dsn := newCacheDatabase(t)
	client, uri, _ := newTestCache(t)
	remote := cache.NewStore(client)

	t.Run("cold warm and independent client", func(t *testing.T) {
		ctx := operationContext(t)
		source := &countedSQL{Store: contactpg.NewStore(db)}
		namespace := rand.Text()
		store := newAside(t, source, remote, namespace, time.Minute)
		contact, err := store.Create(ctx, contacts.Input{Name: "Warm " + rand.Text(), Email: "ana@example.com", Phone: "555"})
		if err != nil {
			t.Fatal(err)
		}
		key, _ := store.Key(contact.ID)
		assertMiss(t, ctx, remote, key)
		if source.reads.Load() != 0 {
			t.Fatal("creation called contact Get")
		}
		assertContact(t, ctx, store, contact)
		if source.reads.Load() != 1 {
			t.Fatal("cold lookup did not load PostgreSQL exactly once")
		}
		if info, err := remote.TTL(ctx, key); err != nil || !info.HasTTL || info.Remaining > time.Minute {
			t.Fatalf("cache fill TTL=(%+v, %v)", info, err)
		}
		// A fresh client sees the same server entry. This is remote Valkey data,
		// not a local map or the valkey-go client-side caching feature.
		second, err := cache.Open(ctx, uri)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(second.Close)
		other := newAside(t, source, cache.NewStore(second), namespace, time.Minute)
		assertContact(t, ctx, other, contact)
		if source.reads.Load() != 1 {
			t.Fatal("warm lookup reached PostgreSQL")
		}
	})

	t.Run("writes invalidate committed values", func(t *testing.T) {
		ctx := operationContext(t)
		source := &countedSQL{Store: contactpg.NewStore(db)}
		store := newAside(t, source, remote, rand.Text(), time.Minute)
		contact, err := store.Create(ctx, contacts.Input{Name: "Write " + rand.Text(), Email: "old@example.com", Phone: "old"})
		if err != nil {
			t.Fatal(err)
		}
		key, _ := store.Key(contact.ID)
		assertContact(t, ctx, store, contact)
		contact, err = store.Replace(ctx, contact.ID, contacts.Input{Name: contact.Name, Phone: "new"})
		if err != nil || contact.Email != "" {
			t.Fatalf("replace=(%+v, %v)", contact, err)
		}
		assertMiss(t, ctx, remote, key)
		persisted, err := contactpg.NewStore(db).Get(ctx, contact.ID)
		if err != nil || persisted != contact {
			t.Fatalf("committed replacement=(%+v, %v)", persisted, err)
		}
		assertContact(t, ctx, store, contact)
		email := "new@example.com"
		contact, err = store.Patch(ctx, contact.ID, contacts.Changes{Email: &email})
		if err != nil || contact.Phone != "new" || contact.Email != email {
			t.Fatalf("patch=(%+v, %v)", contact, err)
		}
		assertMiss(t, ctx, remote, key)
		assertContact(t, ctx, store, contact)
		if source.reads.Load() != 3 {
			t.Fatalf("write/read cycle Get count=%d, want 3", source.reads.Load())
		}
		if err := store.Delete(ctx, contact.ID); err != nil {
			t.Fatal(err)
		}
		assertMiss(t, ctx, remote, key)
		// No negative caching: both reads consult SQL and preserve ErrNotFound.
		for range 2 {
			if _, err := store.Get(ctx, contact.ID); !errors.Is(err, contacts.ErrNotFound) {
				t.Fatal(err)
			}
			assertMiss(t, ctx, remote, key)
		}
		if source.reads.Load() != 5 {
			t.Fatalf("missing Get count=%d, want 5", source.reads.Load())
		}
	})

	t.Run("actual expiration reloads PostgreSQL", func(t *testing.T) {
		ctx := operationContext(t)
		source := &countedSQL{Store: contactpg.NewStore(db)}
		store := newAside(t, source, remote, rand.Text(), 80*time.Millisecond)
		contact, err := store.Create(ctx, contacts.Input{Name: "Expiry " + rand.Text()})
		if err != nil {
			t.Fatal(err)
		}
		assertContact(t, ctx, store, contact)
		key, _ := store.Key(contact.ID)
		deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			_, err := remote.Get(deadline, key)
			if errors.Is(err, cache.ErrMiss) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-deadline.Done():
				t.Fatal(deadline.Err())
			case <-ticker.C:
			}
		}
		assertContact(t, ctx, store, contact)
		if source.reads.Load() != 2 {
			t.Fatalf("expiration loads=%d, want 2", source.reads.Load())
		}
	})

	t.Run("corruption and cancellation stay explicit", func(t *testing.T) {
		ctx := operationContext(t)
		source := &countedSQL{Store: contactpg.NewStore(db)}
		store := newAside(t, source, remote, rand.Text(), time.Minute)
		contact, err := store.Create(ctx, contacts.Input{Name: "Invalid " + rand.Text()})
		if err != nil {
			t.Fatal(err)
		}
		key, _ := store.Key(contact.ID)
		if err := remote.Set(ctx, key, "{", time.Minute); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Get(ctx, contact.ID); !errors.Is(err, cache.ErrInvalidPayload) {
			t.Fatal(err)
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := store.Get(canceled, contact.ID); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if source.reads.Load() != 0 {
			t.Fatal("corrupt/canceled lookup silently fell back to SQL")
		}
	})

	t.Run("demo repeats and preserves neighbors", func(t *testing.T) {
		ctx := operationContext(t)
		// A separate cache server makes the exact key count deterministic; no
		// short-lived entries from another scenario can mask a leaked demo key.
		client, uri, _ := newTestCache(t)
		remote := cache.NewStore(client)
		subject := rand.Text()
		input := contacts.Input{Name: "Neighbor " + rand.Text()}
		created, err := contactpg.NewStore(db).CreateOnce(ctx, subject, "neighbor", input)
		if err != nil {
			t.Fatal(err)
		}
		neighbor := created.Contact
		neighborKey := "neighbor:" + rand.Text()
		if err := remote.Set(ctx, neighborKey, "keep", time.Minute); err != nil {
			t.Fatal(err)
		}
		var before int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sql_service_contacts").Scan(&before); err != nil {
			t.Fatal(err)
		}
		beforeKeys, err := client.Do(ctx, client.B().Dbsize().Build()).ToInt64()
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			var output bytes.Buffer
			if err := asidedemo.Run(ctx, dsn, uri, &output); err != nil {
				t.Fatal(err)
			}
			for _, summary := range []string{"cache empty; PostgreSQL contact reads=0", "HIT -> Valkey only; PostgreSQL contact reads=1", "REPLACE -> PostgreSQL committed -> DEL", "PATCH -> PostgreSQL committed -> DEL", "GET after expiration -> MISS", "ErrNotFound; PostgreSQL contact reads=6"} {
				if !strings.Contains(output.String(), summary) {
					t.Errorf("demo missing %q", summary)
				}
			}
			got, err := contactpg.NewStore(db).Get(ctx, neighbor.ID)
			if err != nil || got != neighbor {
				t.Fatalf("neighbor SQL=(%+v, %v)", got, err)
			}
			replay, err := contactpg.NewStore(db).CreateOnce(ctx, subject, "neighbor", input)
			if err != nil || !replay.Replayed || replay.Contact != neighbor {
				t.Fatalf("neighbor durable replay=(%+v, %v)", replay, err)
			}
			if value, err := remote.Get(ctx, neighborKey); err != nil || value != "keep" {
				t.Fatalf("neighbor cache=(%q, %v)", value, err)
			}
			var after int
			if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sql_service_contacts").Scan(&after); err != nil || after != before {
				t.Fatalf("demo leaked/removed SQL rows: before=%d after=%d error=%v", before, after, err)
			}
			afterKeys, err := client.Do(ctx, client.B().Dbsize().Build()).ToInt64()
			if err != nil || afterKeys != beforeKeys {
				t.Fatalf("demo leaked keys: before=%d after=%d error=%v", beforeKeys, afterKeys, err)
			}
		}
	})
}

// The existing service migration uses its own version table and golang-migrate.
// Pool cleanup is registered after container cleanup, so it runs first.
func newCacheDatabase(t *testing.T) (*sql.DB, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	container, err := postgres.Run(ctx, "postgres:18.6-alpine3.24",
		postgres.WithDatabase("cache_test"), postgres.WithUsername("cache_test"),
		postgres.WithPassword("cache_test_password"), postgres.BasicWaitStrategies())
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("start PostgreSQL (check docker info): %v", err)
	}
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := contactpg.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	db, err := contactsql.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db, dsn
}

func newAside(t *testing.T, source contactcache.Source, remote contactcache.Cache, namespace string, ttl time.Duration) *contactcache.Store {
	t.Helper()
	store, err := contactcache.NewStore(source, remote, namespace, ttl)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func assertContact(t *testing.T, ctx context.Context, store *contactcache.Store, want contacts.Contact) {
	t.Helper()
	if got, err := store.Get(ctx, want.ID); err != nil || got != want {
		t.Fatalf("contact=(%+v, %v), want %+v", got, err, want)
	}
}

func assertMiss(t *testing.T, ctx context.Context, remote *cache.Store, key string) {
	t.Helper()
	if _, err := remote.Get(ctx, key); !errors.Is(err, cache.ErrMiss) {
		t.Fatalf("expected cache miss: %v", err)
	}
}
