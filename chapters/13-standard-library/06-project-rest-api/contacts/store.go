package contacts

import (
	"cmp"
	"context"
	"math"
	"slices"
	"sync"
)

// Store owns its data. Use its pointer; copying it would copy a mutex and share maps.
// Its zero value is ready to use. All operations return independent contact values.
// Mutex waits cannot be interrupted by context; operations check cancellation
// again after acquiring the lock. Cancellation does not undo a completed write.
type Store struct {
	mu sync.Mutex
	// These maps are two lookups (indexes) for the same contacts. byID owns
	// their values; byName reserves each name for an ID. Update both together.
	byID   map[int64]Contact
	byName map[string]int64
	lastID int64
}

// NewStore creates an empty in-memory store. Its maps are allocated on first create.
func NewStore() *Store { return &Store{} }

// Create validates input and assigns a new ID. A rejected create leaves both
// indexes and the next ID unchanged; duplicate normalized names return ErrNameTaken.
func (s *Store) Create(ctx context.Context, input Input) (Contact, error) {
	if err := ctx.Err(); err != nil {
		return Contact{}, err
	}
	input, err := normalize(input)
	if err != nil {
		return Contact{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Cancellation while waiting for the lock must also prevent a new write.
	if err := ctx.Err(); err != nil {
		return Contact{}, err
	}
	if _, exists := s.byName[input.Name]; exists {
		return Contact{}, ErrNameTaken
	}
	if s.lastID == math.MaxInt64 {
		return Contact{}, ErrIDExhausted
	}
	if s.byID == nil {
		s.byID = make(map[int64]Contact)
		s.byName = make(map[string]int64)
	}
	s.lastID++
	contact := Contact{ID: s.lastID, Name: input.Name, Email: input.Email, Phone: input.Phone}
	s.byID[contact.ID] = contact
	s.byName[contact.Name] = contact.ID
	return contact, nil
}

// Get returns an independent contact value, or ErrNotFound for an absent ID.
func (s *Store) Get(ctx context.Context, id int64) (Contact, error) {
	if err := ctx.Err(); err != nil {
		return Contact{}, err
	}
	if err := validateID(id); err != nil {
		return Contact{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Contact{}, err
	}
	contact, exists := s.byID[id]
	if !exists {
		return Contact{}, ErrNotFound
	}
	return contact, nil
}

// List copies the entire collection in increasing ID order. Search uses this
// snapshot internally; callers requesting a page should use Search.
func (s *Store) List(ctx context.Context) ([]Contact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]Contact, 0, len(s.byID))
	for _, contact := range s.byID {
		result = append(result, contact)
	}
	// Map order is unspecified. Stable ID order makes responses predictable.
	slices.SortFunc(result, func(a, b Contact) int { return cmp.Compare(a.ID, b.ID) })
	return result, nil
}

// Replace keeps the ID and replaces Name, Email, and Phone. Empty optional
// fields clear their previous values.
func (s *Store) Replace(ctx context.Context, id int64, input Input) (Contact, error) {
	return s.ReplaceIf(ctx, id, input, Conditions{})
}

// ReplaceIf also checks version conditions while holding the write lock.
func (s *Store) ReplaceIf(ctx context.Context, id int64, input Input, conditions Conditions) (Contact, error) {
	if err := ctx.Err(); err != nil {
		return Contact{}, err
	}
	if err := validateID(id); err != nil {
		return Contact{}, err
	}
	input, err := normalize(input)
	if err != nil {
		return Contact{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Contact{}, err
	}
	old, exists := s.byID[id]
	if !exists {
		return Contact{}, ErrNotFound
	}
	return s.replaceLocked(old, input, conditions)
}

// Patch changes only selected fields. Reading the current contact, applying
// changes, validating, and saving all happen under one lock. Calling Get and
// then Replace separately could overwrite another request's intervening edit.
func (s *Store) Patch(ctx context.Context, id int64, changes Changes) (Contact, error) {
	return s.PatchIf(ctx, id, changes, Conditions{})
}

// PatchIf applies selected fields and version conditions under the same lock.
// Invalid changes or failed conditions leave every field and index unchanged.
func (s *Store) PatchIf(ctx context.Context, id int64, changes Changes, conditions Conditions) (Contact, error) {
	if err := ctx.Err(); err != nil {
		return Contact{}, err
	}
	if err := validateID(id); err != nil {
		return Contact{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Contact{}, err
	}
	old, exists := s.byID[id]
	if !exists {
		return Contact{}, ErrNotFound
	}
	input := Input{Name: old.Name, Email: old.Email, Phone: old.Phone}
	if changes.Name != nil {
		input.Name = *changes.Name
	}
	if changes.Email != nil {
		input.Email = *changes.Email
	}
	if changes.Phone != nil {
		input.Phone = *changes.Phone
	}
	input, err := normalize(input)
	if err != nil {
		return Contact{}, err
	}
	return s.replaceLocked(old, input, conditions)
}

// The caller holds the lock and supplies normalized input. Check the name
// before changing either map so a rejected PUT or PATCH leaves both intact.
func (s *Store) replaceLocked(old Contact, input Input, conditions Conditions) (Contact, error) {
	if owner, exists := s.byName[input.Name]; exists && owner != old.ID {
		return Contact{}, ErrNameTaken
	}
	if err := conditions.check(old); err != nil {
		return Contact{}, err
	}
	updated := Contact{ID: old.ID, Name: input.Name, Email: input.Email, Phone: input.Phone}
	delete(s.byName, old.Name)
	s.byName[input.Name] = old.ID
	s.byID[old.ID] = updated
	return updated, nil
}

// Delete removes an existing contact without a version condition. Repeating a
// successful delete returns ErrNotFound; it does not recreate or reuse the ID.
func (s *Store) Delete(ctx context.Context, id int64) error {
	return s.DeleteIf(ctx, id, Conditions{})
}

// DeleteIf checks conditions and removes both indexes atomically. An absent
// contact returns ErrNotFound before conditions are evaluated.
func (s *Store) DeleteIf(ctx context.Context, id int64, conditions Conditions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateID(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	contact, exists := s.byID[id]
	if !exists {
		return ErrNotFound
	}
	if err := conditions.check(contact); err != nil {
		return err
	}
	delete(s.byID, id)
	delete(s.byName, contact.Name)
	return nil
}
