# Compare manual SQL with sqlc

Write the SQL yourself, then let sqlc generate typed Go parameters, result rows,
and query methods. This lesson puts those methods beside the manual
`database/sql` store so you can follow what changed and what still belongs to
the application.

Start with [SQL fundamentals](../README.md). Read
[transactions and migrations](../02-transactions/README.md) before the `WithTx`
example. [Durable idempotency](../03-idempotency/README.md) provides the preceding
course block; its creation policy remains a separate operation.

## Run the comparison

From the repository root:

```sh
docker compose -f extensions/02-database-sql/compose.yaml up -d --wait
DATABASE_URL='postgres://sql_lab:sql_lab_local@127.0.0.1:5433/sql_lab?sslmode=disable' \
  go -C extensions/02-database-sql run ./04-sqlc
```

Use the dedicated lab database described in the [setup guide](../README.md#start-a-local-teaching-database).
The program uses the supplied generated source; you can run it before installing
or running sqlc. Look for four summaries:

```text
Generated and manual stores read the same PostgreSQL row.
Both stores preserve empty text versus SQL NULL.
WithTx uses our transaction; rollback discards its update.
Same ErrNameTaken/ErrNotFound contract; SQL causes remain inspectable.
```

The finite demonstration owns its pool, creates a contact with a random name,
and attempts to remove that contact before exiting. Other rows are preserved.
It shares `sql_lab_contacts` with the fundamentals lesson, which makes reads
through either store directly comparable. The transaction and idempotency
lessons retain their own tables and migration versions.

This comparison reuses `EnsureSchema` to bootstrap the fundamentals table.
Generation reads a schema file; it does not create or migrate the live database.
Changing an existing deployed schema still requires the migration workflow
learned in stage 2. `CREATE TABLE IF NOT EXISTS` cannot evolve an existing table.

## Learn in passes

| Pass | Follow | Check your understanding |
| --- | --- | --- |
| 1 | A named SQL query and its generated method. | Locate the parameters, SQL text, context, and `Scan`. |
| 2 | The handwritten adapter around that method. | Identify validation, NULL conversion, and error classification. |
| 3 | One row written/read through both stores. | Explain why they have the same behavior despite different Go query code. |
| 4 | `Queries.WithTx(tx)`, then rollback and commit tests. | Identify who begins and ends the transaction. |
| 5 | Regeneration and the CI check. | Explain which files you edit and how drift is detected. |

## Terms beside the example

| Term | Meaning here |
| --- | --- |
| sqlc | A development tool that analyzes SQL and a schema to generate typed query bindings. |
| Binding | Go code that sends a particular SQL query, supplies its parameters, and scans its results. |
| Annotation | A SQL comment such as `-- name: CreateContact :one`, defining the generated method name and result shape. |
| Generated row | A database-facing Go struct, here `SqlLabContact` with a `sql.NullString` email. |
| Adapter | Handwritten code translating generated database types into the existing `contactsql.Contact` API. |
| Generation drift | Generated files differ from what the current schema, queries, configuration, and pinned tool would produce. |

### Trace one create through both implementations

The manual store spells out the database interaction:

```text
contactsql.Store.Create
  normalize input
  db.QueryRowContext(SQL, name, email)
  Scan(id, name, nullable email)
  classify errors and return Contact
```

The sqlc adapter follows the same contract:

```text
contactsqlc.Store.Create
  normalize input
  convert *string -> sql.NullString
  generated Queries.CreateContact(CreateContactParams)
    QueryRowContext(SQL, name, email)
    Scan into SqlLabContact
  classify errors and convert the row -> Contact
```

The SQL remains visible in [queries.sql](../contactsqlc/queries.sql):

```sql
-- name: CreateContact :one
INSERT INTO sql_lab_contacts (name, email) VALUES ($1, $2)
RETURNING id, name, email;
```

sqlc infers `Name string`, `Email sql.NullString`, and the returned row fields
from this query and the schema. A schema/query mismatch can therefore fail
generation before the application runs. Real database tests still check behavior
that static analysis cannot establish, including constraints and cancellation.

The five queries use three result annotations:

| Annotation | Generated shape | Example |
| --- | --- | --- |
| `:one` | One row and an error; missing rows retain `sql.ErrNoRows`. | Create, get, update email. |
| `:many` | A slice and an error, with row iteration and cleanup. | List contacts by increasing ID. |
| `:execrows` | An affected-row count and an error. | Delete, where zero means no matching contact. |

`sqlc.arg(page_size)` names the list parameter. Its SQL `integer` cast produces
an `int32` argument; the adapter checks the 1–100 limit before converting Go's
`int`. The configuration requests empty slices for empty generated collections.
The adapter also returns a non-nil empty `[]contactsql.Contact`, matching the
manual API.

### Keep application rules in the adapter

Both stores reuse `NormalizeInput`, `ValidateID`, `ValidateEmail`, and
`QueryError` from `contactsql`. Sharing these small rules prevents the variants
from disagreeing about valid input or the meaning of a database error. The
manual store still contains its original SQL, scanning, and iteration flow.

sqlc handles parameter/result types and query plumbing. We still decide:

- How to trim a name and validate text limits before writing.
- How to represent optional fields in the public model. nil means SQL NULL;
  a pointer to `""` means present empty text.
- Which named constraint means `ErrNameTaken`, and how `ErrNotFound` retains
  `sql.ErrNoRows`. Driver and context causes remain available to `errors.As`/`Is`.
- Who owns the pool, supplies deadlines, defines transactions, and applies
  migrations.

Each returned optional email owns its value. Changing a returned pointer does
not mutate another result or the stored row. The generated row types stay inside
the SQL module rather than becoming HTTP or Protobuf messages.

This is query generation, not an ORM managing entity relationships or tracking
object changes. Both variants execute parameterized SQL through the same pgx
`database/sql` driver. The lesson makes no performance claim; choosing a tool
does not replace measuring a real workload.

### Bind queries to a transaction

The generated `DBTX` interface contains methods implemented by both `*sql.DB`
and `*sql.Tx`. `Queries.WithTx(tx)` returns another query object using that
transaction. The caller still follows the familiar flow:

```text
db.BeginTx(ctx, options)
defer tx.Rollback()
qtx := queries.WithTx(tx)
qtx performs every query belonging to this transaction
tx.Commit() -- only when the complete operation should succeed
```

The finite demo deliberately rolls back its temporary email update. The tests
also commit a generated insert and observe it through the manual store. Using
the original pool-backed `queries` for one of these writes would place it
outside the intended transaction. Generated code does not choose that boundary.

The raw query example uses known-valid teaching values. In an application, the
surrounding operation must still validate inputs and map errors. The basic sqlc
CRUD adapter does not implement `CreateOnce`; the durable policy from stage 3
must remain intact when services are connected later.

## Read the code in order

1. [Shared schema](../contactsql/schema.sql), then
   [sqlc.yaml](../sqlc.yaml): the schema input, query input, output directory,
   and `database/sql` generator setting.
2. [queries.sql](../contactsqlc/queries.sql): named operations and result shapes.
   Its explanation comments are copied onto the generated methods.
3. [Generated queries](../internal/sqlcgen/queries.sql.go),
   [row model](../internal/sqlcgen/models.go), and
   [database binding](../internal/sqlcgen/db.go). Read the generated code to
   compare it with the manual store; make changes in its SQL/config inputs.
4. [Adapter](../contactsqlc/store.go), beside the
   [manual store](../contactsql/store.go): the same API and shared rules.
5. [Demonstration](../internal/sqlcdemo/demo.go) and [main.go](main.go): both
   stores sharing a pool, a caller-owned transaction, and finite cleanup.
6. [Shared contracts](../integration/fundamentals_test.go) and
   [comparison cases](../integration/sqlc_test.go): both implementations run the
   same scenarios through a small interface declared by the tests that consume it.
7. [Generation command](../internal/cmd/sqlgen/main.go): development tooling,
   read after the runtime flow.

Handwritten code and tests explain their phases and expected outcomes.
[doc.go](../internal/sqlcgen/doc.go) explains the generated infrastructure without
editing files marked `Code generated ... DO NOT EDIT`.

## Regenerate and detect drift

From the repository root:

```sh
go -C extensions/02-database-sql generate .
go -C extensions/02-database-sql run ./internal/cmd/sqlgen -check
```

The wrapper pins **sqlc v1.31.1** and invokes it with a versioned `go run` command.
The first generation can download and build the tool; subsequent runs reuse
Go's cache. It needs no global sqlc installation or running PostgreSQL server
for this configuration. Its build dependencies stay separate from the lesson's
`go.mod`/`go.sum` graph, and the application compiles the supplied Go bindings.

`-check` uses `sqlc diff` to compare expected generated output with the files on
disk without rewriting them. A mismatch fails the command. The
[course workflow](../../../.github/workflows/go.yml) runs this exact check in the
SQL module's job, alongside tests, vet, and module metadata checks. Generation
also works with `CGO_ENABLED=0`, the setting used in that job.

After editing a query or schema input, regenerate, inspect the generated diff,
and run the real PostgreSQL contracts. Update the pinned tool deliberately;
review its output changes together with the SQL and adapter changes.

## Test the shared behavior

Ordinary tests compile the generated code without Docker or running sqlc:

```sh
go -C extensions/02-database-sql test -count=1 -timeout=60s ./...
go -C extensions/02-database-sql vet ./...
go -C extensions/02-database-sql mod tidy -diff
```

For PostgreSQL contracts, start the container engine. The race command also
needs a C compiler:

```sh
go -C extensions/02-database-sql test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/02-database-sql vet -tags=integration ./...
```

Follow the [Testcontainers/Rancher setup](../README.md#testcontainers-integration-tests)
when needed. Tests own disposable databases and never read your `DATABASE_URL`.
`TestPostgreSQL/manual` and `TestPostgreSQL/sqlc` each cover CRUD, NULL/empty
values, repeated email assignments/deletion, literal parameters, reopening a
pool, concurrent unique-name enforcement, cancellation while waiting for a pool
connection, validation/error causes, and independent result values. Separate
cases cover generated transaction commit/rollback and the comparison demo's
preservation of unrelated rows. The earlier migration and durable-idempotency
suites continue to run with the same command.

Some packages show `[no test files]` in an ordinary run: their database contracts
live in the explicitly tagged `integration` package. The generated files are
checked by compilation, the drift check, and those real database tests.

Next, connect PostgreSQL adapters to the REST and gRPC service contracts,
including durable request-key handling for creation.

References: [sqlc v1.31.1](https://github.com/sqlc-dev/sqlc/releases/tag/v1.31.1),
[configuration](https://docs.sqlc.dev/en/latest/reference/config.html),
[transactions](https://docs.sqlc.dev/en/v1.31.1/howto/transactions.html), and
[generation checks in CI](https://docs.sqlc.dev/en/v1.31.1/howto/ci-cd.html).
