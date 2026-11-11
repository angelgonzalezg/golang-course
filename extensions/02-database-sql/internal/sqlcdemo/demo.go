// Package sqlcdemo compares handwritten SQL with generated query bindings.
// Both stores use the same table; only the Go query plumbing changes.
package sqlcdemo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"time"

	"example.com/course/sql-lab/contactsql"
	"example.com/course/sql-lab/contactsqlc"
	"example.com/course/sql-lab/internal/sqlcgen"
)

// Run owns one pool and removes only its randomly named contact. It uses the
// fundamentals schema, leaving transaction/idempotency lesson tables untouched.
func Run(ctx context.Context, dsn string, output io.Writer) (result error) {
	db, err := contactsql.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := contactsql.EnsureSchema(ctx, db); err != nil {
		return err
	}
	manual, generated := contactsql.NewStore(db), contactsqlc.NewStore(db)

	fmt.Fprintln(output, "== 1. Generated create, manual read ==")
	name := "sqlc-" + rand.Text()[:8]
	input := contactsql.Input{Name: " " + name + " "}
	created, err := generated.Create(ctx, input)
	if err != nil {
		return err
	}
	// Cleanup has a fresh budget even if a later operation exhausts ctx.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := generated.Delete(cleanup, created.ID); err != nil && !errors.Is(err, contactsql.ErrNotFound) {
			result = errors.Join(result, fmt.Errorf("clean up sqlc demo: %w", err))
		}
	}()
	found, err := manual.Get(ctx, created.ID)
	if err != nil || found.Name != name || found.Email != nil {
		return fmt.Errorf("manual read did not match generated creation: %v", err)
	}
	fmt.Fprintln(output, "Generated and manual stores read the same PostgreSQL row.")

	fmt.Fprintln(output, "\n== 2. Preserve optional values in both directions ==")
	empty := ""
	if _, err := manual.UpdateEmail(ctx, created.ID, &empty); err != nil {
		return err
	}
	found, err = generated.Get(ctx, created.ID)
	if err != nil || found.Email == nil || *found.Email != "" {
		return fmt.Errorf("generated read lost empty email: %v", err)
	}
	if _, err := generated.UpdateEmail(ctx, created.ID, nil); err != nil {
		return err
	}
	found, err = manual.Get(ctx, created.ID)
	if err != nil || found.Email != nil {
		return fmt.Errorf("manual read lost NULL email: %v", err)
	}
	fmt.Fprintln(output, "Both stores preserve empty text versus SQL NULL.")

	// Raw generated queries also accept a caller-owned transaction. WithTx
	// selects that transaction; it does not create a transaction or decide its end.
	fmt.Fprintln(output, "\n== 3. Bind generated queries to a transaction ==")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queries := sqlcgen.New(db).WithTx(tx)
	changed, err := queries.UpdateContactEmail(ctx, sqlcgen.UpdateContactEmailParams{
		ID: created.ID, Email: sql.NullString{String: "temporary@example.com", Valid: true},
	})
	if err != nil || !changed.Email.Valid || changed.Email.String != "temporary@example.com" {
		return fmt.Errorf("transactional update did not return its write: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		return err
	}
	found, err = manual.Get(ctx, created.ID)
	if err != nil || found.Email != nil {
		return fmt.Errorf("rolled-back email reached committed data: %v", err)
	}
	fmt.Fprintln(output, "WithTx uses our transaction; rollback discards its update.")

	fmt.Fprintln(output, "\n== 4. Keep the same application errors ==")
	if _, err := generated.Create(ctx, input); !errors.Is(err, contactsql.ErrNameTaken) {
		return fmt.Errorf("expected duplicate-name error, got %v", err)
	}
	if err := generated.Delete(ctx, created.ID); err != nil {
		return err
	}
	if _, err := generated.Get(ctx, created.ID); !errors.Is(err, contactsql.ErrNotFound) || !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("expected missing-row error and SQL cause, got %v", err)
	}
	fmt.Fprintln(output, "Same ErrNameTaken/ErrNotFound contract; SQL causes remain inspectable.")
	return nil
}
