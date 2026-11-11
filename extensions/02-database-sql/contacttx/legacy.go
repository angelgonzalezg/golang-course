package contacttx

import (
	"fmt"

	"github.com/golang-migrate/migrate/v4/database"
)

// These digests identify the two files from the superseded teaching runner.
// They are only an adoption check, not checksum tracking by golang-migrate.
var legacyChecksums = [...]string{
	"81e5357b8f264f20b5186a9b5a0136b2e16764d6cdc6d2181daa8aad120ea85a",
	"495386dafffb52745fad6d44ffa0631c09b38f0ba22c01ed07cf0be1076aa563",
}

func (d *forwardOnlyDriver) legacyVersion() (int, bool, error) {
	var exists bool
	if err := d.db.QueryRow("SELECT to_regclass('public.sql_tx_migrations') IS NOT NULL").Scan(&exists); err != nil {
		return 0, false, err
	}
	if !exists {
		if d.adoptLegacy {
			return 0, false, fmt.Errorf("no legacy history to adopt: %w", ErrMigrationState)
		}
		return database.NilVersion, false, nil
	}
	if !d.adoptLegacy {
		return 0, false, fmt.Errorf("legacy history found; review it and run -adopt-legacy once: %w", ErrMigrationState)
	}
	rows, err := d.db.Query("SELECT version, checksum FROM public.sql_tx_migrations ORDER BY version")
	if err != nil {
		return 0, false, err
	}
	defer rows.Close()
	version := 0
	for rows.Next() {
		var next int
		var checksum string
		if err := rows.Scan(&next, &checksum); err != nil {
			return 0, false, err
		}
		if next != version+1 || next > len(legacyChecksums) || checksum != legacyChecksums[next-1] {
			return 0, false, fmt.Errorf("unknown or changed legacy history at %d: %w", next, ErrMigrationState)
		}
		version = next
	}
	if err := rows.Err(); err != nil {
		return 0, false, err
	}
	if version == 0 || version > d.target {
		return 0, false, fmt.Errorf("unsupported legacy version %d: %w", version, ErrMigrationState)
	}
	// The old history has been checked, and the library's advisory lock is held.
	// Record a baseline without re-running CREATE TABLE or deleting learner data.
	if err := d.Driver.SetVersion(version, false); err != nil {
		return 0, false, err
	}
	return version, false, nil
}
