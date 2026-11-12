// Package contactpg implements the shared contacts model with PostgreSQL and
// generated queries. Its service tables are separate from earlier SQL lessons.
package contactpg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"example.com/course/sql-lab/internal/servicegen"
	"github.com/jackc/pgx/v5/pgconn"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// Store borrows a pool prepared by Migrate. The application owns Close.
// PostgreSQL, rather than an in-process mutex or cache, coordinates writers.
type Store struct {
	db      *sql.DB
	queries *servicegen.Queries
}

// NewStore binds generated operations without opening a connection.
func NewStore(db *sql.DB) *Store { return &Store{db: db, queries: servicegen.New(db)} }

// Create is ordinary unkeyed creation. A repeated name is a business conflict,
// not replay; callers needing lost-response recovery use CreateOnce.
func (s *Store) Create(ctx context.Context, input contacts.Input) (contacts.Contact, error) {
	if err := ctx.Err(); err != nil {
		return contacts.Contact{}, err
	}
	input, err := contacts.NormalizeInput(input)
	if err != nil {
		return contacts.Contact{}, err
	}
	row, err := s.queries.CreateContact(ctx, servicegen.CreateContactParams{
		Name: []byte(input.Name), Email: []byte(input.Email), Phone: []byte(input.Phone),
	})
	if err != nil {
		return contacts.Contact{}, fmt.Errorf("create contact: %w", databaseError(ctx, err))
	}
	return contactFromRow(row), nil
}

// Get returns current data. Historical creation replay uses a different table.
func (s *Store) Get(ctx context.Context, id int64) (contacts.Contact, error) {
	if err := ctx.Err(); err != nil {
		return contacts.Contact{}, err
	}
	if err := contacts.ValidateID(id); err != nil {
		return contacts.Contact{}, err
	}
	row, err := s.queries.GetContact(ctx, id)
	if err != nil {
		return contacts.Contact{}, fmt.Errorf("get contact: %w", databaseError(ctx, err))
	}
	return contactFromRow(row), nil
}

// List reads one ID-ordered statement snapshot. Generated code closes rows and
// checks iteration errors. Every returned Contact is an independent Go value.
func (s *Store) List(ctx context.Context) ([]contacts.Contact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListContacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", databaseError(ctx, err))
	}
	values := make([]contacts.Contact, 0, len(rows))
	for _, row := range rows {
		values = append(values, contactFromRow(row))
	}
	return values, nil
}

// Search keeps the existing Go Unicode, sorting, and pagination contract.
// This teaching implementation reads the full snapshot: limit bounds response
// size, not database work or memory. It does not claim scalable SQL pagination.
func (s *Store) Search(ctx context.Context, query contacts.Query) (contacts.Page, error) {
	if err := ctx.Err(); err != nil {
		return contacts.Page{}, err
	}
	query, err := contacts.NormalizeQuery(query)
	if err != nil {
		return contacts.Page{}, err
	}
	values, err := s.List(ctx)
	if err != nil {
		return contacts.Page{}, err
	}
	return contacts.SelectPage(ctx, values, query)
}

// contactFromRow preserves every allowed string byte, including NUL. The shared
// API uses strings; bytea is only a storage detail and never an HTTP field type.
func contactFromRow(row servicegen.SqlServiceContact) contacts.Contact {
	return contacts.Contact{ID: row.ID, Name: string(row.Name), Email: string(row.Email), Phone: string(row.Phone)}
}

// databaseError classifies stable SQLSTATE/constraint values without losing
// driver causes. HTTP only exposes domain messages, not SQL or connection data.
func databaseError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ctx.Err(), err)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %w", contacts.ErrNotFound, err)
	}
	var cause *pgconn.PgError
	if errors.As(err, &cause) {
		if cause.Code == "23505" && cause.ConstraintName == "sql_service_contacts_name_key" {
			return fmt.Errorf("%w: %w", contacts.ErrNameTaken, err)
		}
		if cause.Code == "2200H" {
			return fmt.Errorf("%w: %w", contacts.ErrIDExhausted, err)
		}
	}
	return err
}
