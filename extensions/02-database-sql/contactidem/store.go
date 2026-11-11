// Package contactidem saves a create result and its business row in one SQL
// transaction. Its independent sql_idem_* tables keep earlier lessons runnable.
package contactidem

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"example.com/course/sql-lab/contactsql"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrKeyReuse means this caller/key already identifies different request data.
// Retrying the original data is allowed; changing the key blindly can repeat work.
var ErrKeyReuse = errors.New("idempotency key is bound to another request")

// Result contains the original committed creation snapshot. Replayed describes
// how this invocation obtained it; it is not part of the saved business result.
type Result struct {
	Contact  contactsql.Contact
	Replayed bool
}

// Store borrows a usable pool whose schema has been prepared through Migrate.
// It uses PostgreSQL coordination, so separate pools/processes share the policy.
type Store struct{ db *sql.DB }

// NewStore does not open or close the supplied pool.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// CreateOnce binds (subject, key) to the original Name/Email request values and
// saves one successful result. subject must come from a trusted caller identity;
// this database API does not authenticate strings supplied by a remote client.
// Errors before commit leave no request record. A commit/transport error can be
// ambiguous: retry the same subject/key/data to discover a saved success safely.
func (s *Store) CreateOnce(ctx context.Context, subject, key string, input contactsql.Input) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	normalized, fingerprint, err := prepare(subject, key, input)
	if err != nil {
		return Result{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Result{}, fmt.Errorf("begin keyed create: %w", databaseError(ctx, err))
	}
	// Any early failure discards both writes. After Commit this deferred call
	// returns sql.ErrTxDone and cannot undo the committed result.
	defer tx.Rollback()

	// 1. Serialize only this caller/key operation across database clients.
	// A transaction-scoped lock is released by commit/rollback; waiting on it
	// observes ctx through ExecContext. No in-process mutex supplies the guarantee.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1::bigint)", lockID(subject, key)); err != nil {
		return Result{}, fmt.Errorf("lock keyed create: %w", databaseError(ctx, err))
	}

	// 2. Read AFTER obtaining the lock, in a separate statement. ReadCommitted
	// lets this query see a previous holder's newly committed request record.
	saved, err := readResult(ctx, tx, subject, key)
	replayed := err == nil
	switch {
	case replayed:
		if !bytes.Equal(saved.fingerprint, fingerprint[:]) {
			return Result{}, ErrKeyReuse
		}
	case errors.Is(err, sql.ErrNoRows):
		// 3. A new operation inserts the contact AND its complete result record
		// through this same tx. A failed result INSERT rolls back the contact too.
		saved.contact, err = scanContact(tx.QueryRowContext(ctx, `
			INSERT INTO sql_idem_contacts (name, email) VALUES ($1, $2)
			RETURNING id, name, email`, normalized.Name, normalized.Email))
		if err != nil {
			return Result{}, fmt.Errorf("create contact: %w", databaseError(ctx, err))
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO sql_idem_create_requests
			(subject, request_key, request_hash, response_id, response_name, response_email)
			VALUES ($1, $2, $3, $4, $5, $6)`, subject, key, fingerprint[:],
			saved.contact.ID, saved.contact.Name, saved.contact.Email); err != nil {
			return Result{}, fmt.Errorf("save creation result: %w", databaseError(ctx, err))
		}
	default:
		return Result{}, fmt.Errorf("read creation result: %w", databaseError(ctx, err))
	}

	// 4. Report success only after the transaction finishes. A replay also ends
	// its read transaction here, releasing the key lock before returning.
	if err := tx.Commit(); err != nil {
		return Result{}, fmt.Errorf("commit keyed create: %w", databaseError(ctx, err))
	}
	return Result{Contact: saved.contact, Replayed: replayed}, nil
}

type savedResult struct {
	fingerprint []byte
	contact     contactsql.Contact
}

// readResult returns the saved snapshot, not a join against the current contact.
// The original creation can still replay after that contact changes or is deleted.
func readResult(ctx context.Context, tx *sql.Tx, subject, key string) (savedResult, error) {
	var saved savedResult
	var email sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT request_hash, response_id, response_name, response_email
		FROM sql_idem_create_requests WHERE subject = $1 AND request_key = $2`, subject, key).
		Scan(&saved.fingerprint, &saved.contact.ID, &saved.contact.Name, &email)
	if err != nil {
		return savedResult{}, err
	}
	if email.Valid {
		saved.contact.Email = &email.String
	}
	return saved, nil
}

// scanContact converts SQL presence into an independent optional Go value.
func scanContact(row interface{ Scan(...any) error }) (contactsql.Contact, error) {
	var contact contactsql.Contact
	var email sql.NullString
	if err := row.Scan(&contact.ID, &contact.Name, &email); err != nil {
		return contactsql.Contact{}, err
	}
	if email.Valid {
		contact.Email = &email.String
	}
	return contact, nil
}

// Retain both caller cancellation and driver causes; only this lesson's named
// unique-name violation maps to the shared business error.
func databaseError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ctx.Err(), err)
	}
	var cause *pgconn.PgError
	if errors.As(err, &cause) && cause.Code == "23505" && cause.ConstraintName == "sql_idem_contacts_name_key" {
		return fmt.Errorf("%w: %w", contactsql.ErrNameTaken, err)
	}
	return err
}
