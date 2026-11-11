// Package demo runs a finite sequence of real database operations.
package demo

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"example.com/course/sql-lab/contactsql"
)

// Run owns its pools and removes only the row it creates. Other table rows are
// preserved. Cleanup has a fresh budget even if the caller's context expires.
func Run(ctx context.Context, dsn string, output io.Writer) (result error) {
	// Open verifies connectivity; EnsureSchema only bootstraps this lesson's
	// table. The store borrows the pool, while this function owns its Close.
	db, err := contactsql.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := contactsql.EnsureSchema(ctx, db); err != nil {
		return err
	}
	store := contactsql.NewStore(db)
	fmt.Fprintln(output, "== 1. Pool, schema, and a parameterized insert ==")
	// A unique teaching name allows repeated runs against the same database.
	input := contactsql.Input{Name: " Ana " + rand.Text()[:8] + " "}
	created, err := store.Create(ctx, input)
	if err != nil {
		return err
	}
	// Register row cleanup immediately after creation. It also runs on a later
	// assertion failure, using a fresh context if the caller's budget expired.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := store.Delete(cleanupCtx, created.ID); err != nil && !errors.Is(err, contactsql.ErrNotFound) {
			result = errors.Join(result, fmt.Errorf("clean up demo contact: %w", err))
		}
	}()
	fmt.Fprintf(output, "Created #%d; omitted email is SQL NULL.\n", created.ID)

	fmt.Fprintln(output, "\n== 2. Query one row, then a bounded collection ==")
	// Get targets our assigned ID. List reads a bounded page from the whole
	// table, so its contents may also include rows from another lab run.
	found, err := store.Get(ctx, created.ID)
	if err != nil {
		return err
	}
	page, err := store.List(ctx, 2)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "Read #%d; ordered page contains %d contact(s).\n", found.ID, len(page))

	fmt.Fprintln(output, "\n== 3. NULL and empty text are different SQL values ==")
	// A pointer to an empty string stores a real empty value; nil stores SQL
	// NULL. Check both results so the difference is observable in Go too.
	empty := ""
	updated, err := store.UpdateEmail(ctx, created.ID, &empty)
	if err != nil {
		return err
	}
	if updated.Email == nil || *updated.Email != "" {
		return errors.New("empty email was not preserved")
	}
	updated, err = store.UpdateEmail(ctx, created.ID, nil)
	if err != nil {
		return err
	}
	if updated.Email != nil {
		return errors.New("NULL email was not preserved")
	}
	fmt.Fprintln(output, "Stored empty text, then SQL NULL; presence stayed distinct.")

	fmt.Fprintln(output, "\n== 4. Expected failures and caller cancellation ==")
	// The name conflict is a database rule. Cancellation below is a caller
	// lifetime decision; errors.Is distinguishes these separate outcomes.
	if _, err := store.Create(ctx, input); !errors.Is(err, contactsql.ErrNameTaken) {
		return fmt.Errorf("expected a duplicate-name error, got %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Get(canceled, created.ID); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("expected cancellation, got %v", err)
	}
	fmt.Fprintln(output, "Duplicate name -> ErrNameTaken; canceled read -> context.Canceled.")

	fmt.Fprintln(output, "\n== 5. A different pool reads the same stored row ==")
	// A second Go pool reaches the same server-owned data. The row's lifetime
	// is independent of either pool; Delete below explicitly removes it.
	secondPool, err := contactsql.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer secondPool.Close()
	if _, err := contactsql.NewStore(secondPool).Get(ctx, created.ID); err != nil {
		return err
	}
	fmt.Fprintln(output, "Data belongs to PostgreSQL, rather than to one Go connection pool.")
	if err := store.Delete(ctx, created.ID); err != nil {
		return err
	}
	if _, err := store.Get(ctx, created.ID); !errors.Is(err, contactsql.ErrNotFound) {
		return fmt.Errorf("expected a missing row after deletion, got %v", err)
	}
	fmt.Fprintln(output, "Deleted the demo contact; repeated lookup -> ErrNotFound.")
	return nil
}
