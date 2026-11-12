package contactpg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"example.com/course/sql-lab/internal/servicegen"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// CreateOnce persists a successful creation and its response snapshot together.
// Only committed successes bind a key. Commit/transport errors can be ambiguous:
// retry the SAME caller/key/decoded fields to resolve the outcome safely.
func (s *Store) CreateOnce(ctx context.Context, subject, key string, input contacts.Input) (contacts.CreationResult, error) {
	if err := ctx.Err(); err != nil {
		return contacts.CreationResult{}, err
	}
	if err := contacts.ValidateRequestIdentity(subject, key); err != nil {
		return contacts.CreationResult{}, err
	}
	normalized, err := contacts.NormalizeInput(input)
	if err != nil {
		return contacts.CreationResult{}, err
	}
	// Bind original decoded values, before trimming. JSON formatting/member order
	// do not affect identity; changing a field's whitespace does. Version this
	// fixed typed encoding deliberately while stored results remain replayable.
	body, err := json.Marshal(struct {
		Format int    `json:"format"`
		Name   string `json:"name"`
		Email  string `json:"email"`
		Phone  string `json:"phone"`
	}{1, input.Name, input.Email, input.Phone})
	if err != nil {
		return contacts.CreationResult{}, err
	}
	fingerprint := sha256.Sum256(body)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return contacts.CreationResult{}, databaseError(ctx, err)
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	// 1. Transaction locks coordinate separate pools/processes and release on
	// commit/rollback. ExecContext lets this caller cancel while waiting.
	if err := queries.LockCreation(ctx, creationLock(subject, key)); err != nil {
		return contacts.CreationResult{}, databaseError(ctx, err)
	}
	// 2. Read in a fresh ReadCommitted statement after the lock wait. A duplicate
	// now sees the previous holder's committed fingerprint and complete result.
	saved, err := queries.CreationResult(ctx, servicegen.CreationResultParams{Subject: subject, RequestKey: key})
	replayed := err == nil
	var contact contacts.Contact
	switch {
	case replayed:
		if !bytes.Equal(saved.RequestHash, fingerprint[:]) {
			return contacts.CreationResult{}, contacts.ErrKeyReuse
		}
		contact = contacts.Contact{ID: saved.ResponseID, Name: string(saved.ResponseName), Email: string(saved.ResponseEmail), Phone: string(saved.ResponsePhone)}
	case errors.Is(err, sql.ErrNoRows):
		// 3. Both INSERTs use this transaction. A failed result INSERT or canceled
		// operation rolls back the contact too; there is no committed pending key.
		row, err := queries.CreateContact(ctx, servicegen.CreateContactParams{
			Name: []byte(normalized.Name), Email: []byte(normalized.Email), Phone: []byte(normalized.Phone),
		})
		if err != nil {
			return contacts.CreationResult{}, databaseError(ctx, err)
		}
		contact = contactFromRow(row)
		if err := queries.SaveCreationResult(ctx, servicegen.SaveCreationResultParams{
			Subject: subject, RequestKey: key, RequestHash: fingerprint[:], ResponseID: contact.ID,
			ResponseName: row.Name, ResponseEmail: row.Email, ResponsePhone: row.Phone,
		}); err != nil {
			return contacts.CreationResult{}, databaseError(ctx, err)
		}
	default:
		return contacts.CreationResult{}, fmt.Errorf("read saved creation: %w", databaseError(ctx, err))
	}
	// 4. Report success only after commit. A replay ends its read transaction
	// here too; the returned result never comes from the contact's current row.
	if err := tx.Commit(); err != nil {
		return contacts.CreationResult{}, databaseError(ctx, err)
	}
	return contacts.CreationResult{Contact: contact, Replayed: replayed}, nil
}

// A stable operation prefix separates these locks from earlier lessons. A rare
// 64-bit collision causes extra waiting, not identity confusion: the exact
// (subject, request_key) primary key still identifies each stored operation.
func creationLock(subject, key string) int64 {
	hash := sha256.Sum256([]byte("course/contacts/create/v1\x00" + subject + "\x00" + key))
	return int64(binary.BigEndian.Uint64(hash[:8]))
}
