package contactcache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"example.com/course/cache-lab/cache"
	"golang.org/x/sync/singleflight"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// ErrClosed rejects work after the owner closes or cancels the store lifetime.
var ErrClosed = errors.New("resilient contact cache closed")

// Event exposes routing and suppressed cache failures to logs/metrics without
// including contact data. Observe must be fast, concurrent-safe, and must not
// call back into this store/Close. lookup_queued means registered with
// singleflight; it does not by itself prove that a call has a duplicate.
type Event struct {
	Operation string
	ID        int64
	Err       error
}

// ResilienceOptions separates record lifetime, remote-command budget, and total
// shared-load budget. All are positive; TTL requires whole milliseconds.
type ResilienceOptions struct {
	TTL          time.Duration
	CacheTimeout time.Duration
	LoadTimeout  time.Duration
	Observe      func(Event)
}

// ResilientStore adds a policy to the strict stage-2 Store. Use its pointer:
// copying it would copy locks/singleflight state. It owns its shared loads;
// the application still owns the borrowed SQL pool and Valkey client.
type ResilientStore struct {
	base   *Store
	opts   ResilienceOptions
	ctx    context.Context
	cancel context.CancelFunc
	group  singleflight.Group
	// Fixed stripes bound lock memory. IDs colliding modulo 32 also serialize.
	// Each channel gates both complete loads and writes, with cancelable waits.
	gates  [32]chan struct{}
	mu     sync.Mutex // guards admission before Close starts waiting
	closed bool
	active sync.WaitGroup
}

// NewResilientStore receives an APPLICATION lifetime, not a request context.
// Shared loads use that lifetime plus LoadTimeout, independently of one waiter.
// Close this store before closing its borrowed dependencies.
func NewResilientStore(lifetime context.Context, source Source, remote Cache, namespace string, opts ResilienceOptions) (*ResilientStore, error) {
	if lifetime == nil || opts.CacheTimeout <= 0 || opts.LoadTimeout <= 0 {
		return nil, fmt.Errorf("lifetime and positive cache/load budgets required: %w", cache.ErrInvalidInput)
	}
	if err := lifetime.Err(); err != nil {
		return nil, err
	}
	base, err := NewStore(source, remote, namespace, opts.TTL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(lifetime)
	s := &ResilientStore{base: base, opts: opts, ctx: ctx, cancel: cancel}
	for i := range s.gates {
		s.gates[i] = make(chan struct{}, 1)
	}
	return s, nil
}

// Close stops admission, cancels loads/writes, and joins admitted work. The
// dependencies must honor contexts. A callback scheduled after closure is
// rejected before accessing a dependency. Calling Close repeatedly is safe.
func (s *ResilientStore) Close() {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.active.Wait()
}

// Key preserves the strict baseline's namespace and complete contact payload.
func (s *ResilientStore) Key(id int64) (string, error) { return s.base.Key(id) }

// Get coalesces concurrent lookups per ID. A write retires its earlier flight;
// existing waiters keep that result. Every caller can stop its own wait,
// including the caller that initiated the shared load.
func (s *ResilientStore) Get(ctx context.Context, id int64) (contacts.Contact, error) {
	if err := s.base.check(ctx, id); err != nil {
		return contacts.Contact{}, err
	}
	if err := s.begin(); err != nil {
		return contacts.Contact{}, err
	}
	defer s.active.Done()
	key, _ := s.Key(id)
	result := s.group.DoChan(key, func() (any, error) {
		if err := s.begin(); err != nil {
			return contacts.Contact{}, err
		}
		defer s.active.Done()
		load, cancel := context.WithTimeout(s.ctx, s.opts.LoadTimeout)
		defer cancel()
		return s.load(load, id, key)
	})
	s.emit("lookup_queued", id, nil)
	select {
	case <-ctx.Done():
		return contacts.Contact{}, ctx.Err()
	case <-s.ctx.Done():
		return contacts.Contact{}, ErrClosed
	case reply := <-result: // DoChan sends once; its channel is not closed.
		if err := ctx.Err(); err != nil {
			return contacts.Contact{}, err
		}
		if s.ctx.Err() != nil {
			return contacts.Contact{}, ErrClosed
		}
		if reply.Err != nil {
			return contacts.Contact{}, reply.Err
		}
		return reply.Val.(contacts.Contact), nil
	}
}

// load holds the gate through SQL read AND cache fill. A participating writer
// waits, commits afterward, then deletes that copy before releasing the gate.
// An old loader therefore cannot repopulate after that invalidation locally.
func (s *ResilientStore) load(ctx context.Context, id int64, key string) (contacts.Contact, error) {
	release, err := s.acquire(ctx, id)
	if err != nil {
		return contacts.Contact{}, err
	}
	defer release()
	command, cancel := context.WithTimeout(ctx, s.opts.CacheTimeout)
	value, err := s.base.cache.Get(command, key)
	cancel()
	if ctx.Err() != nil {
		return contacts.Contact{}, ctx.Err()
	}
	if err == nil {
		contact, decodeErr := decode(value, id)
		if decodeErr == nil {
			s.emit("cache_hit", id, nil)
			return contact, nil
		}
		s.emit("cache_payload_error", id, decodeErr)
	} else if errors.Is(err, cache.ErrMiss) {
		s.emit("cache_miss", id, nil)
	} else {
		s.emit("cache_read_error", id, err)
	}
	// Fall back only while the total load budget remains. SQL/domain errors
	// remain authoritative; a missing row never creates a negative-cache entry.
	s.emit("source_read", id, nil)
	contact, err := s.base.source.Get(ctx, id)
	if err != nil {
		return contacts.Contact{}, err
	}
	value, err = encode(contact, id)
	if err != nil {
		return contacts.Contact{}, err
	}
	command, cancel = context.WithTimeout(ctx, s.opts.CacheTimeout)
	err = s.base.cache.Set(command, key, value, s.base.ttl)
	cancel()
	if ctx.Err() != nil {
		return contacts.Contact{}, ctx.Err()
	}
	if err != nil {
		// A fill failure no longer discards the valid SQL result in this stage.
		s.emit("cache_fill_error", id, err)
	} else {
		s.emit("cache_fill", id, nil)
	}
	return contact, nil
}

// The same mutex guards admission and Close, preventing Add after shutdown has
// started waiting. It never covers network calls or observer callbacks.
func (s *ResilientStore) begin() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx.Err() != nil {
		return ErrClosed
	}
	s.active.Add(1)
	return nil
}

// A channel gate allows canceled callers to stop waiting for a local writer.
func (s *ResilientStore) acquire(ctx context.Context, id int64) (func(), error) {
	gate := s.gates[uint64(id)%uint64(len(s.gates))]
	select {
	case gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-gate
			return nil, err
		}
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *ResilientStore) emit(operation string, id int64, err error) {
	if s.opts.Observe != nil {
		s.opts.Observe(Event{Operation: operation, ID: id, Err: err})
	}
}
