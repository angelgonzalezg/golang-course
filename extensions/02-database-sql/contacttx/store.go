package contacttx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"example.com/course/sql-lab/contactsql"
	"github.com/jackc/pgx/v5/pgconn"
)

// Contact adds a monotonically increasing row version to the teaching model.
// It is a counter, not the REST project's content-derived ETag.
type Contact struct {
	ID      int64
	Name    string
	Email   *string
	Version int64
}

// ErrVersionConflict means no row matched both ID and expected version. A missing
// ID has the same outcome here; transport-specific error distinctions come later.
var ErrVersionConflict = errors.New("contact missing or version changed")

// Store borrows a pool whose schema has reached LatestVersion through Migrate.
type Store struct{ db *sql.DB }

// NewStore does not own or close db. Run migrations before serving operations.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// CreateBatch inserts 1–10 contacts atomically. A database error on any insert
// rolls back earlier inserts; identity sequence numbers can still have gaps.
func (s *Store) CreateBatch(ctx context.Context, inputs []contactsql.Input) ([]Contact, error) {
	if len(inputs) < 1 || len(inputs) > 10 {
		return nil, fmt.Errorf("batch must contain 1–10 contacts: %w", contactsql.ErrInvalidInput)
	}
	normalized := make([]contactsql.Input, len(inputs))
	for i, input := range inputs {
		value, err := contactsql.NormalizeInput(input)
		if err != nil {
			return nil, fmt.Errorf("batch item %d: %w", i, err)
		}
		normalized[i] = value
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin batch: %w", err)
	}
	defer tx.Rollback()
	result := make([]Contact, 0, len(inputs))
	for i, input := range normalized {
		// Use tx for every statement in the unit of work. Calling s.db here
		// would borrow a different connection outside this transaction.
		contact, err := scanContact(tx.QueryRowContext(ctx, `
			INSERT INTO sql_tx_contacts (name, email) VALUES ($1, $2)
			RETURNING id, name, email, version`, input.Name, input.Email))
		if err != nil {
			return nil, fmt.Errorf("insert batch item %d: %w", i, databaseError(ctx, err))
		}
		result = append(result, contact)
	}
	// Do not report success or return the tentative rows until Commit succeeds.
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit batch: %w", err)
	}
	return result, nil
}

// ReplaceIfVersion replaces the editable fields and records the version change
// in the same transaction. A stale/missing row produces ErrVersionConflict.
// The caller decides what to do after a conflict; this method does not retry.
func (s *Store) ReplaceIfVersion(ctx context.Context, id, expected int64, input contactsql.Input) (Contact, error) {
	if id <= 0 || expected <= 0 {
		return Contact{}, fmt.Errorf("id and expected version must be positive: %w", contactsql.ErrInvalidInput)
	}
	input, err := contactsql.NormalizeInput(input)
	if err != nil {
		return Contact{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Contact{}, fmt.Errorf("begin replacement: %w", err)
	}
	defer tx.Rollback()
	// Compare and increment inside one SQL UPDATE. Two writers using version 1
	// cannot both replace it: after one commits, the other predicate no longer matches.
	contact, err := scanContact(tx.QueryRowContext(ctx, `
		UPDATE sql_tx_contacts SET name = $3, email = $4, version = version + 1
		WHERE id = $1 AND version = $2 RETURNING id, name, email, version`,
		id, expected, input.Name, input.Email))
	if errors.Is(err, sql.ErrNoRows) {
		return Contact{}, ErrVersionConflict
	}
	if err != nil {
		return Contact{}, fmt.Errorf("replace contact: %w", databaseError(ctx, err))
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sql_tx_contact_changes (contact_id, from_version, to_version)
		VALUES ($1, $2, $3)`, id, expected, contact.Version); err != nil {
		return Contact{}, fmt.Errorf("record contact change: %w", databaseError(ctx, err))
	}
	if err := tx.Commit(); err != nil {
		return Contact{}, fmt.Errorf("commit replacement: %w", err)
	}
	return contact, nil
}

// Get reads a committed contact value through the pool.
func (s *Store) Get(ctx context.Context, id int64) (Contact, error) {
	contact, err := scanContact(s.db.QueryRowContext(ctx,
		"SELECT id, name, email, version FROM sql_tx_contacts WHERE id = $1", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Contact{}, contactsql.ErrNotFound
	}
	if err != nil {
		return Contact{}, fmt.Errorf("get versioned contact: %w", databaseError(ctx, err))
	}
	return contact, nil
}

// Delete removes a contact and its change records through ON DELETE CASCADE.
func (s *Store) Delete(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM sql_tx_contacts WHERE id = $1", id)
	if err != nil {
		return databaseError(ctx, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return contactsql.ErrNotFound
	}
	return nil
}

func scanContact(row interface{ Scan(...any) error }) (Contact, error) {
	var contact Contact
	var email sql.NullString
	if err := row.Scan(&contact.ID, &contact.Name, &email, &contact.Version); err != nil {
		return Contact{}, err
	}
	if email.Valid {
		contact.Email = &email.String
	}
	return contact, nil
}

func databaseError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ctx.Err(), err)
	}
	var cause *pgconn.PgError
	if errors.As(err, &cause) && cause.Code == "23505" && cause.ConstraintName == "sql_tx_contacts_name_key" {
		return fmt.Errorf("%w: %w", contactsql.ErrNameTaken, err)
	}
	return err
}
