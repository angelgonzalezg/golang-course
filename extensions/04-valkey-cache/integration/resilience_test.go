//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/course/cache-lab/cache"
	"example.com/course/cache-lab/contactcache"
	"example.com/course/cache-lab/internal/cachecontrol"
	"example.com/course/cache-lab/internal/resiliencedemo"
	"example.com/course/sql-lab/contactpg"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

type resilientReply struct {
	value contacts.Contact
	err   error
}

func resilientRead(ctx context.Context, store *contactcache.ResilientStore, id int64) <-chan resilientReply {
	ch := make(chan resilientReply, 1)
	go func() { value, err := store.Get(ctx, id); ch <- resilientReply{value, err} }()
	return ch
}

func await[T any](t *testing.T, ctx context.Context, ch <-chan T) T {
	t.Helper()
	value, err := cachecontrol.Wait(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func newResilient(t *testing.T, source contactcache.Source, remote contactcache.Cache, namespace string, probe *cachecontrol.Probe) *contactcache.ResilientStore {
	t.Helper()
	store, err := contactcache.NewResilientStore(t.Context(), source, remote, namespace, contactcache.ResilienceOptions{TTL: time.Minute, CacheTimeout: 200 * time.Millisecond, LoadTimeout: 3 * time.Second, Observe: probe.Observe})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return store
}

// All cases own real SQL/Valkey resources. The last case stops ONLY the owned
// cache to distinguish simulated command failures from a real server outage.
func TestCacheResilience(t *testing.T) {
	db, dsn := newCacheDatabase(t)
	client, _, container := newTestCache(t)
	remote := cache.NewStore(client)
	newRig := func(t *testing.T) (*contactcache.ResilientStore, *cachecontrol.HeldSource, *cachecontrol.Probe, contacts.Contact) {
		t.Helper()
		source := &cachecontrol.HeldSource{Source: contactpg.NewStore(db)}
		probe := &cachecontrol.Probe{}
		store := newResilient(t, source, remote, rand.Text(), probe)
		contact, err := store.Create(operationContext(t), contacts.Input{Name: "Resilient " + rand.Text(), Phone: "old"})
		if err != nil {
			t.Fatal(err)
		}
		return store, source, probe, contact
	}

	// Channels hold a completed SQL snapshot until every waiter is registered.
	// Canceling the initiating caller leaves the one shared SQL load alive.
	t.Run("shared SQL load survives initiator cancellation", func(t *testing.T) {
		ctx := operationContext(t)
		store, source, probe, contact := newRig(t)
		hold := source.HoldNextGet()
		defer hold.Release()
		queued := probe.Watch("lookup_queued", 6)
		firstCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		first := resilientRead(firstCtx, store, contact.ID)
		await(t, ctx, hold.Entered)
		others := make([]<-chan resilientReply, 5)
		for i := range others {
			others[i] = resilientRead(ctx, store, contact.ID)
		}
		for range 6 {
			await(t, ctx, queued)
		}
		cancel()
		if reply := await(t, ctx, first); !errors.Is(reply.err, context.Canceled) {
			t.Fatal(reply.err)
		}
		hold.Release()
		for _, ch := range others {
			if reply := await(t, ctx, ch); reply.err != nil || reply.value != contact {
				t.Fatalf("shared result=%+v", reply)
			}
		}
		if source.Reads.Load() != 1 {
			t.Fatalf("SQL loads=%d, want 1", source.Reads.Load())
		}
	})

	// A single instance holds its local gate through load/fill and through the
	// writer's commit/DEL, preventing a delayed old fill after invalidation.
	t.Run("local writer invalidates an earlier fill", func(t *testing.T) {
		ctx := operationContext(t)
		store, source, probe, contact := newRig(t)
		hold := source.HoldNextGet()
		defer hold.Release()
		reading := resilientRead(ctx, store, contact.ID)
		await(t, ctx, hold.Entered)
		waiting := probe.Watch("write_wait", 1)
		writing := make(chan resilientReply, 1)
		phone := "new"
		go func() {
			value, err := store.Patch(ctx, contact.ID, contacts.Changes{Phone: &phone})
			writing <- resilientReply{value, err}
		}()
		await(t, ctx, waiting)
		if got, err := contactpg.NewStore(db).Get(ctx, contact.ID); err != nil || got != contact {
			t.Fatal("SQL changed before old local load finished")
		}
		hold.Release()
		if reply := await(t, ctx, reading); reply.err != nil || reply.value != contact {
			t.Fatalf("old read=%+v", reply)
		}
		changed := await(t, ctx, writing)
		if changed.err != nil || changed.value.Phone != phone {
			t.Fatalf("write=%+v", changed)
		}
		key, _ := store.Key(contact.ID)
		assertMiss(t, ctx, remote, key)
		if got, err := store.Get(ctx, contact.ID); err != nil || got != changed.value {
			t.Fatalf("fresh read=(%+v,%v)", got, err)
		}
	})

	// Two store objects have independent gates/groups, just like two processes.
	// Deliberately reproduce stale repopulation to prove the local guarantee's limit.
	t.Run("another instance can still repopulate stale data", func(t *testing.T) {
		ctx := operationContext(t)
		namespace := rand.Text()
		source := &cachecontrol.HeldSource{Source: contactpg.NewStore(db)}
		first := newResilient(t, source, remote, namespace, &cachecontrol.Probe{})
		other := newResilient(t, contactpg.NewStore(db), remote, namespace, &cachecontrol.Probe{})
		contact, err := first.Create(ctx, contacts.Input{Name: "Two instances " + rand.Text(), Phone: "old"})
		if err != nil {
			t.Fatal(err)
		}
		hold := source.HoldNextGet()
		defer hold.Release()
		reading := resilientRead(ctx, first, contact.ID)
		await(t, ctx, hold.Entered)
		phone := "new"
		changed, err := other.Patch(ctx, contact.ID, contacts.Changes{Phone: &phone})
		if err != nil {
			t.Fatal(err)
		}
		hold.Release()
		if reply := await(t, ctx, reading); reply.err != nil || reply.value != contact {
			t.Fatalf("old instance result=%+v", reply)
		}
		if got, err := other.Get(ctx, contact.ID); err != nil || got != contact {
			t.Fatal("expected demonstration of cross-instance stale cache")
		}
		if got, err := contactpg.NewStore(db).Get(ctx, contact.ID); err != nil || got != changed {
			t.Fatal("SQL lost the committed update")
		}
		if err := other.Invalidate(ctx, contact.ID); err != nil {
			t.Fatal(err)
		}
		if got, err := other.Get(ctx, contact.ID); err != nil || got != changed {
			t.Fatal("targeted repair failed")
		}
	})

	// The finite demo uses controlled Go failures, real data, and isolated keys.
	// Repeated runs preserve a neighbor row, its durable replay, and its cache key.
	t.Run("demo cleanup preserves rows keys and replay", func(t *testing.T) {
		client, uri, _ := newTestCache(t)
		remote := cache.NewStore(client)
		ctx := operationContext(t)
		subject := rand.Text()
		input := contacts.Input{Name: "Neighbor " + rand.Text()}
		created, err := contactpg.NewStore(db).CreateOnce(ctx, subject, "neighbor", input)
		if err != nil {
			t.Fatal(err)
		}
		key := "neighbor:" + rand.Text()
		if err := remote.Set(ctx, key, "keep", time.Minute); err != nil {
			t.Fatal(err)
		}
		var before int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sql_service_contacts").Scan(&before); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			var output bytes.Buffer
			if err := resiliencedemo.Run(ctx, dsn, uri, &output); err != nil {
				t.Fatal(err)
			}
			for _, summary := range []string{"8 callers -> 1 shared SQL load", "initiating caller canceled", "cache_read_error observed", "cache_fill_error observed", "cache_payload_error observed", "Retry only Invalidate", "PostgreSQL contact loads=8"} {
				if !strings.Contains(output.String(), summary) {
					t.Errorf("missing demo output %q", summary)
				}
			}
			var after int
			if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sql_service_contacts").Scan(&after); err != nil || before != after {
				t.Fatalf("row cleanup before=%d after=%d err=%v", before, after, err)
			}
			if got, err := contactpg.NewStore(db).CreateOnce(ctx, subject, "neighbor", input); err != nil || !got.Replayed || got.Contact != created.Contact {
				t.Fatal("neighbor replay changed")
			}
			if value, err := remote.Get(ctx, key); err != nil || value != "keep" {
				t.Fatal("neighbor key changed")
			}
			if count, err := client.Do(ctx, client.B().Dbsize().Build()).ToInt64(); err != nil || count != 1 {
				t.Fatalf("demo leaked keys: %d %v", count, err)
			}
		}
	})

	// This is a real TCP/server failure, beyond the demo's fault switches. Reads
	// fall back, while failed post-commit invalidation remains an explicit outcome.
	t.Run("real cache outage", func(t *testing.T) {
		ctx := operationContext(t)
		store, source, probe, contact := newRig(t)
		if _, err := store.Get(ctx, contact.ID); err != nil {
			t.Fatal(err)
		}
		if err := container.Stop(ctx, nil); err != nil {
			t.Fatal(err)
		}
		if got, err := store.Get(ctx, contact.ID); err != nil || got != contact {
			t.Fatalf("outage fallback=(%+v,%v)", got, err)
		}
		phone := "during-outage"
		changed, err := store.Patch(ctx, contact.ID, contacts.Changes{Phone: &phone})
		var committed *contactcache.InvalidationError
		if !errors.As(err, &committed) || changed.Phone != phone {
			t.Fatalf("outage write=(%+v,%v)", changed, err)
		}
		if got, err := store.Get(ctx, contact.ID); err != nil || got != changed {
			t.Fatalf("outage updated read=(%+v,%v)", got, err)
		}
		if source.Reads.Load() != 3 || probe.Count("cache_read_error") != 2 || probe.Count("cache_fill_error") != 2 || probe.Count("invalidation_error") != 1 {
			t.Fatal("real outage was hidden or misrouted")
		}
	})
}
