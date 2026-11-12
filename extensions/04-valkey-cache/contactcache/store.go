// Package contactcache teaches cache-aside for contacts independently of HTTP
// and gRPC. PostgreSQL owns the records; Valkey holds expiring copies by ID.
package contactcache

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"example.com/course/cache-lab/cache"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// Source contains only the authoritative operations needed in this lesson.
// contactpg.Store satisfies it. Successful writes must be committed before
// returning; cache invalidation happens AFTER that boundary.
type Source interface {
	Create(context.Context, contacts.Input) (contacts.Contact, error)
	Get(context.Context, int64) (contacts.Contact, error)
	Replace(context.Context, int64, contacts.Input) (contacts.Contact, error)
	Patch(context.Context, int64, contacts.Changes) (contacts.Contact, error)
	Delete(context.Context, int64) error
}

// Cache is the remote string API already taught in the fundamentals. Store
// borrows both dependencies; their application owner closes pools and clients.
type Cache interface {
	Get(context.Context, string) (string, error)
	Set(context.Context, string, string, time.Duration) error
	Delete(context.Context, string) (bool, error)
}

// Store decorates a source with lazy loading and write invalidation. It does
// not cache collection queries or missing records. This first version exposes
// cache failures as errors; fallback and concurrent-load policy follow later.
type Store struct {
	source Source
	cache  Cache
	prefix string
	ttl    time.Duration
}

// NewStore requires a namespace identifying ONE database/data set. Applications
// using different databases must not share it, even when contact IDs coincide.
// The random namespace in the demo isolates runs; a service uses a stable one.
func NewStore(source Source, remote Cache, namespace string, ttl time.Duration) (*Store, error) {
	if source == nil || remote == nil {
		return nil, fmt.Errorf("source and cache are required: %w", cache.ErrInvalidInput)
	}
	if namespace == "" || len(namespace) > 80 || strings.IndexFunc(namespace, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	}) >= 0 {
		return nil, fmt.Errorf("namespace requires 1–80 ASCII letters, digits, hyphens or underscores: %w", cache.ErrInvalidInput)
	}
	if ttl < time.Millisecond || ttl%time.Millisecond != 0 {
		return nil, fmt.Errorf("TTL requires positive whole milliseconds: %w", cache.ErrInvalidInput)
	}
	return &Store{source: source, cache: remote, prefix: "course:cache-aside:" + namespace + ":v1:contact:", ttl: ttl}, nil
}

// Key exposes the exact owned key for inspection and targeted cleanup. The v1
// component versions this payload, separately from the SQL migration version.
func (s *Store) Key(id int64) (string, error) {
	if err := contacts.ValidateID(id); err != nil {
		return "", err
	}
	return s.prefix + strconv.FormatInt(id, 10), nil
}

// Get follows three steps: look in Valkey, load a miss from PostgreSQL, then
// populate Valkey with an atomic SET/PX. A hit never calls source.Get. A hit
// also leaves the original TTL unchanged, so frequently read keys still expire.
func (s *Store) Get(ctx context.Context, id int64) (contacts.Contact, error) {
	if err := ctx.Err(); err != nil {
		return contacts.Contact{}, err
	}
	key, err := s.Key(id)
	if err != nil {
		return contacts.Contact{}, err
	}
	value, err := s.cache.Get(ctx, key)
	if err == nil {
		return decode(value, id)
	}
	if !errors.Is(err, cache.ErrMiss) {
		// Unavailable Valkey and corrupt payloads are not ordinary misses. A later
		// lesson adds an explicit fallback policy instead of silently hiding them.
		return contacts.Contact{}, fmt.Errorf("read contact cache: %w", err)
	}
	contact, err := s.source.Get(ctx, id)
	if err != nil {
		// ErrNotFound stays a source error; no negative-cache entry is created.
		return contacts.Contact{}, err
	}
	value, err = encode(contact, id)
	if err != nil {
		return contacts.Contact{}, err
	}
	if err := s.cache.Set(ctx, key, value, s.ttl); err != nil {
		return contacts.Contact{}, fmt.Errorf("fill contact cache: %w", err)
	}
	return contact, nil
}

// Create writes only to the source. IDs are never reused and absent IDs are not
// cached, so there is no old entry to invalidate. The first Get fills the cache.
func (s *Store) Create(ctx context.Context, input contacts.Input) (contacts.Contact, error) {
	if err := ctx.Err(); err != nil {
		return contacts.Contact{}, err
	}
	return s.source.Create(ctx, input)
}

// Replace sends all editable fields directly to the source, without consulting
// a cached copy. It invalidates only after the source reports a successful write.
func (s *Store) Replace(ctx context.Context, id int64, input contacts.Input) (contacts.Contact, error) {
	if err := s.check(ctx, id); err != nil {
		return contacts.Contact{}, err
	}
	contact, err := s.source.Replace(ctx, id, input)
	if err != nil {
		return contacts.Contact{}, err
	}
	return contact, s.invalidate(ctx, id)
}

// Patch lets the source merge fields under its database row lock. Merging with
// a cached copy could overwrite a newer value and bypass the SQL guarantees.
func (s *Store) Patch(ctx context.Context, id int64, changes contacts.Changes) (contacts.Contact, error) {
	if err := s.check(ctx, id); err != nil {
		return contacts.Contact{}, err
	}
	contact, err := s.source.Patch(ctx, id, changes)
	if err != nil {
		return contacts.Contact{}, err
	}
	return contact, s.invalidate(ctx, id)
}

// Delete removes the authoritative row first, then its temporary cached copy.
// Failed source operations leave the existing cache entry alone.
func (s *Store) Delete(ctx context.Context, id int64) error {
	if err := s.check(ctx, id); err != nil {
		return err
	}
	if err := s.source.Delete(ctx, id); err != nil {
		return err
	}
	return s.invalidate(ctx, id)
}

// InvalidationError means the source ALREADY confirmed the write, but DEL
// failed. It must not be mistaken for a rolled-back transaction or blindly
// retried as a whole write. Unwrap preserves cancellation and driver causes.
type InvalidationError struct {
	ID  int64
	Err error
}

func (e *InvalidationError) Error() string {
	return fmt.Sprintf("source write succeeded for contact %d; cache invalidation failed: %v", e.ID, e.Err)
}

func (e *InvalidationError) Unwrap() error { return e.Err }

func (s *Store) check(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return contacts.ValidateID(id)
}

func (s *Store) invalidate(ctx context.Context, id int64) error {
	key, _ := s.Key(id) // Every writer validated the ID before reaching the source.
	if _, err := s.cache.Delete(ctx, key); err != nil {
		return &InvalidationError{ID: id, Err: err}
	}
	// DEL on an already absent key is successful invalidation too.
	return nil
}
