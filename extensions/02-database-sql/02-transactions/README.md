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

Requirements: Go 1.27, a full repository checkout, and a running Docker-compatible
engine with the Docker CLI and Compose v2. Use bash/zsh from the root containing
`go.mod`, `chapters`, and `extensions`. The first run can download pinned Go
dependencies and container images. Race checks also need clang or gcc.

```sh
go version
docker info
docker compose version
docker compose -f extensions/02-database-sql/compose.yaml up -d --wait
export DATABASE_URL='postgres://sql_lab:sql_lab_local@127.0.0.1:5433/sql_lab?sslmode=disable'
go -C extensions/02-database-sql run ./02-transactions
```

Compose exposes the dedicated lab database on `127.0.0.1:5433` and retains data
in a named volume. The URL above uses its local teaching credentials and disables
database TLS for this loopback setup. The program applies migrations itself.

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
These commands use POSIX environment syntax. In PowerShell, assign the same URL
to `$env:DATABASE_URL` and run the `go -C` command.

After the Go processes stop, stop the local database with:

```sh
docker compose -f extensions/02-database-sql/compose.yaml down
```

This keeps the volume. Use `down -v` only to intentionally discard the shared
SQL/REST/gRPC lab database and its retained data.

## Learn in passes

The executable prepares the schema before running the business examples. On a
first read, treat that as setup and start with `CreateBatch` in
[store.go](../contacttx/store.go). Return to the migration adapter in pass 4.
Recovery and adoption of the previous runner's history are optional operations
at the end of this guide.

| Pass | Focus | Check your understanding |
| --- | --- | --- |
| 1 | Commit a batch. | Find where the transaction starts, which connection executes the inserts, and when success is returned. |
| 2 | Reject the second insert. | Explain why the first insert disappears but its sequence number can be consumed. |
| 3 | Replace a contact and append history. | Trace the version comparison, both writes, and their common commit. |
| 4 | Apply schema versions 1 and 2. | Distinguish the schema's version from a contact's version and recognize dirty migration state. |
| 5 | Read concurrency/cancellation tests. | Explain why only one writer succeeds and how canceling a blocked transaction removes its tentative update. |

## Terms beside the code

| Term | Meaning here |
| --- | --- |
| Transaction / `*sql.Tx` | A unit of SQL work using one connection reserved from the pool. |
| Commit | Confirm all of that transaction's changes together. |
| Rollback | Discard its uncommitted changes. |
| Atomicity | The transaction's database changes take effect together or are discarded together. |
| Isolation level | Rules for which changes a transaction can observe while other transactions run. |
| Read committed / `sql.LevelReadCommitted` | A statement sees data committed when it starts, plus this transaction's own earlier writes. A later statement can see newer commits. |
| Optimistic version check | Update only if the stored row still has the version the caller supplied. |

## 1. Use one transaction for the complete operation

Read `CreateBatch` in [store.go](../contacttx/store.go). It accepts 1–10 inputs,
normalizes them, then begins a `ReadCommitted` transaction. Each insert uses
`tx.QueryRowContext`, followed by `Scan`. The method commits before returning
the created contacts.

Start with a successful batch:

```text
Input: two valid contacts named "Ana" and "Bea"
BeginTx -> INSERT "Ana" -> INSERT "Bea" -> Commit
Result: both stored contacts are returned to the caller
```

`ReadCommitted` keeps other transactions' unfinished writes out of this
transaction's reads. Each statement starts with a view of committed data and
can also see its own transaction's earlier writes. A later statement may see
new commits from another transaction, so this level does not freeze one snapshot
for the whole operation. The version check in the next section protects against
a stale replacement.

Now follow a batch whose second name already exists:

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

Both inserts belong to one transaction. The diagram separates a statement
failure from a Commit error, whose outcome may be uncertain to the caller:

```mermaid
flowchart TD
    B[BeginTx] --> I[Insert batch using tx]
    I --> Q{Inserts succeeded?}
    Q -->|No| R[Rollback tentative rows]
    Q -->|Yes| C[Call Commit]
    C --> A{Commit confirmed?}
    A -->|Yes| S[Return created contacts]
    A -->|Error| U[Return error: outcome uncertain]
```

BeginTx errors return before any inserts. Identity sequence numbers can still
be consumed on the rollback path, even though no new contact rows remain.

`defer tx.Rollback()` supplies cleanup for an early error or panic. Once Commit
succeeds, that deferred call returns `sql.ErrTxDone` and does not undo the commit.
Identity sequences can have gaps after rollback; this is separate from whether
a contact row committed.

## 2. Commit the contact and its history together

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

## 3. Cancellation and idempotency limits

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

Continue to [durable idempotency](../03-idempotency/README.md) for request-key
records and lost-reply recovery. It uses the transaction boundary learned here
to commit a business write and its saved result together. Transport wiring
remains a [later stage](../README.md).

## 4. Evolve the schema without editing applied files

| Term | Meaning here |
| --- | --- |
| Migration | An ordered, versioned schema change stored as a SQL file. |
| Dirty migration | A migration started but was not recorded as successfully completed; inspect the database before continuing. |
| Advisory lock | A PostgreSQL lock that cooperating migration runners acquire using a shared identifier. |

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

The runner reads migration state under the advisory lock and rejects dirty or
invalid targets. For a clean schema and a valid forward target, it then advances
one file at a time while holding that lock:

```mermaid
flowchart TD
    N{Pending file?} -->|No| D[Release lock, report version]
    N -->|Yes| M[Record next version: dirty=true]
    M --> F[Run file: BEGIN, DDL, COMMIT]
    F --> S{SQL succeeded?}
    S -->|Yes| C[Record version: dirty=false]
    C --> N
    S -->|SQL failure| X[Roll back file SQL, retain dirty state]
    X --> U[Release lock, return error for review]
```

The failure branch illustrates a SQL failure inside the file. A lost connection
or process interruption still requires inspecting actual schema state; this
diagram does not treat every error as proof that DDL rolled back.

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

For optional implementation reading, trace the driver's lock, `Version`,
`SetVersion`, and `Run` methods. The adapter's `forwardOnlyDriver` adds the
lesson's forward-only policy; the library executes and records migrations.

## Validate with PostgreSQL and Testcontainers

Ordinary tests and checks need no running database:

```sh
go -C extensions/02-database-sql test -count=1 -timeout=60s ./...
go -C extensions/02-database-sql vet ./...
go -C extensions/02-database-sql mod tidy -diff
```

Integration tests start their own PostgreSQL containers, wait for readiness, and
register cleanup. They do not use Compose or `DATABASE_URL`; only the container
engine must be running. The race detector additionally requires a C compiler.

For Rancher Desktop on macOS, select **dockerd (moby)**. When its socket is under
your home directory and forwarded ports are reachable on localhost, set these
in the same terminal before running the tests:

```sh
export DOCKER_HOST="unix://$HOME/.rd/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1
```

Docker Desktop/local Docker usually works with automatic detection. A Rancher
VM networking setup may instead need its reachable VM address as the host
override; `rdctl shell ip a show vznat` shows the VZ interface. Never set the
override to an empty value. Verify the engine and run:

```sh
docker info
CGO_ENABLED=1 go -C extensions/02-database-sql test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/02-database-sql vet -tags=integration ./...
```

[transactions_test.go](../integration/transactions_test.go) checks upgrades
without losing data, repeated migrations, per-file rollback and dirty-state
rejection, unknown future versions, concurrent migrators, batch commit/rollback, failure
of the history insert, competing writers, cancellation after an UPDATE, and the
finite demo's cleanup. The top-level SQL suites own separate containers
through [the shared fixture](../integration/database_test.go).
[migrations_test.go](../integration/migrations_test.go) also checks explicit
legacy adoption with retained rows, rejected legacy history, and down/up cycles
in the isolated test database.

Ordinary tests still compile the lessons and check input/error rules without
Docker. CI's existing integration-tagged job automatically includes the new
suite. The migration dependency is confined to this module; workflow commands
remain the same.

## Optional operations: recovery and previous-runner adoption

These sections support existing or failed lab databases. They are not required
for a first run against a fresh database or for understanding business transactions.

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

References: [golang-migrate migration format](https://github.com/golang-migrate/migrate/blob/v4.20.1/MIGRATIONS.md),
[pgx migration driver](https://pkg.go.dev/github.com/golang-migrate/migrate/v4/database/pgx/v5),
[Go transaction guide](https://go.dev/doc/database/execute-transactions),
[PostgreSQL isolation](https://www.postgresql.org/docs/18/transaction-iso.html),
and [PostgreSQL locking](https://www.postgresql.org/docs/18/explicit-locking.html).
