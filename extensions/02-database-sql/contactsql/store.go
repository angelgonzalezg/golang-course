package contactsql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

// Contact is one SQL result. A nil Email means SQL NULL; a pointer to "" means
// an explicitly stored empty string. Each result owns its optional email value.
type Contact struct {
	ID    int64
	Name  string
	Email *string
}

// Input holds the fields accepted by this introductory SQL model.
// Name is trimmed and required; Email retains NULL versus empty-string presence.
// Keep the supplied values stable until the operation returns.
type Input struct {
	Name  string
	Email *string
}

var (
	ErrInvalidInput = errors.New("invalid contact input")
	ErrNotFound     = errors.New("contact not found")
	ErrNameTaken    = errors.New("contact name already exists")
)

// Store borrows a database pool. Its owner, rather than Store, closes that pool.
// Methods use concurrent-safe sql.DB; PostgreSQL enforces the unique name index.
type Store struct{ db *sql.DB }

// NewStore connects the lesson operations to an already opened pool.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Create inserts one contact and reads the server-generated ID with RETURNING.
func (s *Store) Create(ctx context.Context, input Input) (Contact, error) {
	input, err := normalize(input)
	if err != nil {
		return Contact{}, err
	}
	// Values travel separately from SQL text. A name containing quotes remains
	// data; it cannot become a command. A nil *string is sent as SQL NULL.
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO sql_lab_contacts (name, email) VALUES ($1, $2)
		RETURNING id, name, email`, input.Name, input.Email)
	contact, err := scanContact(row)
	if err != nil {
		return Contact{}, fmt.Errorf("create contact: %w", QueryError(ctx, err))
	}
	return contact, nil
}

// Get retrieves one contact. QueryRowContext reports missing rows through Scan.
func (s *Store) Get(ctx context.Context, id int64) (Contact, error) {
	if err := ValidateID(id); err != nil {
		return Contact{}, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, email FROM sql_lab_contacts WHERE id = $1`, id)
	contact, err := scanContact(row)
	if err != nil {
		return Contact{}, fmt.Errorf("get contact: %w", QueryError(ctx, err))
	}
	return contact, nil
}

// List returns at most limit contacts in increasing ID order. A valid limit is
// 1–100. An empty result is an empty slice, not a missing-row error.
func (s *Store) List(ctx context.Context, limit int) ([]Contact, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("limit must be between 1 and 100: %w", ErrInvalidInput)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, email FROM sql_lab_contacts ORDER BY id LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", QueryError(ctx, err))
	}
	// Rows holds a connection while results are being read. Close on every path,
	// including an early return from Scan, so the pool can reuse the connection.
	defer rows.Close()
	contacts := make([]Contact, 0)
	for rows.Next() {
		contact, err := scanContact(rows)
		if err != nil {
			return nil, fmt.Errorf("scan contact: %w", QueryError(ctx, err))
		}
		contacts = append(contacts, contact)
	}
	// A successful QueryContext does not guarantee that iteration also succeeded.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contacts: %w", QueryError(ctx, err))
	}
	return contacts, nil
}

// UpdateEmail assigns one field. nil stores NULL; a pointer to "" stores empty
// text. This lesson method does not implement HTTP PUT or PATCH semantics.
func (s *Store) UpdateEmail(ctx context.Context, id int64, email *string) (Contact, error) {
	if err := ValidateID(id); err != nil {
		return Contact{}, err
	}
	if err := ValidateEmail(email); err != nil {
		return Contact{}, err
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE sql_lab_contacts SET email = $2 WHERE id = $1
		RETURNING id, name, email`, id, email)
	contact, err := scanContact(row)
	if err != nil {
		return Contact{}, fmt.Errorf("update email: %w", QueryError(ctx, err))
	}
	return contact, nil
}

// Delete removes one row. ExecContext supplies a result rather than rows;
// RowsAffected distinguishes a successful delete from a missing contact.
func (s *Store) Delete(ctx context.Context, id int64) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM sql_lab_contacts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete contact: %w", QueryError(ctx, err))
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted row count: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

// Both *sql.Row and *sql.Rows provide Scan. This small interface keeps NULL
// conversion in one place without hiding SQL behind a large abstraction.
func scanContact(row interface{ Scan(...any) error }) (Contact, error) {
	var contact Contact
	var email sql.NullString
	if err := row.Scan(&contact.ID, &contact.Name, &email); err != nil {
		return Contact{}, err
	}
	if email.Valid {
		// Valid separates SQL NULL from a stored empty string. Each scan creates
		// its own optional value, so editing a result does not update PostgreSQL.
		contact.Email = &email.String
	}
	return contact, nil
}

// normalize checks this model's text rules before sending SQL. It returns a
// trimmed name without changing the caller's Input struct; email is borrowed.
func normalize(input Input) (Input, error) {
	input.Name = strings.TrimSpace(input.Name)
	if !utf8.ValidString(input.Name) || strings.ContainsRune(input.Name, 0) ||
		input.Name == "" || utf8.RuneCountInString(input.Name) > 80 {
		return Input{}, fmt.Errorf("name must contain 1–80 UTF-8 runes without NUL: %w", ErrInvalidInput)
	}
	if err := ValidateEmail(input.Email); err != nil {
		return Input{}, err
	}
	return input, nil
}

// ValidateEmail checks optional text without losing NULL versus empty presence.
// Both the manual store and the sqlc adapter use this same contact-model rule.
func ValidateEmail(email *string) error {
	if email != nil && (!utf8.ValidString(*email) || strings.ContainsRune(*email, 0) || utf8.RuneCountInString(*email) > 254) {
		return fmt.Errorf("email must contain at most 254 UTF-8 runes without NUL: %w", ErrInvalidInput)
	}
	return nil
}

// ValidateID rejects invalid identifiers before either store issues a query.
func ValidateID(id int64) error {
	if id <= 0 {
		return fmt.Errorf("id must be positive: %w", ErrInvalidInput)
	}
	return nil
}

// QueryError gives both stores the same business errors while preserving SQL,
// driver, and context causes. It is specific to the sql_lab_contacts schema.
func QueryError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		// Preserve both cancellation and the driver cause for errors.Is/As.
		return fmt.Errorf("%w: %w", ctx.Err(), err)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" &&
		postgresError.ConstraintName == "sql_lab_contacts_name_key" {
		// Inspect a stable SQLSTATE and constraint name, not localized error text.
		return fmt.Errorf("%w: %w", ErrNameTaken, err)
	}
	return err
}
