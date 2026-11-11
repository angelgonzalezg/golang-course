// Package contactsqlc adapts sqlc-generated queries to the same teaching model
// and behavior as contactsql.Store. Both implementations use sql_lab_contacts.
package contactsqlc

import (
	"context"
	"database/sql"
	"fmt"

	"example.com/course/sql-lab/contactsql"
	"example.com/course/sql-lab/internal/sqlcgen"
)

// Store keeps generated database details behind the lesson's Contact/Input API.
// Validation and error classification remain handwritten application decisions.
type Store struct{ queries *sqlcgen.Queries }

// NewStore borrows a prepared pool; the caller remains responsible for Close.
// Constructing Queries neither opens a connection nor runs schema migrations.
func NewStore(db *sql.DB) *Store { return &Store{queries: sqlcgen.New(db)} }

// Create normalizes the name, converts optional email, then calls generated SQL.
// sqlc supplies typed arguments and Scan; it does not enforce input text rules.
func (s *Store) Create(ctx context.Context, input contactsql.Input) (contactsql.Contact, error) {
	input, err := contactsql.NormalizeInput(input)
	if err != nil {
		return contactsql.Contact{}, err
	}
	row, err := s.queries.CreateContact(ctx, sqlcgen.CreateContactParams{
		Name: input.Name, Email: nullableEmail(input.Email),
	})
	if err != nil {
		return contactsql.Contact{}, fmt.Errorf("create contact: %w", contactsql.QueryError(ctx, err))
	}
	return contactFromRow(row), nil
}

// Get preserves both ErrNotFound and sql.ErrNoRows when no contact matches.
func (s *Store) Get(ctx context.Context, id int64) (contactsql.Contact, error) {
	if err := contactsql.ValidateID(id); err != nil {
		return contactsql.Contact{}, err
	}
	row, err := s.queries.GetContact(ctx, id)
	if err != nil {
		return contactsql.Contact{}, fmt.Errorf("get contact: %w", contactsql.QueryError(ctx, err))
	}
	return contactFromRow(row), nil
}

// List retains the manual store's 1–100 limit and non-nil empty slice contract.
// Generated code owns Rows.Close/Rows.Err and scanning; inspect queries.sql.go.
func (s *Store) List(ctx context.Context, limit int) ([]contactsql.Contact, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("limit must be between 1 and 100: %w", contactsql.ErrInvalidInput)
	}
	// Validation bounds the conversion to the generated PostgreSQL integer type.
	rows, err := s.queries.ListContacts(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", contactsql.QueryError(ctx, err))
	}
	contacts := make([]contactsql.Contact, 0, len(rows))
	for _, row := range rows {
		contacts = append(contacts, contactFromRow(row))
	}
	return contacts, nil
}

// UpdateEmail assigns NULL for nil and empty text for a pointer to "". These are
// the SQL lesson's field-assignment rules, not HTTP PATCH omission semantics.
func (s *Store) UpdateEmail(ctx context.Context, id int64, email *string) (contactsql.Contact, error) {
	if err := contactsql.ValidateID(id); err != nil {
		return contactsql.Contact{}, err
	}
	if err := contactsql.ValidateEmail(email); err != nil {
		return contactsql.Contact{}, err
	}
	row, err := s.queries.UpdateContactEmail(ctx, sqlcgen.UpdateContactEmailParams{
		ID: id, Email: nullableEmail(email),
	})
	if err != nil {
		return contactsql.Contact{}, fmt.Errorf("update email: %w", contactsql.QueryError(ctx, err))
	}
	return contactFromRow(row), nil
}

// Delete uses :execrows' generated count to distinguish removal from absence.
func (s *Store) Delete(ctx context.Context, id int64) error {
	if err := contactsql.ValidateID(id); err != nil {
		return err
	}
	count, err := s.queries.DeleteContact(ctx, id)
	if err != nil {
		return fmt.Errorf("delete contact: %w", contactsql.QueryError(ctx, err))
	}
	if count == 0 {
		return contactsql.ErrNotFound
	}
	return nil
}

// nullableEmail copies the pointed-to value into the generated SQL parameter.
// Valid distinguishes a present empty string from SQL NULL.
func nullableEmail(email *string) sql.NullString {
	if email == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *email, Valid: true}
}

// contactFromRow keeps generated types inside this adapter. Each optional result
// owns its email value; editing it cannot mutate another result or the database.
func contactFromRow(row sqlcgen.SqlLabContact) contactsql.Contact {
	contact := contactsql.Contact{ID: row.ID, Name: row.Name}
	if row.Email.Valid {
		contact.Email = &row.Email.String
	}
	return contact
}
