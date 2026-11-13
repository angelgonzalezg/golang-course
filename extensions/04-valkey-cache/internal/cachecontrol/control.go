// Package cachecontrol provides explicit fault switches and synchronization
// barriers for teaching demos/tests. It is not part of the cache policy itself.
package cachecontrol

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"example.com/course/cache-lab/contactcache"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

var ErrInjected = errors.New("controlled cache failure")

// FaultCache forwards healthy calls to the real cache. Switches simulate one
// command failing without stopping a learner's Compose services.
type FaultCache struct {
	contactcache.Cache
	FailGet, FailSet, FailDelete atomic.Bool
}

func (c *FaultCache) Get(ctx context.Context, key string) (string, error) {
	if c.FailGet.Load() {
		return "", ErrInjected
	}
	return c.Cache.Get(ctx, key)
}

func (c *FaultCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if c.FailSet.Load() {
		return ErrInjected
	}
	return c.Cache.Set(ctx, key, value, ttl)
}

func (c *FaultCache) Delete(ctx context.Context, key string) (bool, error) {
	if c.FailDelete.Load() {
		return false, ErrInjected
	}
	return c.Cache.Delete(ctx, key)
}

// Hold pauses AFTER the source read, retaining that exact snapshot. Releasing
// twice is safe, so tests can defer Release even after explicitly releasing it.
type Hold struct {
	Entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (h *Hold) Release() { h.once.Do(func() { close(h.release) }) }

// HeldSource counts authoritative Get attempts and can hold the next result.
// SQL itself remains real in integration tests and the console demonstration.
type HeldSource struct {
	contactcache.Source
	Reads atomic.Int64
	mu    sync.Mutex
	next  *Hold
}

func (s *HeldSource) HoldNextGet() *Hold {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next != nil {
		panic("previous held read has not started")
	}
	h := &Hold{Entered: make(chan struct{}), release: make(chan struct{})}
	s.next = h
	return h
}

func (s *HeldSource) Get(ctx context.Context, id int64) (contacts.Contact, error) {
	s.mu.Lock()
	h := s.next
	s.next = nil
	s.mu.Unlock()
	s.Reads.Add(1)
	contact, err := s.Source.Get(ctx, id)
	if h != nil {
		close(h.Entered)
		select {
		case <-h.release:
		case <-ctx.Done():
			return contacts.Contact{}, ctx.Err()
		}
	}
	return contact, err
}

// Probe records events safely and optionally signals a bounded scenario
// channel. Notifications do not block a production call or invoke store methods.
type Probe struct {
	mu      sync.Mutex
	events  []contactcache.Event
	watches map[string]chan contactcache.Event
}

func (p *Probe) Observe(event contactcache.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
	if ch := p.watches[event.Operation]; ch != nil {
		select {
		case ch <- event:
		default:
		}
	}
}

func (p *Probe) Watch(operation string, capacity int) <-chan contactcache.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.watches == nil {
		p.watches = make(map[string]chan contactcache.Event)
	}
	ch := make(chan contactcache.Event, capacity)
	p.watches[operation] = ch
	return ch
}

func (p *Probe) Count(operation string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	count := 0
	for _, event := range p.events {
		if event.Operation == operation {
			count++
		}
	}
	return count
}

// Wait lets demos/tests coordinate exact events under their own deadline.
func Wait[T any](ctx context.Context, ch <-chan T) (T, error) {
	select {
	case value := <-ch:
		return value, nil
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}
