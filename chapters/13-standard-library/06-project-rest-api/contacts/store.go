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
type Store struct {
	mu     sync.Mutex
	byID   map[int64]Contact
	byName map[string]int64
	lastID int64
}

func NewStore() *Store { return &Store{} }

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

func (s *Store) Delete(ctx context.Context, id int64) error {
	return s.DeleteIf(ctx, id, Conditions{})
}

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
