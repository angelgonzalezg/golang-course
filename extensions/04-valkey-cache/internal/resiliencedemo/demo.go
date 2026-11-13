// Package resiliencedemo demonstrates real SQL/cache data with controlled Go
// failure switches and channel barriers. It never stops a learner's services.
package resiliencedemo

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"example.com/course/cache-lab/cache"
	"example.com/course/cache-lab/contactcache"
	"example.com/course/cache-lab/internal/cachecontrol"
	"example.com/course/sql-lab/contactpg"
	"example.com/course/sql-lab/contactsql"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

type result struct {
	contact contacts.Contact
	err     error
}

// Run creates one owned contact/namespace. Close joins admitted cache work
// before fresh-context cleanup and before closing either borrowed connection.
func Run(ctx context.Context, dsn, uri string, out io.Writer) (runErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := contactpg.Migrate(dsn); err != nil {
		return err
	}
	db, err := contactsql.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	client, err := cache.Open(ctx, uri)
	if err != nil {
		return err
	}
	defer client.Close()
	remote := cache.NewStore(client)
	source := &cachecontrol.HeldSource{Source: contactpg.NewStore(db)}
	faults := &cachecontrol.FaultCache{Cache: remote}
	probe := &cachecontrol.Probe{}
	namespace := "resilience-" + rand.Text()[:12]
	store, err := contactcache.NewResilientStore(ctx, source, faults, namespace, contactcache.ResilienceOptions{TTL: time.Minute, CacheTimeout: 300 * time.Millisecond, LoadTimeout: 5 * time.Second, Observe: probe.Observe})
	if err != nil {
		return err
	}
	var ownedID int64
	defer func() {
		store.Close()
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if ownedID != 0 {
			if err := source.Source.Delete(cleanup, ownedID); err != nil && !errors.Is(err, contacts.ErrNotFound) {
				runErr = errors.Join(runErr, err)
			}
			key, _ := store.Key(ownedID)
			_, err := remote.Delete(cleanup, key)
			runErr = errors.Join(runErr, err)
		}
	}()
	contact, err := store.Create(ctx, contacts.Input{Name: "Resilience " + namespace, Email: "ana@example.com", Phone: "old"})
	if err != nil {
		return err
	}
	ownedID = contact.ID

	fmt.Fprintln(out, "== 1. Cold and warm reads ==")
	if err := expect(ctx, store, contact); err != nil {
		return err
	}
	if err := expect(ctx, store, contact); err != nil {
		return err
	}
	if source.Reads.Load() != 1 {
		return errors.New("warm read reached PostgreSQL")
	}
	fmt.Fprintln(out, "Cold MISS then warm HIT -> PostgreSQL contact loads=1")

	fmt.Fprintln(out, "\n== 2. Shared load and independent cancellation ==")
	if err := store.Invalidate(ctx, contact.ID); err != nil {
		return err
	}
	hold := source.HoldNextGet()
	defer hold.Release()
	queued := probe.Watch("lookup_queued", 8)
	firstCtx, cancelFirst := context.WithCancel(ctx)
	defer cancelFirst()
	first := lookup(firstCtx, store, contact.ID)
	if _, err := cachecontrol.Wait(ctx, hold.Entered); err != nil {
		return err
	}
	others := make([]<-chan result, 7)
	for i := range others {
		others[i] = lookup(ctx, store, contact.ID)
	}
	for range 8 {
		if _, err := cachecontrol.Wait(ctx, queued); err != nil {
			return err
		}
	}
	cancelFirst()
	initial, err := cachecontrol.Wait(ctx, first)
	if err != nil {
		return err
	}
	if !errors.Is(initial.err, context.Canceled) {
		return fmt.Errorf("initiator cancellation: %v", initial.err)
	}
	hold.Release()
	for _, reply := range others {
		r, err := cachecontrol.Wait(ctx, reply)
		if err != nil {
			return err
		}
		if r.err != nil || r.contact != contact {
			return fmt.Errorf("shared read: %+v", r)
		}
	}
	if source.Reads.Load() != 2 {
		return errors.New("burst duplicated PostgreSQL loads")
	}
	fmt.Fprintln(out, "8 callers -> 1 shared SQL load; initiating caller canceled; 7 callers succeeded")

	fmt.Fprintln(out, "\n== 3. Local read/write ordering ==")
	if err := store.Invalidate(ctx, contact.ID); err != nil {
		return err
	}
	held := source.HoldNextGet()
	defer held.Release()
	reading := lookup(ctx, store, contact.ID)
	if _, err := cachecontrol.Wait(ctx, held.Entered); err != nil {
		return err
	}
	waiting := probe.Watch("write_wait", 1)
	writing := make(chan result, 1)
	phone := "new"
	go func() {
		value, err := store.Patch(ctx, contact.ID, contacts.Changes{Phone: &phone})
		writing <- result{value, err}
	}()
	if _, err := cachecontrol.Wait(ctx, waiting); err != nil {
		return err
	}
	held.Release()
	old, err := cachecontrol.Wait(ctx, reading)
	if err != nil {
		return err
	}
	if old.err != nil || old.contact != contact {
		return errors.New("held reader lost its original snapshot")
	}
	updated, err := cachecontrol.Wait(ctx, writing)
	if err != nil {
		return err
	}
	if updated.err != nil {
		return updated.err
	}
	contact = updated.contact
	key, _ := store.Key(contact.ID)
	if _, err := remote.Get(ctx, key); !errors.Is(err, cache.ErrMiss) {
		return errors.New("old local fill survived write invalidation")
	}
	if err := expect(ctx, store, contact); err != nil {
		return err
	}
	fmt.Fprintln(out, "Old local load finishes -> writer commits -> DEL -> next GET sees new phone")

	fmt.Fprintln(out, "\n== 4. Visible cache failures with SQL fallback ==")
	if err := store.Invalidate(ctx, contact.ID); err != nil {
		return err
	}
	faults.FailGet.Store(true)
	if err := expect(ctx, store, contact); err != nil {
		return err
	}
	faults.FailGet.Store(false)
	fmt.Fprintln(out, "Controlled GET failure -> valid SQL result; cache_read_error observed")
	if err := store.Invalidate(ctx, contact.ID); err != nil {
		return err
	}
	faults.FailSet.Store(true)
	if err := expect(ctx, store, contact); err != nil {
		return err
	}
	faults.FailSet.Store(false)
	fmt.Fprintln(out, "Controlled SET failure -> valid SQL result; cache_fill_error observed")
	if err := remote.Set(ctx, key, "{", time.Minute); err != nil {
		return err
	}
	if err := expect(ctx, store, contact); err != nil {
		return err
	}
	fmt.Fprintln(out, "Corrupt JSON -> SQL reload and replacement; cache_payload_error observed")

	fmt.Fprintln(out, "\n== 5. Failed invalidation and explicit repair ==")
	faults.FailDelete.Store(true)
	phone = "after-failed-del"
	changed, err := store.Patch(ctx, contact.ID, contacts.Changes{Phone: &phone})
	var committed *contactcache.InvalidationError
	if !errors.As(err, &committed) || changed.Phone != phone {
		return fmt.Errorf("expected committed write with failed DEL: %v", err)
	}
	// A hit can still be old even though SQL changed. Do not hide that limit.
	if err := expect(ctx, store, contact); err != nil {
		return err
	}
	fmt.Fprintln(out, "SQL committed; DEL failed -> InvalidationError; old cached phone can still be read")
	faults.FailDelete.Store(false)
	if err := store.Invalidate(ctx, contact.ID); err != nil {
		return err
	}
	if err := expect(ctx, store, changed); err != nil {
		return err
	}
	fmt.Fprintln(out, "Retry only Invalidate -> next GET reloads committed data; no SQL write retried")
	if source.Reads.Load() != 8 || probe.Count("cache_read_error") != 1 || probe.Count("cache_fill_error") != 1 || probe.Count("cache_payload_error") != 1 || probe.Count("invalidation_error") != 1 {
		return errors.New("unexpected routing/diagnostic counters")
	}
	fmt.Fprintln(out, "PostgreSQL contact loads=8; all controlled failures observed")
	fmt.Fprintln(out, "Local coordination only; other instances still need a distributed consistency policy")
	return nil
}

// One buffered result lets a canceled caller finish without blocking the sender.
func lookup(ctx context.Context, store *contactcache.ResilientStore, id int64) <-chan result {
	reply := make(chan result, 1)
	go func() { value, err := store.Get(ctx, id); reply <- result{value, err} }()
	return reply
}

func expect(ctx context.Context, store *contactcache.ResilientStore, want contacts.Contact) error {
	got, err := store.Get(ctx, want.ID)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("contact=%+v, want %+v", got, want)
	}
	return nil
}
