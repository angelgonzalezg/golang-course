package contactcache

import (
	"context"
	"fmt"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// Create writes only to the source. No negative caching or ID reuse means
// there is no old entry for this newly assigned ID to invalidate.
func (s *ResilientStore) Create(ctx context.Context, input contacts.Input) (contacts.Contact, error) {
	if err := ctx.Err(); err != nil {
		return contacts.Contact{}, err
	}
	if err := s.begin(); err != nil {
		return contacts.Contact{}, err
	}
	defer s.active.Done()
	ctx, finish := s.operation(ctx)
	defer finish()
	return s.base.source.Create(ctx, input)
}

// Replace delegates full replacement and validation to the source.
func (s *ResilientStore) Replace(ctx context.Context, id int64, input contacts.Input) (contacts.Contact, error) {
	return s.write(ctx, id, func(operation context.Context) (contacts.Contact, error) {
		return s.base.source.Replace(operation, id, input)
	})
}

// Patch merges inside the SQL adapter's transaction, never with a cached copy.
func (s *ResilientStore) Patch(ctx context.Context, id int64, changes contacts.Changes) (contacts.Contact, error) {
	return s.write(ctx, id, func(operation context.Context) (contacts.Contact, error) {
		return s.base.source.Patch(operation, id, changes)
	})
}

// Delete commits source deletion before invalidation under the local gate.
func (s *ResilientStore) Delete(ctx context.Context, id int64) error {
	_, err := s.write(ctx, id, func(operation context.Context) (contacts.Contact, error) {
		return contacts.Contact{}, s.base.source.Delete(operation, id)
	})
	return err
}

// write never retries a SQL mutation. After commit, DEL gets a fresh bounded
// owner context even if the original request has just been canceled.
func (s *ResilientStore) write(ctx context.Context, id int64, mutate func(context.Context) (contacts.Contact, error)) (contacts.Contact, error) {
	if err := s.base.check(ctx, id); err != nil {
		return contacts.Contact{}, err
	}
	if err := s.begin(); err != nil {
		return contacts.Contact{}, err
	}
	defer s.active.Done()
	operation, finish := s.operation(ctx)
	defer finish()
	s.emit("write_wait", id, nil)
	release, err := s.acquire(operation, id)
	if err != nil {
		return contacts.Contact{}, err
	}
	defer release()
	contact, err := mutate(operation)
	if err != nil {
		return contacts.Contact{}, err
	}
	key, _ := s.Key(id)
	// New calls must not join a pre-write flight whose result delivery is still
	// pending. Forget leaves its existing waiters alone and runs under the gate.
	defer s.group.Forget(key)
	cleanup, cancel := context.WithTimeout(s.ctx, s.opts.CacheTimeout)
	defer cancel()
	if _, err := s.base.cache.Delete(cleanup, key); err != nil {
		s.emit("invalidation_error", id, err)
		return contact, &InvalidationError{ID: id, Err: err}
	}
	s.emit("invalidated", id, nil)
	return contact, nil
}

// Invalidate retries ONLY cache deletion after an InvalidationError. It makes
// no SQL write. A failed DEL can leave an old hit until repair or TTL, including
// after a cache reconnect; fallback alone does not solve that consistency issue.
func (s *ResilientStore) Invalidate(ctx context.Context, id int64) error {
	if err := s.base.check(ctx, id); err != nil {
		return err
	}
	if err := s.begin(); err != nil {
		return err
	}
	defer s.active.Done()
	operation, finish := s.operation(ctx)
	defer finish()
	release, err := s.acquire(operation, id)
	if err != nil {
		return err
	}
	defer release()
	key, _ := s.Key(id)
	defer s.group.Forget(key)
	command, cancel := context.WithTimeout(operation, s.opts.CacheTimeout)
	defer cancel()
	if _, err := s.base.cache.Delete(command, key); err != nil {
		return fmt.Errorf("retry contact invalidation: %w", err)
	}
	return nil
}

// Shutdown cancels writes too. Caller context still controls SQL; only the
// post-commit invalidation uses the independently bounded owner context.
func (s *ResilientStore) operation(caller context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(caller)
	stop := context.AfterFunc(s.ctx, cancel)
	return ctx, func() { stop(); cancel() }
}
