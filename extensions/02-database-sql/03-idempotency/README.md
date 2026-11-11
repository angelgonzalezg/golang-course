# Durable idempotency with PostgreSQL

Create a contact once, then recover its original result when a caller retries.
The contact and its saved creation result commit in the same transaction.
PostgreSQL retains both after the Go process exits.

Read [SQL fundamentals](../README.md) and
[transactions and migrations](../02-transactions/README.md) first. Start with the
finite demonstration, then read the transaction, then its concurrency tests.
The REST and gRPC projects still use their existing in-memory stores; this
standalone lesson prepares the database policy before transport integration.

## Run the demonstration

From the repository root, start the lab's local PostgreSQL service:

```sh
docker compose -f extensions/02-database-sql/compose.yaml up -d --wait
DATABASE_URL='postgres://sql_lab:sql_lab_local@127.0.0.1:5433/sql_lab?sslmode=disable' \
  go -C extensions/02-database-sql run ./03-idempotency
```

Use a dedicated lab database. The command applies this lesson's migrations and
prints four sections. Look for:

```text
Idempotency schema version: 1
Same key/data -> original result; one contact created.
Changed payload -> ErrKeyReuse; no new contact.
New pool -> original result replayed from PostgreSQL.
Deleted contact -> original result replayed; contact stays absent.
```

Each run uses a random demo subject and attempts to remove only its own contact
and saved result before exiting. Deleting request records forgets their keys;
this cleanup makes the finite example repeatable and is not the service policy.
Other callers' rows are preserved. The integration tests retain records between
two child processes to check actual process independence.

To prepare only this lesson's schema, run:

```sh
DATABASE_URL='postgres://sql_lab:sql_lab_local@127.0.0.1:5433/sql_lab?sslmode=disable' \
  go -C extensions/02-database-sql run ./03-idempotency -migrate-only
```

Repeating that command reports version 1 without applying the file again.
When finished, `docker compose -f extensions/02-database-sql/compose.yaml down`
stops the service and retains its volume. See the
[SQL setup guide](../README.md#start-a-local-teaching-database) for configuration
and intentional volume removal.

## Learn in passes

| Pass | Follow | Check your understanding |
| --- | --- | --- |
| 1 | A create and its replay. | Explain why a unique contact name alone cannot recover the original response. |
| 2 | Caller, key, and request fields. | Predict what happens when the key is reused with another name, whitespace, or email presence. |
| 3 | The two inserts inside one transaction. | Explain what remains if saving the result fails after inserting the contact. |
| 4 | Two callers using the same key. | Identify where they wait and why the second caller sees the first caller's commit. |
| 5 | A lost reply and a new process. | Explain which data lives in PostgreSQL and why retrying the original request is safe. |

Migration internals and failure-recovery details can be a second reading after
the business flow is clear.

## Terms beside the example

| Term | Meaning here |
| --- | --- |
| Subject | The caller identity that owns the key namespace, such as `user-1`. A future service must obtain it from trusted authentication. |
| Idempotency key | A caller-chosen identifier reused for retries of one intended operation, such as `create-1`. |
| Fingerprint | A SHA-256 digest of the original typed request fields, used to detect changed data under an existing key. It is not authentication. |
| Saved result / snapshot | The contact ID, name, and optional email returned by the original successful creation. |
| Replay | Returning that saved snapshot without inserting another contact. `Result.Replayed` identifies this path. |
| Advisory lock | A PostgreSQL coordination mechanism. Writers following this protocol wait for the same caller/key before inspecting or creating its result. |
| Durable | Committed records remain in PostgreSQL across connection-pool and application-process lifetimes. |

### Bind the key to a caller and the original request

The public operation is:

```go
result, err := store.CreateOnce(ctx, "user-1", "create-1", contactsql.Input{
    Name: " Ana ",
})
```

`NewStore` borrows a prepared `*sql.DB`; the caller owns its lifetime. Pass a
context with a suitable deadline and keep the input, including optional pointer
values, stable until the call returns.

Subject and key each accept 1–64 ASCII letters, digits, `_`, or `-`. The
standalone API validates their shape but does not authenticate their contents.
Names and email reuse the earlier lesson's validation and SQL NULL rules.

For the call above, PostgreSQL stores the business name as `Ana`, but the
fingerprint uses the original `" Ana "`. Once it succeeds:

| Next call | Outcome |
| --- | --- |
| Same subject, key, and original fields | Original contact and `Replayed == true`. |
| Same subject/key, name changed to `"Ana"` | `ErrKeyReuse`, even though trimming produces the same business name. |
| Same subject/key, email changed from nil to a pointer to `""` | `ErrKeyReuse`: absent email and empty text differ. |
| Another subject with `create-1` | A separate operation; ordinary business rules still apply, including unique contact names. |
| Same subject with a new key | A new operation, not a retry. |

The fixed typed encoding includes a format version before hashing. Keep this
identity format compatible while records remain replayable; changing how fields
are encoded or normalized can change a request's identity. It does not define a
canonical representation for arbitrary JSON documents.

### Follow one transaction

Read the four numbered steps in [store.go](../contactidem/store.go):

```text
BeginTx(ReadCommitted)
  1. Acquire the transaction advisory lock for this subject/key.
  2. SELECT its saved result, after acquiring the lock.
     Existing record + matching fingerprint -> use the saved snapshot.
     Existing record + different fingerprint -> ErrKeyReuse, then rollback.
  3. No record -> INSERT contact, then INSERT fingerprint + result snapshot.
  4. Commit, release the lock, and return the result.
Any earlier error -> rollback both writes and release the lock.
```

Use the same `tx` for both inserts. Calling `db.ExecContext` for the second write
would run outside this transaction and break their common commit. There is no
committed "pending" reservation: another successful reader sees a complete
result. The deferred `Rollback` covers early returns; it cannot undo a completed
commit.

The result table has a primary key on `(subject, request_key)`. It saves the
complete creation snapshot rather than joining against the current contact.
Its contact ID intentionally has no foreign key: later edits or deletion must
not erase the original response. A replay can therefore return the original
contact even when a current lookup would find changed data or no contact at all.
Replay never recreates that deleted contact.

### Coordinate concurrent requests

The advisory lock belongs to the transaction and is released on commit or
rollback. Every participating writer computes its lock identifier using the
same operation prefix, subject, and key. A rare collision in that 64-bit lock
identifier only makes unrelated operations wait; the exact database primary
key still identifies their separate records.

The saved-result `SELECT` runs as a **separate statement after the lock is
acquired**. PostgreSQL's Read Committed isolation gives each statement a fresh
view of committed rows. If a duplicate waited for the original writer, it can
now see that writer's committed result. Moving this read before the wait, or
combining it with a read using an earlier snapshot, would undermine the flow.

The coordination works across pools and processes, provided all writers follow
the protocol. SQL constraints also protect stored data. The lock is cooperative;
it cannot make unrelated direct SQL code obey this creation policy.

`ExecContext` makes waiting for the key cancellable. Canceling a waiting duplicate
ends that duplicate's transaction without canceling the original writer. If the
writer itself is canceled before commit, both its contact and result roll back.

### Decide what to retain and how to retry

This operation saves **successful committed creations only**. A known failure
before commit, including a unique-name error or a failed result insert, leaves
the key available. After such a rollback, a corrected request may use it.
The existing [gRPC in-memory policy](../../01-protobuf-and-grpc/05-resilience/README.md)
also retains terminal errors. These are different contracts: the database
operation here owns both writes and can roll back the complete attempted work.

A lost response does not mean the transaction failed. A `Commit` error can also
leave the caller uncertain whether PostgreSQL committed. Retry with the **same
subject, key, and original fields** to recover a saved success or perform the
previously uncommitted operation. Do not switch keys or change the payload to
resolve an ambiguous outcome. The store does not run a hidden retry loop.

Records have no automatic expiry. Their presence supplies the replay guarantee;
deleting them allows those keys to describe new operations. A later service must
choose an explicit retention policy and protect the stored response data. A
database transaction covers these SQL writes, not external effects such as an
email or a payment request. This example does not promise exactly-once network
delivery.

## Read the code in order

1. [Migration SQL](../contactidem/migrations/000001_contacts_and_results.up.sql):
   business rows, request identity, and historical results in separate tables.
2. [request.go](../contactidem/request.go): validation, fingerprint encoding, and
   the stable lock identifier.
3. [store.go](../contactidem/store.go): lock, lookup, two writes, and commit.
4. [demo.go](../internal/idempotencydemo/demo.go) and [main.go](main.go): pool
   ownership, a finite operation budget, and cleanup of the random demo subject.
5. [Unit tests](../contactidem/request_test.go), then the
   [PostgreSQL suite](../integration/idempotency_test.go): validation first,
   committed behavior and row counts next.
6. [Cancellation tests](../integration/idempotency_context_test.go) and
   [process tests](../integration/idempotency_process_test.go): controlled waiting
   and recovery without shared application memory.
7. [migrations.go](../contactidem/migrations.go): the same `golang-migrate`
   approach as stage 2, with this lesson's independent migration state.

Code and test comments explain the purpose of each phase and scenario. The
additional test helpers keep container setup, state observation, and assertions
out of the business operation.

### Independent schema and migration state

This lesson owns `sql_idem_contacts`, `sql_idem_create_requests`, and
`sql_idem_schema_migrations` in `public`. Stage 2 keeps its separate tables and
version 2. The new schema starts at version 1; earlier lesson commands remain
runnable, and applied stage-2 migration files are unchanged.

The migration adapter owns a separate pool because its driver closes that pool.
It uses a 5-second connection timeout, a 5-second server lock timeout, a 10-second
statement limit, and a 15-second migration-lock wait. These are per-operation
bounds, not one overall context deadline: the library accepts no context.
Normal commands apply only forward migrations. Dirty or unknown future migration
state causes an error and is not overwritten. Review recovery using the
[stage-2 migration guide](../02-transactions/README.md#4-evolve-the-schema-without-editing-applied-files);
do not force an unreviewed state to clean. The supplied down file drops this
lesson's data, including all retained request keys.

## Verify the behavior

Ordinary tests and checks need no running database:

```sh
go -C extensions/02-database-sql test -count=1 -timeout=60s ./...
go -C extensions/02-database-sql vet ./...
go -C extensions/02-database-sql mod tidy -diff
```

For real PostgreSQL tests, start the container engine. The race command also
requires a C compiler:

```sh
docker info
go -C extensions/02-database-sql test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/02-database-sql vet -tags=integration ./...
```

See [Testcontainers setup](../README.md#testcontainers-integration-tests) for
Rancher Desktop settings. These tests create their own disposable databases and
never read your `DATABASE_URL`. The integration suite verifies:

- A successful create and retries retain one business row and one result.
- Changed fields conflict; different callers have separate key namespaces.
- Business errors, forced result-insert failure, and cancellation before commit
  leave no partial creation or reserved key.
- Twelve concurrent calls through two independent pools produce one fresh
  creation and eleven replays. Competing payloads produce one creation and one
  key conflict.
- Canceling a waiting duplicate leaves the original writer able to commit.
- Replays retain the original fields after the contact is edited or deleted.
- One child process commits and exits with its reply discarded; a second process
  recovers the same ID. This models a lost response after commit, without
  claiming to inject real network packet loss.
- Migrations remain independent, repeat safely, and reject dirty/future state.
- Finite-demo cleanup preserves other callers' rows and replayable results.

Cancellation tests wait until `pg_stat_activity` reports the intended blocked
statement. Their watchdogs bound failure time; fixed sleeps do not decide when
the test assumes a write has happened. `TestIdempotencyProcessHelper` is the
test-binary entry point used by the child processes; in an ordinary parent run
it returns without opening another fixture.

Continue to the [sqlc comparison](../04-sqlc/README.md) for generated queries
beside the manual implementation. Persistent REST/gRPC adapters follow it.

References: [Go transactions](https://go.dev/doc/database/execute-transactions),
[PostgreSQL transaction advisory locks](https://www.postgresql.org/docs/18/explicit-locking.html#ADVISORY-LOCKS),
and [Read Committed isolation](https://www.postgresql.org/docs/18/transaction-iso.html#XACT-READ-COMMITTED).
