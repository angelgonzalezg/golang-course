// Package txdemo demonstrates migrations and transactions through finite calls.
package txdemo

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"example.com/course/sql-lab/contactsql"
	"example.com/course/sql-lab/contacttx"
)

// Run owns its pool and attempts to remove only its own committed contacts.
func Run(ctx context.Context, dsn string, output io.Writer) (result error) {
	db, err := contactsql.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	version, err := contacttx.Migrate(dsn)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "== 1. Versioned migrations ==\nSchema version: %d\n", version)
	store := contacttx.NewStore(db)
	prefix := "Tx-" + rand.Text()[:8]
	created, err := store.CreateBatch(ctx, []contactsql.Input{{Name: prefix + "-Ana"}, {Name: prefix + "-Bea"}})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		for _, contact := range created {
			if err := store.Delete(cleanup, contact.ID); err != nil && !errors.Is(err, contactsql.ErrNotFound) {
				result = errors.Join(result, fmt.Errorf("clean up transaction demo: %w", err))
			}
		}
	}()
	fmt.Fprintln(output, "\n== 2. Commit a whole batch ==\nCommitted 2 contacts together.")
	_, err = store.CreateBatch(ctx, []contactsql.Input{{Name: prefix + "-RolledBack"}, {Name: created[1].Name}})
	if !errors.Is(err, contactsql.ErrNameTaken) {
		return fmt.Errorf("expected a batch name conflict, got %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sql_tx_contacts WHERE name = $1", prefix+"-RolledBack").Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return errors.New("failed batch left its first contact committed")
	}
	fmt.Fprintln(output, "\n== 3. Roll back after a later insert fails ==\nRejected batch: prior insert rolled back.")
	email := "ana@example.com"
	updated, err := store.ReplaceIfVersion(ctx, created[0].ID, created[0].Version,
		contactsql.Input{Name: created[0].Name, Email: &email})
	if err != nil {
		return err
	}
	_, err = store.ReplaceIfVersion(ctx, created[0].ID, created[0].Version, contactsql.Input{Name: created[0].Name})
	if !errors.Is(err, contacttx.ErrVersionConflict) {
		return fmt.Errorf("expected a stale-version conflict, got %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sql_tx_contact_changes WHERE contact_id = $1", updated.ID).Scan(&count); err != nil {
		return err
	}
	if updated.Version != 2 || count != 1 {
		return errors.New("contact version and history disagree")
	}
	fmt.Fprintln(output, "\n== 4. Compare versions and commit history together ==\nVersion 1 -> 2 with one change record; stale version rejected.")
	return nil
}
