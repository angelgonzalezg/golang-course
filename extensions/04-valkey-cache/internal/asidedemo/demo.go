// Package asidedemo runs a finite, sequential cache-aside demonstration against
// real PostgreSQL and Valkey. It owns only its newly created contacts and keys.
package asidedemo

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"example.com/course/cache-lab/cache"
	"example.com/course/cache-lab/contactcache"
	"example.com/course/sql-lab/contactpg"
	"example.com/course/sql-lab/contactsql"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// countedSource counts calls to Get, not every SQL statement. Migrations, PING,
// writes, and SELECTs inside write transactions are outside this counter. This
// finite demo is sequential; a concurrent application would synchronize metrics.
type countedSource struct {
	contactcache.Source
	reads int
}

func (s *countedSource) Get(ctx context.Context, id int64) (contacts.Contact, error) {
	s.reads++
	return s.Source.Get(ctx, id)
}

// Run applies the existing golang-migrate service migrations, opens its own
// pool/client, and closes them after targeted cleanup. It starts no HTTP server.
func Run(ctx context.Context, dsn, uri string, out io.Writer) (result error) {
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
	source := &countedSource{Source: contactpg.NewStore(db)}
	namespace := "demo-" + rand.Text()[:12]
	store, err := contactcache.NewStore(source, remote, namespace, time.Minute)
	if err != nil {
		return err
	}

	var owned []int64
	defer func() {
		// Request cancellation must not cancel cleanup. Never FLUSHDB, truncate
		// a shared table, or remove another lab's durable idempotency results.
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, id := range owned {
			if err := source.Delete(cleanup, id); err != nil && !errors.Is(err, contacts.ErrNotFound) {
				result = errors.Join(result, err)
			}
			key, _ := store.Key(id)
			_, err := remote.Delete(cleanup, key)
			result = errors.Join(result, err)
		}
	}()

	fmt.Fprintln(out, "== 1. Create writes only to PostgreSQL ==")
	contact, err := store.Create(ctx, contacts.Input{Name: "Ana " + namespace, Email: "ana@example.com", Phone: "555-0100"})
	if err != nil {
		return err
	}
	owned = append(owned, contact.ID)
	if err := absent(ctx, remote, store, contact.ID); err != nil {
		return err
	}
	fmt.Fprintln(out, "CREATE -> PostgreSQL; cache empty; PostgreSQL contact reads=0")

	fmt.Fprintln(out, "\n== 2. Cold read loads the source ==")
	if err := showRead(ctx, out, store, source, contact, 1, "First GET"); err != nil {
		return err
	}
	key, _ := store.Key(contact.ID)
	encoded, err := remote.Get(ctx, key)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Cached JSON: %s\n", encoded)

	fmt.Fprintln(out, "\n== 3. Warm read reaches only Valkey ==")
	if err := showRead(ctx, out, store, source, contact, 1, "Second GET"); err != nil {
		return err
	}

	fmt.Fprintln(out, "\n== 4. Full replacement commits, then invalidates ==")
	contact, err = store.Replace(ctx, contact.ID, contacts.Input{Name: contact.Name, Phone: "555-0200"})
	if err != nil {
		return err
	}
	if contact.Email != "" {
		return errors.New("replacement did not clear the omitted email")
	}
	if err := absent(ctx, remote, store, contact.ID); err != nil {
		return err
	}
	fmt.Fprintln(out, "REPLACE -> PostgreSQL committed -> DEL; omitted email cleared")
	if err := showRead(ctx, out, store, source, contact, 2, "GET after replacement"); err != nil {
		return err
	}

	fmt.Fprintln(out, "\n== 5. Partial update uses the SQL row, then invalidates ==")
	email := "updated@example.com"
	contact, err = store.Patch(ctx, contact.ID, contacts.Changes{Email: &email})
	if err != nil {
		return err
	}
	if contact.Email != email || contact.Phone != "555-0200" {
		return errors.New("partial update lost a field")
	}
	if err := absent(ctx, remote, store, contact.ID); err != nil {
		return err
	}
	fmt.Fprintln(out, "PATCH -> PostgreSQL committed -> DEL; phone preserved")
	if err := showRead(ctx, out, store, source, contact, 3, "GET after partial update"); err != nil {
		return err
	}

	fmt.Fprintln(out, "\n== 6. Expiration reloads durable data ==")
	expiring, err := store.Create(ctx, contacts.Input{Name: "TTL " + namespace})
	if err != nil {
		return err
	}
	owned = append(owned, expiring.ID)
	short, err := contactcache.NewStore(source, remote, namespace, 120*time.Millisecond)
	if err != nil {
		return err
	}
	if err := showRead(ctx, out, short, source, expiring, 4, "GET short-TTL contact"); err != nil {
		return err
	}
	if err := waitForExpiration(ctx, remote, short, expiring.ID); err != nil {
		return err
	}
	fmt.Fprintln(out, "Valkey key expired; PostgreSQL row remains")
	if err := showRead(ctx, out, short, source, expiring, 5, "GET after expiration"); err != nil {
		return err
	}

	fmt.Fprintln(out, "\n== 7. Delete removes the source and invalidates ==")
	if err := store.Delete(ctx, contact.ID); err != nil {
		return err
	}
	if err := absent(ctx, remote, store, contact.ID); err != nil {
		return err
	}
	if _, err := store.Get(ctx, contact.ID); !errors.Is(err, contacts.ErrNotFound) {
		return fmt.Errorf("deleted contact lookup: expected ErrNotFound, got %v", err)
	}
	if source.reads != 6 {
		return fmt.Errorf("contact reads=%d, want 6", source.reads)
	}
	fmt.Fprintln(out, "DELETE -> PostgreSQL committed -> DEL")
	fmt.Fprintln(out, "GET after delete -> MISS -> PostgreSQL -> ErrNotFound; PostgreSQL contact reads=6")
	fmt.Fprintln(out, "Only this run's contacts and keys are cleaned up; idempotency records are untouched.")
	return nil
}

// Verify the value and source count, then report the observed route. These
// counts prove avoided contact reads, not a latency benchmark or all-query total.
func showRead(ctx context.Context, out io.Writer, store *contactcache.Store, source *countedSource, want contacts.Contact, reads int, label string) error {
	before := source.reads
	got, err := store.Get(ctx, want.ID)
	if err != nil {
		return err
	}
	if got != want || source.reads != reads {
		return fmt.Errorf("%s: value=%+v reads=%d, want %+v reads=%d", label, got, source.reads, want, reads)
	}
	route := "HIT -> Valkey only"
	if source.reads != before {
		route = "MISS -> PostgreSQL -> fill Valkey"
	}
	fmt.Fprintf(out, "%s -> %s; PostgreSQL contact reads=%d\n", label, route, source.reads)
	return nil
}

func absent(ctx context.Context, remote *cache.Store, store *contactcache.Store, id int64) error {
	key, _ := store.Key(id)
	if _, err := remote.Get(ctx, key); !errors.Is(err, cache.ErrMiss) {
		return fmt.Errorf("expected absent contact key, got %v", err)
	}
	return nil
}

// Poll the real server under a deadline; Go's clock cannot expire container keys.
// Polling only Valkey leaves the PostgreSQL contact-read counter unchanged.
func waitForExpiration(ctx context.Context, remote *cache.Store, store *contactcache.Store, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	key, _ := store.Key(id)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := remote.Get(ctx, key); errors.Is(err, cache.ErrMiss) {
			return nil
		} else if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
