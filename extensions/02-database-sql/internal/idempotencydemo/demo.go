// Package idempotencydemo follows one durable create through repeats, a new
// database pool, and later business-row deletion. Transport wiring follows later.
package idempotencydemo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"time"

	"example.com/course/sql-lab/contactidem"
	"example.com/course/sql-lab/contactsql"
)

// Run owns its pools and cleans up only its random demo subject's rows. Cleanup
// is a finite-example convenience; normal applications retain request records.
func Run(ctx context.Context, dsn string, output io.Writer) (result error) {
	version, err := contactidem.Migrate(dsn)
	if err != nil {
		return err
	}
	db, err := contactsql.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	store := contactidem.NewStore(db)
	subject := "demo-" + rand.Text()[:12]
	input := contactsql.Input{Name: " Ana " + subject + " "}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		result = errors.Join(result, cleanupDemo(cleanup, db, subject))
	}()

	fmt.Fprintf(output, "== 1. Commit a contact and its saved result ==\nIdempotency schema version: %d\n", version)
	first, err := store.CreateOnce(ctx, subject, "create-1", input)
	if err != nil || first.Replayed {
		return fmt.Errorf("expected a fresh creation: %v", err)
	}
	again, err := store.CreateOnce(ctx, subject, "create-1", input)
	if err != nil || !again.Replayed || again.Contact.ID != first.Contact.ID {
		return fmt.Errorf("repeat did not replay the saved creation: %v", err)
	}
	fmt.Fprintln(output, "Same key/data -> original result; one contact created.")

	fmt.Fprintln(output, "\n== 2. Reject a changed request under the same key ==")
	_, err = store.CreateOnce(ctx, subject, "create-1", contactsql.Input{Name: "Different " + subject})
	if !errors.Is(err, contactidem.ErrKeyReuse) {
		return fmt.Errorf("expected a key/payload conflict, got %v", err)
	}
	fmt.Fprintln(output, "Changed payload -> ErrKeyReuse; no new contact.")

	// A new Go pool/store has no shared cache to consult. It reads the committed
	// request record from PostgreSQL. Integration tests also use separate processes.
	fmt.Fprintln(output, "\n== 3. Recover the result through a new pool ==")
	secondPool, err := contactsql.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer secondPool.Close()
	recovered, err := contactidem.NewStore(secondPool).CreateOnce(ctx, subject, "create-1", input)
	if err != nil || !recovered.Replayed || recovered.Contact.ID != first.Contact.ID {
		return fmt.Errorf("new pool did not recover the creation: %v", err)
	}
	fmt.Fprintln(output, "New pool -> original result replayed from PostgreSQL.")

	// Direct SQL is an observation tool in this standalone lab. Replay must
	// return the stored snapshot, not read current fields or recreate a deleted row.
	fmt.Fprintln(output, "\n== 4. Keep the creation result after edits and deletion ==")
	if _, err := db.ExecContext(ctx, "UPDATE sql_idem_contacts SET name = $2 WHERE id = $1", first.Contact.ID, "Edited "+subject); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM sql_idem_contacts WHERE id = $1", first.Contact.ID); err != nil {
		return err
	}
	historical, err := store.CreateOnce(ctx, subject, "create-1", input)
	if err != nil || !historical.Replayed || historical.Contact.ID != first.Contact.ID || historical.Contact.Name != first.Contact.Name {
		return fmt.Errorf("historical result was not retained: %v", err)
	}
	var exists bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM sql_idem_contacts WHERE id = $1)", first.Contact.ID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return errors.New("replay recreated a deleted contact")
	}
	fmt.Fprintln(output, "Deleted contact -> original result replayed; contact stays absent.")
	return nil
}

// cleanupDemo is restricted to this run's randomly named subject. Deleting
// request records forgets their keys, so this is not normal service behavior.
func cleanupDemo(ctx context.Context, db *sql.DB, subject string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sql_idem_contacts WHERE id IN
		(SELECT response_id FROM sql_idem_create_requests WHERE subject = $1)`, subject); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sql_idem_create_requests WHERE subject = $1", subject); err != nil {
		return err
	}
	return tx.Commit()
}
