# PostgreSQL and database/sql

Learn to send SQL from Go, recover typed results, and manage database resources.
This independent module uses Go 1.27, PostgreSQL, and pgx's `database/sql` driver.
Read functions, pointers, errors, interfaces, modules, context, and testing first.

Execution requirements: a full repository checkout, Go 1.27 on PATH, and a
Docker-compatible engine with the Docker CLI and Compose v2 for manual demos.
Run bash/zsh commands from the root containing `go.mod`, `chapters`, and
`extensions`. The first run can download pinned dependencies/images. Race
checks also require clang or gcc; ordinary tests do not need a running database.

The first stage is a small SQL teaching model. It does not yet replace the
in-memory stores in REST or gRPC. Its nullable email illustrates SQL presence;
the HTTP contact model's normalization and PUT/PATCH rules remain in that project.

| Stage | Status and scope |
| --- | --- |
| 1. Fundamentals | Implemented: [standalone runtime/test instructions](01-fundamentals/README.md), driver/pool lifecycle, parameterized CRUD, NULL, error causes, cancellation, and real PostgreSQL integration tests. |
| 2. Transactions and migrations | Implemented: [atomic batches, rollback, golang-migrate schema changes, and concurrent version checks](02-transactions/README.md). |
| 3. Durable idempotency | Implemented: [request-bound keys, atomic creation/results, concurrent retries, lost replies, and process-independent replay](03-idempotency/README.md). |
| 4. sqlc comparison | Implemented: [generated CRUD beside manual SQL, shared PostgreSQL contracts, explicit transaction binding, and pinned generation checked in CI](04-sqlc/README.md). |
| 5. Service integration | Implemented: [REST with PostgreSQL](05-rest-postgres/README.md) and [gRPC with PostgreSQL](../01-protobuf-and-grpc/06-postgres/README.md) share persistent contacts, atomic version checks, and durable keyed creation. |

## Start a local teaching database

Install a Docker-compatible runtime and start its engine. Check it first:

```sh
go version
docker info
docker compose version
```

From the repository root, use the supplied Compose file:

```sh
docker compose -f extensions/02-database-sql/compose.yaml up -d --wait
```

This starts `postgres:18.6-alpine3.24`, waits for readiness, and exposes it on
`127.0.0.1:5433`. The credentials and database below belong to this local lab.
The image tag is fixed so the demonstration and tests use the same release.
Compose keeps PostgreSQL data in its own named volume.

Run the finite demonstration in a POSIX-compatible shell:

```sh
export DATABASE_URL='postgres://sql_lab:sql_lab_local@127.0.0.1:5433/sql_lab?sslmode=disable'
go -C extensions/02-database-sql run ./01-fundamentals
```

For PowerShell, set `$env:DATABASE_URL` to the same URL, then run the `go -C`
command. `sslmode=disable` matches this loopback-only teaching setup; use the
appropriate certificate verification when configuring a deployed database.
Point the demo at a dedicated lab database: it creates `sql_lab_contacts`,
adds one contact with a random name suffix, and attempts to remove its own row
before exiting. Other rows are preserved. Connection strings contain credentials;
the demo does not print its URL.

The program prints five numbered sections. IDs and page size depend on the
database's existing contents. Look for these summaries:

```text
Stored empty text, then SQL NULL; presence stayed distinct.
Duplicate name -> ErrNameTaken; canceled read -> context.Canceled.
Data belongs to PostgreSQL, rather than to one Go connection pool.
Deleted the demo contact; repeated lookup -> ErrNotFound.
```

With the same database and `DATABASE_URL`, each later demo can also run directly:

```sh
go -C extensions/02-database-sql run ./02-transactions
go -C extensions/02-database-sql run ./03-idempotency
go -C extensions/02-database-sql run ./04-sqlc
go -C extensions/02-database-sql run ./05-rest-postgres
```

They prepare their own schema/migrations and finite demo data. Each lesson's
README also includes its complete runtime and integration-test setup.

Stop the Compose service when finished:

```sh
docker compose -f extensions/02-database-sql/compose.yaml down
```

This keeps its data volume for another session. To intentionally discard that
lab database as well, use `down -v` instead. Tests use separate disposable
containers and never connect to this Compose database.

## Learn in passes

| Pass | Focus | Check your understanding |
| --- | --- | --- |
| 1 | Open a pool and create a table. | Explain why `sql.Open` alone does not confirm connectivity and what `PingContext` checks. |
| 2 | Insert and retrieve one contact. | Trace a Go argument through `$1`, PostgreSQL, `RETURNING`, and `Scan`. |
| 3 | Read a bounded collection and nullable fields. | Explain `Rows.Close`, `Rows.Err`, and NULL versus empty text. |
| 4 | Follow rejected and canceled operations. | Distinguish a missing row, a unique-name conflict, and a canceled context. |
| 5 | Read the integration tests. | Identify who creates the database, isolates each case, closes pools, and removes the container. |

## Terms beside the example

| Term | Meaning here |
| --- | --- |
| PostgreSQL | The database server that stores rows and executes SQL. |
| Driver | pgx's adapter connecting Go's `database/sql` API to PostgreSQL. |
| Connection string / DSN | The settings identifying the server, database, user, and connection options. |
| `*sql.DB` | A reusable handle managing a pool of connections; it is not one permanently reserved connection. |
| Schema | The table structure and constraints. |
| Parameter | A value passed separately from SQL text, represented by `$1`, `$2`, and so on in this driver. |
| Result set | The rows returned by a query, read through `Scan`. |
| SQL NULL | A missing SQL value, distinct from an empty string. |

### SQL vocabulary for the examples

A table stores records as rows; each named field is a column. `INSERT` creates
rows, `SELECT` reads them, `UPDATE` changes them, and `DELETE` removes them.
`WHERE` selects which rows an operation affects. `RETURNING` sends values from
a write back to Go, such as the ID that PostgreSQL just assigned.

The constraints in [schema.sql](contactsql/schema.sql) and the
[transaction lesson's schema](contacttx/migrations/000002_versions_and_changes.up.sql)
let PostgreSQL enforce rules even when several callers write at once:

| SQL term | Meaning in this lab |
| --- | --- |
| `PRIMARY KEY` | Identifies each row uniquely and cannot be NULL; here it is `id`. |
| `GENERATED ALWAYS AS IDENTITY` | Asks PostgreSQL to generate the ID on insert. Generated numbers can have gaps. |
| `NOT NULL` | Requires a stored value. For name, a separate rule also rejects empty text. |
| `UNIQUE` | Prevents two contacts from storing the same name. |
| `CHECK` | Requires a condition, such as the name's length limit, to hold. A SQL NULL condition does not reject a row; use `NOT NULL` when absence is forbidden. |
| `FOREIGN KEY` | Requires a change record's `contact_id` to identify an existing contact. Stage 2 introduces this relationship. |
| `ON DELETE CASCADE` | Deleting that contact also deletes its related change records. It does not delete unrelated contacts. |

For example, a stage-2 change row with `contact_id = 7` belongs to contact 7.
The foreign key rejects it if contact 7 does not exist. Deleting contact 7
removes its change rows through the cascade rule.

The word *schema* can mean the table structure. A PostgreSQL schema such as
`public` is also a namespace grouping tables inside one database. These examples
use that namespace, so `public.sql_lab_contacts` names the stage-1 table.

### Trace an insert from Go to a row

Follow a create:

```text
Input: Name=" Ana ", Email=nil
Go validation: trim name to "Ana", check text limits
QueryRowContext: INSERT ... VALUES ($1, $2) RETURNING id, name, email
Arguments: ["Ana", nil] -- not concatenated into the query text
PostgreSQL: assign an ID, check constraints, store the row
Scan: read id/name and a sql.NullString for email
Result: Contact{ID: assigned ID, Name: "Ana", Email: nil}
```

A quote in an argument remains data. The integration test stores a name that
looks like SQL and verifies that the table remains usable. Parameters bind
values, rather than table or column names; this lesson keeps identifiers fixed.

## Read the code in order

1. [schema.sql](contactsql/schema.sql): identity ID, required name, unique index,
   and nullable email. `CREATE TABLE IF NOT EXISTS` bootstraps this table; it
   does not migrate an existing table whose definition differs. Stage 2 adds
   [versioned migrations](02-transactions/README.md) for its separate tables.
2. [database.go](contactsql/database.go): driver registration, `sql.Open`, a
   four-connection teaching limit, `PingContext`, and embedded schema execution.
   The caller owns `Close`; stores borrow the pool. The chosen size is a lab
   bound, rather than a general performance recommendation.
3. [store.go](contactsql/store.go): SQL statements beside the operations they
   implement. `QueryRowContext` plus `Scan` retrieves one row; `QueryContext`
   iterates several rows; `ExecContext` supplies a result with `RowsAffected`.
4. [demo.go](internal/demo/demo.go) and [main.go](01-fundamentals/main.go): a
   caller-owned deadline, finite operations, a second independent pool, and
   cleanup with a fresh budget. Defers in `run`/`Run` complete before `main`
   reports an error and exits.
5. [Unit tests](contactsql/store_test.go), then
   [integration tests](integration/fundamentals_test.go): text boundaries/error
   causes first, real SQL behavior and ownership next. Stage 4 runs those same
   contracts against a [sqlc adapter](04-sqlc/README.md); the manual implementation
   remains available to study each database/sql step.

### Values, NULL, and rows

`Input.Email == nil` sends SQL NULL. A pointer to `""` sends empty text. Scan
first uses `sql.NullString`: `Valid == false` represents NULL. Returned contacts
have independent optional email values. The caller must keep input stable
until its operation returns.

`UpdateEmail` assigns only email. Its nil argument means **store NULL**; this
differs from the REST domain's `Changes`, where nil means **keep the field**.
Transport updates will need an explicit mapping in stage 5.

Rows can fail after a query has started. Always check `Rows.Err` after iteration
and close rows on early returns to release the borrowed connection. An empty
collection is not `sql.ErrNoRows`; a missing single-row lookup is. ID order is
explicit because SQL results have no default ordering. Identity sequences can
have gaps, including after a rejected insert; use returned IDs, not assumptions
that they are consecutive.

### Idempotency: state, uniqueness, and request keys

An operation is idempotent when repeating it has the same intended effect as
performing it once. The response can differ. Compare three ideas in this lab:

| Mechanism | Current behavior | Test coverage |
| --- | --- | --- |
| Assign an email | Repeating the same value leaves the contact's ID, name, and email unchanged after the first assignment. nil assigns SQL NULL. | Explicit repetitions for nonempty text, empty text, and NULL; each checks the returned value, a fresh read, and that only one contact remains. |
| Delete a contact | The contact stays absent. The first call succeeds; the repeated call returns `ErrNotFound`. | The CRUD case repeats deletion and verifies that the contact is still missing. |
| Unique name | PostgreSQL rejects another row with that name, including concurrent inserts. | Duplicate creation and six concurrent attempts: one creation and five name conflicts. |

For example, assigning `"ana@example.com"` twice leaves that email stored.
PostgreSQL can execute both UPDATE statements; these tests check the requested
contact state, rather than asserting that the second statement never runs.

The unique-name constraint protects a business rule. It does not identify a
client's original request or save its response for later retrieval. A repeated
create in stage 1 returns `ErrNameTaken`; that store has no request-key records.

The related course examples have different scopes:

- [REST fundamentals](../../chapters/13-standard-library/06-project-rest-api/README.md#idempotency-and-atomicity)
  tests repeated PUT, DELETE, and its assignment/removal PATCH operations. Its
  [REST with PostgreSQL](05-rest-postgres/README.md) project adds durable keyed POST
  through the shared HTTP adapter; the base in-memory command keeps ordinary POST.
- [gRPC resilience](../01-protobuf-and-grpc/05-resilience/README.md) binds a
  creation key to the caller and request, and saves results in memory. Its tests
  include concurrent retries and a simulated lost reply. The records do not
  survive a process restart.
- [gRPC with PostgreSQL](../01-protobuf-and-grpc/06-postgres/README.md) uses the
  same service store/ledger as persistent REST. It retains committed successes
  and tests concurrent RPCs, cancellation, cross-transport writes/replay, and
  separate-process recovery.
- The SQL fundamentals exercise repeated state changes and database uniqueness.
  [Stage 3](03-idempotency/README.md) adds durable request-key handling before
  transport wiring.

The durable stage stores a business write and its complete creation result in
one transaction. Its tests cover repeated requests, changed data under an
existing key, concurrent requests, rollback, cancellation, and a lost reply
followed by recovery in a separate process. Retained result snapshots remain
replayable even after the contact changes or is deleted. This stage saves only
successful committed results; its guide explains the retry and retention policy.

### Errors and cancellation

Use `errors.Is` for `ErrInvalidInput`, `ErrNotFound`, `ErrNameTaken`, and context
errors. Missing-row mapping retains `sql.ErrNoRows`; duplicate-name mapping
checks PostgreSQL SQLSTATE `23505` and the named constraint, preserving the
`*pgconn.PgError` for `errors.As`. Other database failures remain wrapped causes.

Every business database operation receives a context. Its budget can include waiting for
an available pooled connection. A test reserves the only connection, lets a
waiting query's deadline expire, then verifies that the pool remains usable.
Cancellation of a database call does not imply rollback of an already committed
write. Each stage-1 statement commits independently. Continue to
[transactions and migrations](02-transactions/README.md) for multi-statement
atomicity, rollback, and the limits of Commit acknowledgments.

## Testcontainers integration tests

Ordinary tests need Go and the module dependencies, with no running database:

```sh
go -C extensions/02-database-sql test -count=1 -timeout=60s ./...
go -C extensions/02-database-sql vet ./...
go -C extensions/02-database-sql mod tidy -diff
```

Explicitly select integration tests when the container engine is running:

```sh
docker info
go -C extensions/02-database-sql test -tags=integration -count=1 -timeout=5m -v ./...
```

With a C compiler available, also instrument Go code for races:

```sh
CGO_ENABLED=1 go -C extensions/02-database-sql test -tags=integration -race -count=1 -timeout=5m ./...
```

Testcontainers starts real, disposable PostgreSQL servers, discovers their mapped
ports, waits for readiness, and registers container cleanup. Each top-level
suite owns a container and runs its cases serially. Stage 1 truncates its own
test table; stages 2 and 3 reset only their respective tables and migration state.
The fundamentals contracts run against both the manual and sqlc-backed stores.
They cover CRUD, NULL/empty values, repeated email assignments, repeated
deletion, parameter binding, concurrent uniqueness, reading data after
closing/reopening a pool, deadline cancellation while waiting for a connection,
validation/error causes, and independent result values. Finite demos check their
own row cleanup; the [sqlc cases](integration/sqlc_test.go) additionally verify
transaction binding and preservation of other rows.
The [stage-2 suite](integration/transactions_test.go) adds migration upgrades and
dirty-state failures, concurrency, multi-statement rollback, and in-flight
cancellation. Its [migration cases](integration/migrations_test.go) also cover
explicit adoption of the old runner's history and isolated down/up cycles.
The [stage-3 suite](integration/idempotency_test.go) checks durable creation and
replay, request binding, concurrent retries across pools, atomic rollback,
cancellation, historical results, and lost-reply recovery across child processes.
The [persistent REST suite](integration/rest_test.go) adds matching HTTP responses
across memory/PostgreSQL, atomic conditional writes, keyed POST, cancellation,
and HTTP recovery across separate processes.

The `integration` build tag is intentional. Without it, the Docker-dependent
package is excluded. With it, an unavailable engine causes a failing test;
the suite does not silently skip database verification. The first run may pull
PostgreSQL and Testcontainers' cleanup-helper image. The tests never use a
learner-supplied `DATABASE_URL`.

For Rancher Desktop, use its Docker-compatible **moby** engine and follow the
[Testcontainers configuration guide](https://golang.testcontainers.org/system_requirements/rancher/).
When its socket is under your home directory and mapped ports are available on
localhost, typical shell settings are:

```sh
export DOCKER_HOST="unix://$HOME/.rd/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1
```

Other Rancher networking configurations use a different reachable host address;
`rdctl shell ip a show vznat` shows the VZ interface for inspecting its VM address.
Use that reachable address when localhost forwarding is unavailable. Do not export `TESTCONTAINERS_HOST_OVERRIDE`
with an empty value: it can produce a URL with no host and make PostgreSQL's
driver try a Unix socket instead. `docker info` must succeed before
running integration tests. Keep these machine-specific settings in your shell,
rather than hardcoding them in the tests or CI.

## Validation and next stages

The [course workflow](../../.github/workflows/go.yml) includes this seventh module
in ordinary tests/vet/metadata and race checks. A dedicated PostgreSQL integration
job runs the tagged tests with the race detector on a Docker-enabled Linux runner.
See the [validation guide](../../docs/validation.md#postgresqlsql-module).

Stages 1–5 are implemented. After the standalone fundamentals, read
[transactions and migrations](02-transactions/README.md), then
[durable idempotency](03-idempotency/README.md) and the
[sqlc comparison](04-sqlc/README.md). Continue to
[REST with PostgreSQL](05-rest-postgres/README.md), then
[gRPC with PostgreSQL](../01-protobuf-and-grpc/06-postgres/README.md). The manual
`database/sql` implementation remains the foundation for learning Scan, result
sets, error causes, and resource ownership. The sqlc adapter shares its CRUD
contracts and real PostgreSQL tests. The workflow also checks pinned generation
with `go -C extensions/02-database-sql run ./internal/cmd/sqlgen -check`.
The separate [MongoDB fundamentals lab](../03-mongodb/01-fundamentals/README.md)
now introduces document CRUD, BSON presence, unique indexes, and real database
tests. Its [second stage](../03-mongodb/02-document-modeling/README.md) adds
embedding, filters/projections, and nested updates. Its
[third stage](../03-mongodb/03-schema-evolution/README.md) introduces database
validation and repeatable data evolution. Its
[fourth stage](../03-mongodb/04-references-and-aggregation/README.md) adds references,
aggregation reports, and explain. The independent
[Valkey fundamentals](../04-valkey-cache/01-fundamentals/README.md) introduce
remote cache values, TTLs, and serialization. The
[cache-aside stage](../04-valkey-cache/02-cache-aside/README.md) reuses this SQL
adapter with cold/warm contact-read counters and commit-before-invalidation.
Cache concurrency/failure policy and dedicated REST/gRPC cache labs remain
later blocks; guided exercises stay last.

References: [Go database/sql guides](https://go.dev/doc/database/),
[pgx's stdlib adapter](https://pkg.go.dev/github.com/jackc/pgx/v5/stdlib),
[Testcontainers PostgreSQL module](https://golang.testcontainers.org/modules/postgres/),
[PostgreSQL constraints](https://www.postgresql.org/docs/18/ddl-constraints.html),
and [sqlc documentation](https://docs.sqlc.dev/en/latest/).
