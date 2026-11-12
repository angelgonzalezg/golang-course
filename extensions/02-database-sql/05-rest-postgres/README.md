# REST with PostgreSQL

Run the existing contacts HTTP API with persistent storage and optional durable
keyed creation. This lesson connects the shared contact rules and HTTP adapter
to generated PostgreSQL queries.

This is the second REST project, following
[REST fundamentals](../../../chapters/13-standard-library/06-project-rest-api/README.md)
and the SQL lessons. PostgreSQL retains committed contacts and creation results
across server restarts.

Read [transactions/migrations](../02-transactions/README.md),
[durable idempotency](../03-idempotency/README.md), and
[sqlc](../04-sqlc/README.md) first.

## Run the finite walkthrough

From the repository root, start the dedicated lab database and configure its URL:

```sh
docker compose -f extensions/02-database-sql/compose.yaml up -d --wait
export DATABASE_URL='postgres://sql_lab:sql_lab_local@127.0.0.1:5433/sql_lab?sslmode=disable'
go -C extensions/02-database-sql run ./05-rest-postgres
```

The demo uses a real loopback HTTP server, applies service migrations, and prints:

```text
Repeated POST -> same 201, Location, ETag, and contact; replay=true.
Changed fields under the same key -> 409; no extra contact.
PATCH clears one field; PUT clears omitted fields; stale ETag -> 412.
Original creation replays from PostgreSQL; deleted contact stays absent.
```

Each run uses a random demo caller and attempts to remove only its own contact
and saved creation record. Other callers' data remains. This cleanup makes the
finite example repeatable; the interactive server retains committed records.
The demo stops its server, joins the Serve goroutine, and closes its pool.

Apply only migrations with:

```sh
go -C extensions/02-database-sql run ./05-rest-postgres -migrate-only
```

It reports `Service schema version: 1`. Repeating it checks state without
reapplying the file. Stages 1–4 keep their existing tables and commands.

## Try persistent requests

Keep the server running in one terminal:

```sh
go -C extensions/02-database-sql run ./05-rest-postgres -serve
```

Its default address is `127.0.0.1:8081`. Use `-addr` to choose another address.
The database URL comes from the environment and is not printed. The loopback
HTTP setup uses the local lab database configuration described in the
[SQL setup guide](../README.md#start-a-local-teaching-database).

In another terminal, send this request twice:

```sh
curl -i http://127.0.0.1:8081/contacts \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: create-ana-1' \
  -d '{"name":"Ana","email":"ana@example.com","phone":"555-0100"}'
```

The first successful call returns `201`, a contact, `Location`, its original
ETag, and `Idempotency-Replayed: false`. The repeat returns the same creation
representation with `Idempotency-Replayed: true`. IDs are database-assigned and
can have gaps; use the returned Location rather than assuming `/contacts/1`.

Stop the Go server with Ctrl+C, start it again, and repeat that exact request.
The result is still replayed because PostgreSQL retains it. Changing a field
under that key returns `409`. A POST without the header remains ordinary
creation; repeating a taken name returns a name conflict rather than replay.

Use the returned resource URL with the chapter's
[PUT/PATCH examples](../../../chapters/13-standard-library/06-project-rest-api/README.md#put-versus-patch)
and [conditional request guide](../../../chapters/13-standard-library/06-project-rest-api/conditional-requests.md).
Read the current resource before choosing its If-Match validator.

Stopping Compose with `docker compose -f extensions/02-database-sql/compose.yaml down`
retains its volume. Intentional volume removal also removes its data and replay
records; the setup guide explains that separate operation.

## Learn in passes

| Pass | Follow | Check your understanding |
| --- | --- | --- |
| 1 | Unkeyed create/read through the same HTTP handler. | Identify the interface that lets storage change without rewriting routes. |
| 2 | PUT/PATCH/DELETE transactions. | Explain why reading the current contact and checking its ETag must share the write's row lock. |
| 3 | Keyed POST and its replay. | Trace the caller, key, original fields, contact insert, saved snapshot, and commit. |
| 4 | Concurrent requests and cancellation. | Distinguish canceling the original writer from canceling a waiting duplicate. |
| 5 | Process restart and retained state. | Explain why the new Go process can recover a response it never held in memory. |

## Keep the REST contract

The [shared HTTP adapter](../../../chapters/13-standard-library/06-project-rest-api/httpapi/handler.go)
still handles JSON limits, problem responses, route methods, resource links,
HEAD, OPTIONS, ETags, and HTTP preconditions. The PostgreSQL adapter implements
the same `ContactStore` interface with shared domain rules:

- Names/email/phone are trimmed; names remain required and case-sensitive unique.
- PUT replaces all editable fields, clearing omitted optional values. Its
  transformed success response has no ETag; GET supplies the current validator.
- PATCH preserves omitted fields, clears optional fields on null, and rejects
  invalid changes without modifying any field.
- Version conditions are checked atomically with the corresponding mutation.
- Filtering, sort order, ID tie-breaking, pagination links, and collection ETags
  retain their existing behavior. Empty collections stay `[]`.
- Repeated assignments have the same intended state effect; repeated DELETE
  returns 404 after the first 204. These differ from saved POST result replay.

### Row locks and validators

In [mutations.go](../contactpg/mutations.go), PUT/PATCH begin a transaction and
read the row with `SELECT ... FOR UPDATE`. PATCH merges into that locked value;
then normal validation, name checks, and `Conditions.Check` precede the write.
The write and commit use the same transaction. DELETE similarly holds the row
lock from its condition check through removal.

A second writer waits for the row and then reads the newly committed values.
Two requests using the same old If-Match validator cannot both change the
contact successfully. Two unconditional PATCHes of different fields preserve
both changes. PUT still intentionally replaces all fields. Context cancellation
can stop waiting for the database lock, and rollback releases it.

### Preserve Go string and collection behavior

The existing domain accepts valid UTF-8 strings, including NUL. PostgreSQL
`text` cannot store NUL, so this service schema stores name/email/phone as
`bytea`; the adapter converts to and from ordinary Go strings. A binary unique
constraint preserves exact, case-sensitive name equality. Shared Go validation
enforces rune limits; database checks add byte-size bounds. Direct SQL writers
must follow the same domain rules.

Collection search reads one ID-ordered SQL snapshot, then calls the shared
Go paging function. This preserves Unicode lowercase, name ordering, and filter
semantics independently of database collation. **It reads the full collection**:
the page limit bounds response size, not database work or application memory.
This lesson preserves the small course API's contract rather than demonstrating
large-dataset SQL pagination. Offset navigation still takes a new snapshot for
each request.

## Keyed creation policy

| Item | Contract |
| --- | --- |
| Header | One `Idempotency-Key`, containing 1–64 ASCII letters, digits, `_`, or `-`. Empty, combined/repeated, or malformed values return 400. |
| Caller namespace | Comes from the application's trusted `SubjectResolver`, not an arbitrary client identity header. |
| Request identity | Original decoded name/email/phone before trimming, in a versioned typed fingerprint. JSON formatting/member order do not matter. |
| Optional fields | As in ordinary POST, omitted, null, and empty optional strings decode to the same domain value. |
| Fresh success | Contact and complete creation snapshot commit together; response is 201 with `Idempotency-Replayed: false`. |
| Repeat | Same caller/key/fields return the original contact, Location, and creation ETag with replay=true. |
| Changed fields | The same caller/key with different valid decoded fields returns 409. Changing field whitespace also changes identity. |
| Errors before commit | Roll back the whole creation and leave no saved key. Corrected input can reuse a key after a known rollback. |
| Lost reply / uncertain commit | Retry the same caller, key, and original fields. A new key can describe new work and does not resolve the uncertain old result. |
| Retention | Successful records have no automatic expiry. Deleting them deliberately forgets their keys. |

The runnable server uses the fixed, operator-configured `-subject local-user`
namespace for a single-user lab. It does not authenticate clients or authorize
access to individual contacts. In a multi-user application, the resolver must
obtain identity from trusted authentication while keeping it stable across
retries. A resolver returning no caller produces 403 for keyed creation; an
invalid trusted identity is a server configuration error. The base chapter
handler rejects keys when keyed creation has not been configured.

`CreateOnce` in [idempotency.go](../contactpg/idempotency.go) follows stage 3:

```text
BeginTx(ReadCommitted)
  acquire transaction advisory lock for caller/key
  read saved result in a new statement after acquiring the lock
  existing matching fingerprint -> historical snapshot
  existing different fingerprint -> key conflict, rollback
  absent record -> INSERT contact + INSERT complete result using the same tx
Commit, then return success and release the lock
```

The saved result has no foreign key to the live contact. Later edits/deletion
do not change a creation replay, and replay does not recreate a deleted contact.
Its ETag identifies the historical response body; GET provides a current view.
Only committed successes are retained. This differs from the earlier generic
gRPC in-memory cache, which also retains terminal errors. External side effects
such as sending email are outside this SQL transaction.

## Read the code in order

1. [Migration](../contactpg/migrations/000001_service_contacts.up.sql) and
   [named queries](../contactpg/queries.sql): current rows, saved results, and
   the two kinds of database lock.
2. [Store](../contactpg/store.go), then
   [mutations](../contactpg/mutations.go): shared normalization, row conversion,
   snapshot paging, and atomic conditional updates.
3. [Keyed creation](../contactpg/idempotency.go), beside
   [HTTP key handling](../../../chapters/13-standard-library/06-project-rest-api/httpapi/idempotency.go).
4. [Generated queries](../internal/servicegen/queries.sql.go): concrete SQL,
   typed parameters, context-aware calls, and scanning. Regenerate with the
   [pinned sqlc command](../04-sqlc/README.md#regenerate-and-detect-drift).
5. [Finite demo](../internal/restapp/demo.go),
   [server lifecycle](../internal/restapp/server.go), and [main.go](main.go).
6. [HTTP/database contracts](../integration/rest_test.go), then
   [concurrency](../integration/rest_concurrency_test.go),
   [cancellation](../integration/rest_context_test.go),
   [process recovery](../integration/rest_process_test.go), and
   [migrations](../integration/rest_migrations_test.go).

Comments describe transaction phases, ownership, and what each test scenario
proves. Generated files retain their generation markers; explanation comments
come from query inputs and the handwritten package documentation.

The service schema has its own `sql_service_schema_migrations` table and uses
golang-migrate. Startup applies only forward migrations and rejects dirty or
unknown future state. Migration pool ownership and per-operation limits follow
stage 2. The SQL module locally imports the course module; the root course
continues to depend only on the standard library.

## Verify the implementation

```sh
go test -count=1 -timeout=60s ./chapters/13-standard-library/06-project-rest-api/...
go -C extensions/02-database-sql test -count=1 -timeout=60s ./...
go -C extensions/02-database-sql vet -tags=integration ./...
go -C extensions/02-database-sql mod tidy -diff
go -C extensions/02-database-sql run ./internal/cmd/sqlgen -check
```

With the container engine and a C compiler available:

```sh
go -C extensions/02-database-sql test -tags=integration -race -count=1 -timeout=5m -v ./...
```

See [Testcontainers/Rancher configuration](../README.md#testcontainers-integration-tests).
Tests create their own disposable databases and never use `DATABASE_URL`.
They compare memory/PostgreSQL HTTP responses, including UTF-8/NUL data, paging,
PUT/PATCH, ETags, and error priority. Other cases cover independent pools/servers,
concurrent keys and conditional writers, rollback after a forced result failure,
cancellation after insertion or while waiting, historical replay, and preservation
of unrelated demo data. Cancellation tests observe actual `pg_stat_activity`
lock waits rather than using sleeps to guess when writes occur.

The restart test discards one child process's successful HTTP reply, then retries
through a new server/pool in a second OS process. It models lost-response
recovery without claiming real packet-loss injection. `TestRESTProcessHelper`
is that test-binary child entry and simply returns during ordinary parent runs.

The next block connects PostgreSQL to the existing gRPC service contracts.

References: [PostgreSQL row/advisory locks](https://www.postgresql.org/docs/18/explicit-locking.html),
[PostgreSQL character storage](https://www.postgresql.org/docs/18/datatype-character.html),
and [HTTP idempotent methods](https://www.rfc-editor.org/rfc/rfc9110.html#name-idempotent-methods).
