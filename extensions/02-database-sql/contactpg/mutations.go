package contactpg

import (
	"context"
	"database/sql"
	"fmt"

	"example.com/course/sql-lab/internal/servicegen"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// Replace is an unconditional full replacement, retaining the assigned ID.
func (s *Store) Replace(ctx context.Context, id int64, input contacts.Input) (contacts.Contact, error) {
	return s.ReplaceIf(ctx, id, input, contacts.Conditions{})
}

// ReplaceIf validates all supplied fields before looking up the resource,
// matching the memory store. Omitted optional fields are replaced by empty text.
func (s *Store) ReplaceIf(ctx context.Context, id int64, input contacts.Input, conditions contacts.Conditions) (contacts.Contact, error) {
	if err := ctx.Err(); err != nil {
		return contacts.Contact{}, err
	}
	if err := contacts.ValidateID(id); err != nil {
		return contacts.Contact{}, err
	}
	input, err := contacts.NormalizeInput(input)
	if err != nil {
		return contacts.Contact{}, err
	}
	return s.update(ctx, id, conditions, func(contacts.Contact) (contacts.Input, error) { return input, nil })
}

// Patch is an unconditional partial update. nil fields keep their old values.
func (s *Store) Patch(ctx context.Context, id int64, changes contacts.Changes) (contacts.Contact, error) {
	return s.PatchIf(ctx, id, changes, contacts.Conditions{})
}

// PatchIf reads, merges, validates, and checks conditions while the database row
// is locked. Independent concurrent field edits cannot overwrite each other.
func (s *Store) PatchIf(ctx context.Context, id int64, changes contacts.Changes, conditions contacts.Conditions) (contacts.Contact, error) {
	if err := ctx.Err(); err != nil {
		return contacts.Contact{}, err
	}
	if err := contacts.ValidateID(id); err != nil {
		return contacts.Contact{}, err
	}
	return s.update(ctx, id, conditions, func(old contacts.Contact) (contacts.Input, error) {
		input := contacts.Input{Name: old.Name, Email: old.Email, Phone: old.Phone}
		if changes.Name != nil {
			input.Name = *changes.Name
		}
		if changes.Email != nil {
			input.Email = *changes.Email
		}
		if changes.Phone != nil {
			input.Phone = *changes.Phone
		}
		return contacts.NormalizeInput(input)
	})
}

// update is the common transaction boundary for PUT and PATCH. The callback
// supplies normalized replacement values only after the latest row is locked.
func (s *Store) update(ctx context.Context, id int64, conditions contacts.Conditions, inputFor func(contacts.Contact) (contacts.Input, error)) (contacts.Contact, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return contacts.Contact{}, databaseError(ctx, err)
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	// 1. Lock the current row. A competing writer waits, then sees fresh data.
	row, err := queries.LockContact(ctx, id)
	if err != nil {
		return contacts.Contact{}, fmt.Errorf("lock contact: %w", databaseError(ctx, err))
	}
	old := contactFromRow(row)
	input, err := inputFor(old)
	if err != nil {
		return contacts.Contact{}, err
	}
	// 2. Preserve normal business-error priority before checking validators.
	taken, err := queries.NameTaken(ctx, servicegen.NameTakenParams{Name: []byte(input.Name), ID: id})
	if err != nil {
		return contacts.Contact{}, databaseError(ctx, err)
	}
	if taken {
		return contacts.Contact{}, contacts.ErrNameTaken
	}
	if err := conditions.Check(old); err != nil {
		return contacts.Contact{}, err
	}
	// 3. The write uses the SAME transaction. Its unique constraint also checks
	// names acquired by a concurrent transaction after the availability query.
	updated, err := queries.ReplaceContact(ctx, servicegen.ReplaceContactParams{
		ID: id, Name: []byte(input.Name), Email: []byte(input.Email), Phone: []byte(input.Phone),
	})
	if err != nil {
		return contacts.Contact{}, fmt.Errorf("replace contact: %w", databaseError(ctx, err))
	}
	if err := tx.Commit(); err != nil {
		return contacts.Contact{}, databaseError(ctx, err)
	}
	return contactFromRow(updated), nil
}

// Delete performs unconditional removal; repeated removal returns ErrNotFound.
func (s *Store) Delete(ctx context.Context, id int64) error {
	return s.DeleteIf(ctx, id, contacts.Conditions{})
}

// DeleteIf holds the row lock from version checking through deletion and commit.
// Missing rows are reported before evaluating conditions, as in the memory store.
func (s *Store) DeleteIf(ctx context.Context, id int64, conditions contacts.Conditions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := contacts.ValidateID(id); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return databaseError(ctx, err)
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	row, err := queries.LockContact(ctx, id)
	if err != nil {
		return databaseError(ctx, err)
	}
	if err := conditions.Check(contactFromRow(row)); err != nil {
		return err
	}
	if _, err := queries.DeleteContact(ctx, id); err != nil {
		return databaseError(ctx, err)
	}
	return databaseError(ctx, tx.Commit())
}
