# Transactions and versioned migrations

Read the [SQL fundamentals](../README.md) first. This stage groups SQL statements
into one unit of work, evolves the schema, and rejects a stale concurrent edit.
It uses `database/sql` and the same pgx driver as stage 1, with
[`golang-migrate` v4.20.1](https://github.com/golang-migrate/migrate/tree/v4.20.1)
for schema migrations. The version is pinned in this module's `go.mod`; running
the lesson does not require installing a separate migration executable.

The `sql_tx_*` tables belong to this lesson. Stage 1 keeps its original
`sql_lab_contacts` table and its direct queries. Both lessons reuse the same
input normalization rules through `contactsql.NormalizeInput`.

## Run from the repository root

Start the [local teaching database](../README.md#start-a-local-teaching-database),
then run:

```sh
DATABASE_URL='postgres://sql_lab:sql_lab_local@127.0.0.1:5433/sql_lab?sslmode=disable' \
  go -C extensions/02-database-sql run ./02-transactions
```

The finite demonstration applies pending migrations, creates two contacts,
checks a rejected batch, and performs a versioned replacement. It attempts to
delete its own committed contacts and change records before exiting. Existing
contacts from other runs are preserved. Look for:

```text
Schema version: 2
Committed 2 contacts together.
Rejected batch: prior insert rolled back.
Version 1 -> 2 with one change record; stale version rejected.
```

To apply only the migrations:

```sh
DATABASE_URL='postgres://sql_lab:sql_lab_local@127.0.0.1:5433/sql_lab?sslmode=disable' \
  go -C extensions/02-database-sql run ./02-transactions -migrate-only
```

Repeating this command checks migration state and reports version 2 without applying
the files again. The shared [Compose file](../compose.yaml) needs no changes.
See stage 1 for PowerShell environment syntax and stopping the database.

## Learn in passes

| Pass | Focus | Check your understanding |
| --- | --- | --- |
| 1 | Apply schema versions 1 and 2. | Distinguish an applied migration record from a table's current contents. |
| 2 | Commit a batch. | Find where the transaction starts, which connection executes the inserts, and when success is returned. |
| 3 | Reject the second insert. | Explain why the first insert disappears but its sequence number can be consumed. |
| 4 | Replace a contact and append history. | Trace the version comparison, both writes, and their common commit. |
| 5 | Read concurrency/cancellation tests. | Explain why only one writer succeeds and how canceling a blocked transaction removes its tentative update. |

## Terms beside the code

| Term | Meaning here |
| --- | --- |
| Transaction / `*sql.Tx` | A unit of SQL work using one connection reserved from the pool. |
| Commit | Confirm all of that transaction's changes together. |
| Rollback | Discard its uncommitted changes. |
| Atomicity | The transaction's database changes take effect together or are discarded together. |
| Migration | An ordered, versioned schema change stored as a SQL file. |
| Dirty migration | A migration started but was not recorded as successfully completed; inspect the database before continuing. |
| Advisory lock | A PostgreSQL lock that cooperating migration runners acquire using a shared identifier. |
| Optimistic version check | Update only if the stored row still has the version the caller supplied. |

## 1. Evolve the schema without editing applied files

Read [migrations.go](../contacttx/migrations.go) alongside these embedded files:

1. [000001_contacts.up.sql](../contacttx/migrations/000001_contacts.up.sql) creates the
   lesson's contact table with name uniqueness and nullable email.
2. [000002_versions_and_changes.up.sql](../contacttx/migrations/000002_versions_and_changes.up.sql)
   adds a version counter, initializes existing rows to version 1, and creates
   a change-history table with a foreign key to the contact.

The standard filename format is `<version>_<description>.up.sql`, paired with
a `.down.sql` file for the inverse schema change. The files are embedded with
`go:embed`; the library's `iofs` source reads them from the executable.
`Migrate(dsn)` applies the current file set. `MigrateTo(dsn, 1)` lets the upgrade
test save data at version 1 before applying version 2.

The pgx migration driver takes a PostgreSQL advisory lock and maintains one
current `(version, dirty)` row in `sql_tx_schema_migrations`. This is not a row
per applied file. The library owns ordering, locking, execution, and recording
state. Our small adapter checks the target **while that lock is held**, so an
older program cannot accidentally migrate a newer schema backward. Unknown
future versions and backward targets return `ErrMigrationState`; a dirty schema
returns the library's `migrate.ErrDirty`.

Each SQL file explicitly uses `BEGIN` / `COMMIT`. Its own DDL is atomic, but
**the whole set of pending files is not one transaction**. If version 2 fails,
version 1 remains applied, version 2's DDL rolls back, and the version table
records `(2, true)`. Repeating the command stops for review. A process failure
can also leave dirty state even when SQL committed; the flag alone does not
prove which statements took effect.

The migration pool is separately owned and closed because the library closes
its driver's pool. The business store's pool remains available. Unlike the
context-aware business API, the library's migration API takes no context.
This adapter sets a 5-second connection timeout, PostgreSQL's 5-second lock and
10-second statement timeouts, and the driver's 10-second execution timeout.
These are per-operation limits, not a deadline for an entire deployment.

Keep applied files immutable and add a numbered pair for each new change.
`golang-migrate` tracks version and dirty state, **not file checksums or live
schema drift**. Review immutable migration artifacts in version control.
The examples use PostgreSQL's `public` schema. Statements such as
`CREATE INDEX CONCURRENTLY` cannot run inside these transaction-wrapped files
and need their own migration policy and failure handling.

### Down files and failure recovery

[Version 2 down](../contacttx/migrations/000002_versions_and_changes.down.sql)
drops change history and the row-version column; version 1 down drops contacts.
Their effects can destroy data. The ordinary lesson command only moves forward.
The integration suite exercises down/up using the library directly in its own
disposable database. A down migration is not a backup or an automatic response
to a failed deployment.

For dirty state, stop competing migration commands, inspect the error and actual
schema, and decide whether to complete the intended change or return to the
last consistent version. Only after verifying that state should an operator
use the standard tool's `force <verified-version>` to align its version record,
configured for the same database, `public` schema, and
`sql_tx_schema_migrations` table (not the tool's default table).
`force` changes metadata; it does not execute or undo SQL. This lesson never
automatically clears dirty state. For production, run migrations as a controlled
deployment step with suitable DDL permissions before starting business traffic;
the finite demo runs them first for convenience.

### If you ran the previous teaching runner

An existing `sql_tx_migrations` table belongs to the superseded manual runner.
The normal command stops rather than replaying CREATE TABLE over that schema.
After stopping old migration commands, reviewing the schema, and backing up
data you want to retain, use this one-time adoption command:

```sh
DATABASE_URL='postgres://sql_lab:sql_lab_local@127.0.0.1:5433/sql_lab?sslmode=disable' \
  go -C extensions/02-database-sql run ./02-transactions -adopt-legacy
```

[legacy.go](../contacttx/legacy.go) accepts only the old runner's known,
contiguous version 1 or 2 history and exact file digests. Under the new migration
lock it records that baseline, then applies any pending up migration. It keeps
existing contacts and the old ledger, and refuses changed, missing, or unknown
history. This check does not prove the live schema was never manually edited.
It cannot clear an existing dirty record. Fresh databases need no adoption.

The main learning path now uses the established tool. To study migration
internals optionally, trace the driver's lock, `Version`, `SetVersion`, and
`Run` methods; maintaining a second migration engine is outside this lab.

## 2. Use one transaction for the complete operation

Read `CreateBatch` in [store.go](../contacttx/store.go). It accepts 1–10 inputs,
normalizes them, then begins a `ReadCommitted` transaction. Each insert uses
`tx.QueryRowContext`, followed by `Scan`. The method commits before returning
the created contacts.

Follow a batch whose second name already exists:

```text
Before: one existing contact named "Bea"
BeginTx
INSERT "Ana" -> succeeds inside the transaction
INSERT "Bea" -> unique-name constraint fails
Deferred Rollback -> discard this transaction's "Ana"
After: only the original "Bea" remains
```

Use the `tx` methods throughout the unit of work. A call to `db.ExecContext`
would use the pool outside that transaction, potentially on another connection.

`defer tx.Rollback()` supplies cleanup for an early error or panic. Once Commit
succeeds, that deferred call returns `sql.ErrTxDone` and does not undo the commit.
Identity sequences can have gaps after rollback; this is separate from whether
a contact row committed.

## 3. Commit the contact and its history together

`ReplaceIfVersion` performs a full replacement of this lesson's editable fields.
The conditional statement is conceptually:

```sql
UPDATE sql_tx_contacts
SET name = $3, email = $4, version = version + 1
WHERE id = $1 AND version = $2
RETURNING id, name, email, version;
```

After reading the updated row, the same transaction inserts a change record.
Only a successful common commit reports success. If history insertion fails,
the new fields and version are rolled back as well.

Two writers can both read version 1, but only one can replace that version.
PostgreSQL's row locking and predicate recheck make the other update match no
row after the winner commits. This method reports `ErrVersionConflict` for
either a missing ID or a stale version; the lesson deliberately does not infer
404 versus a version-precondition status from that zero-row result.

All store transactions explicitly use `ReadCommitted`. The version predicate
protects this row's participating writes; it is not a claim of serializable
behavior for arbitrary multi-row business rules. Direct SQL that bypasses the
version protocol can bypass its application guarantee.

## 4. Cancellation and idempotency limits

The cancellation test locks the history table on another connection. It observes
the writer waiting on the real history INSERT through `pg_stat_activity`, then
cancels its context. This proves the preceding UPDATE has been reached. The
test joins the writer and verifies that neither the tentative fields/version
nor a history record remain. Polling the server state is bounded; a fixed sleep
does not choose when to cancel.

These are database-transaction guarantees. Rollback does not reverse external
network calls or other effects outside the transaction. A Commit error can also
leave the caller uncertain whether PostgreSQL committed before communication
failed; it should not automatically retry creation as a new request.

Version checks prevent a stale edit. They do not save a response under an
idempotency key. Repeating a request with the original version returns a
conflict after its first successful replacement. A new accepted replacement
increments the counter even if the supplied fields are equal. This counter is
different from the REST project's content-derived ETag.

Durable request-key records, lost-reply recovery, and transport wiring remain
the [next stages](../README.md). The transaction boundary here provides the
foundation for committing a business write and its key record together later.

## Validate with PostgreSQL and Testcontainers

The existing [integration command and Rancher setup](../README.md#testcontainers-integration-tests)
run both SQL stages:

```sh
go -C extensions/02-database-sql test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/02-database-sql vet -tags=integration ./...
```

[transactions_test.go](../integration/transactions_test.go) checks upgrades
without losing data, repeated migrations, per-file rollback and dirty-state
rejection, unknown future versions, concurrent migrators, batch commit/rollback, failure
of the history insert, competing writers, cancellation after an UPDATE, and the
finite demo's cleanup. The two top-level SQL suites own separate containers
through [the shared fixture](../integration/database_test.go).
[migrations_test.go](../integration/migrations_test.go) also checks explicit
legacy adoption with retained rows, rejected legacy history, and down/up cycles
in the isolated test database.

Ordinary tests still compile the lessons and check input/error rules without
Docker. CI's existing integration-tagged job automatically includes the new
suite. The migration dependency is confined to this module; workflow commands
remain the same.

References: [golang-migrate migration format](https://github.com/golang-migrate/migrate/blob/v4.20.1/MIGRATIONS.md),
[pgx migration driver](https://pkg.go.dev/github.com/golang-migrate/migrate/v4/database/pgx/v5),
[Go transaction guide](https://go.dev/doc/database/execute-transactions),
[PostgreSQL isolation](https://www.postgresql.org/docs/18/transaction-iso.html),
and [PostgreSQL locking](https://www.postgresql.org/docs/18/explicit-locking.html).
