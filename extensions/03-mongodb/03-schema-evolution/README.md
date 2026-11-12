# Validation and schema evolution

Add database-enforced rules, read old and new documents during a rollout, and
backfill data without assuming the entire collection changes atomically. This
stage can be run directly; Go context, BSON, and MongoDB fundamentals are useful
background. The separate [references/aggregation stage](../04-references-and-aggregation/README.md)
builds on document persistence with lookups and reports.

A backfill updates documents already stored in the database to the new shape.
Here it moves the city into an embedded address while keeping its original value.

## Requirements and execution

Use Go 1.27 on PATH, a full checkout, and a running Docker-compatible engine
with Docker CLI and Compose v2. Run bash/zsh from the repository root containing
`go.mod`, `chapters`, and `extensions`. First runs download pinned dependencies
and MongoDB 8.0.38. Race checks additionally need clang or gcc.

```sh
go version
docker info
docker compose version
docker compose -f extensions/03-mongodb/compose.yaml up -d --wait
docker compose -f extensions/03-mongodb/compose.yaml --profile schema run --rm schema-setup
export MONGODB_URI='mongodb://mongo_lab:mongo_lab_local@127.0.0.1:27018/mongo_lab?authSource=mongo_lab'
export MONGODB_SCHEMA_URI='mongodb://mongo_schema:mongo_schema_local@127.0.0.1:27018/mongo_lab?authSource=mongo_lab'
go -C extensions/03-mongodb run ./03-schema-evolution
```

Compose exposes real MongoDB on `127.0.0.1:27018` and keeps its data in a named
volume. The official Go driver is pinned at v2.10.0. These local teaching
credentials use loopback without transport TLS. `authSource=mongo_lab` selects
the database holding the accounts; code explicitly selects the data namespace.

### Why there are two connection variables

| Account | Role in `mongo_lab` | Used for |
| --- | --- | --- |
| `mongo_lab` | `readWrite` | Data reads/writes, backfill, and owned collection creation/cleanup |
| `mongo_schema` | `dbAdmin` | `collMod` to install collection validators |

The explicit `schema-setup` command uses the image's local admin account only
to provision the database-scoped schema account. It works with an existing lab
volume and can be repeated: an existing schema user's credentials/roles are
retained. It does not grant schema permissions to the application user. If you
previously changed credentials, use those values rather than expecting setup
to rotate them. The first two labs need only `MONGODB_URI`.

The demo creates a randomly named `schema_demo_*` collection, changes only its
validator/data, and finally drops that owned collection with a fresh cleanup
budget. Other lab data is retained. It uses a 30-second operation context and
separate bounded client/collection cleanup contexts.

Look for these lines among the five sections:

```text
New invalid write -> server code 121; an invalid old document still exists.
One reader understands v1 city and v2 address.city; new Go writes use v2.
Repaired the known bad value; existing documents now match a supported schema.
First batch committed; cancellation did not undo it. Resume -> 3 migrated; repeat -> 0.
v2-only policy: legacy writer rejected; current writer accepted; original city preserved.
```

## Learn in passes

| Pass | Read | Check your understanding |
| --- | --- | --- |
| 1 | Run the demo, then compare [v1.json](../schemaevolution/schemas/v1.json) and [v2.json](../schemaevolution/schemas/v2.json). | Identify required fields, BSON types, and the changed city location. |
| 2 | Follow [reader.go](../schemaevolution/reader.go) and `InstallCompatible` in [validation.go](../schemaevolution/validation.go). | Explain how new writers and old data coexist. |
| 3 | Follow one batch in [migrate.go](../schemaevolution/migrate.go). | Find the size limit, repeated version/shape check, server-side field expression, and atomic document update. |
| 4 | Read `Finalize` and the commented [integration cases](../integration/schema_test.go). | Explain why installing a validator is different from auditing existing data. |

## The two stored shapes

```javascript
// Original document: schemaVersion may be absent or integer 1.
{ _id: ObjectId("..."), name: "Ana", city: "Guadalajara" }

// Current document: schemaVersion must be integer 2.
{
  _id: ObjectId("..."),
  name: "Ana",
  schemaVersion: 2,
  address: { city: "Guadalajara" }
}
```

The version is a property of each document, not a global database migration
number. The reader interprets a missing version as the known original shape.
Explicit null, strings, floats, conflicting old/new city fields, or unknown
versions are failures. Integer BSON `int` and `long` are accepted for versions.

`Read` returns one normalized Go read model with ID, name, City, and StoredVersion.
It does not write on read. `Insert` trims/validates new input and always writes v2.
Do not marshal the normalized read model back as a replacement document: it does
not contain every stored field. Unknown fields are retained by the migration.

Reading compatibility does not migrate a stored record. Both known shapes map
to the same Go read model; unsupported data returns an error:

```mermaid
flowchart TD
    R[Read raw BSON document] --> V{Supported valid shape?}
    V -->|Version absent or integer 1| A[Read top-level city]
    V -->|Integer version 2| B[Read address.city]
    V -->|Malformed or unsupported| E[Return document or version error]
    A --> G[Go Profile with City and StoredVersion]
    B --> G
```

Only the explicit backfill changes v1 documents to v2. New Insert calls already
write v2; no read in this diagram writes back to MongoDB.

## Rules in Go and in MongoDB

Go validation gives callers early errors. A MongoDB `$jsonSchema` validator also
checks writes made by other clients. The embedded JSON files specify ObjectID
identity, required nonempty text of at most 80 characters without NUL, version
types/values, and the appropriate city location. Other fields are allowed.
Existing text is preserved; the migration does not trim or guess its meaning.

The compatibility validator accepts either valid v1 or valid v2. V1 must not
already contain `address`; v2 must not retain top-level `city`. A collision needs
an explicit repair decision rather than overwriting one of two possible values.

`collMod` installs:

```javascript
validationLevel: "strict"
validationAction: "error"
```

In MongoDB 8, `strict` checks all inserts/updates, while `moderate` exempts updates
to pre-existing documents that already fail the rules. Neither level repairs or
retroactively certifies old data. `error` rejects a violating write; `warn` would
allow it and log a warning. This lesson uses strict/error throughout and checks
stable server code **121**, preserving driver errors through wrapping.

The demo intentionally inserts one malformed document before validation exists.
Adding the validator succeeds, and that document is still present. An audit flags
it. Updating an unrelated field does not repair it; an explicit correction must
produce a valid document. The migration never coerces its numeric city to text.

## Expand, backfill, contract

1. **Expand:** explicitly install validation that accepts v1 and v2. Deploy a
   reader that understands both and new writers that produce only v2.
2. **Audit and repair:** `CheckCompatible` finds unsupported shapes. Repair
   known bad data deliberately and investigate future versions.
3. **Backfill:** repeatedly call `BackfillBatch` with a size from 1 to 100.
   Each call selects at most that many valid legacy IDs, then rechecks the
   legacy shape in its write filter.
4. **Contract:** retire old writers, call `Finalize` to require v2, then audit
   the existing collection under those rules. Remove compatibility code only
   after your rollout's data and application requirements allow it.

The backfill uses MongoDB's native update pipeline:

```javascript
[
  { $set: { "address.city": "$city", schemaVersion: 2 } },
  { $unset: "city" }
]
```

`"$city"` is a server-side field expression. It reads the document's current
city when the update executes, not a stale copy read earlier in Go. A city value
beginning with `$` remains data. The pipeline is atomic **per document** and
preserves other fields. Already-current/deleted documents no longer match the
write filter, so concurrent workers can safely converge on the remaining v1 set.

Document versions provide progress: cancel or stop between batches, then reconnect
and select remaining v1 documents. A repeat after completion modifies zero records.
The batch limit bounds selected IDs and writes, not all work needed to find them.
An error/cancellation can follow committed writes; neither UpdateMany nor the
whole rollout is a collection-wide transaction. Inspect/retry instead of assuming
rollback. Selected, Matched, and Modified counts can differ under concurrency.

This is an application data-backfill example using native MongoDB commands,
not a general framework for migration history, locks, deployments, or rollback.
A production rollout must coordinate application versions and operational
recovery. A request idempotency key is also a separate contract.

### Why Finalize installs v2 before its audit

Counting legacy documents first leaves a window for an old writer to insert
another one. `Finalize` closes legacy writes first, then checks existing data.
If incompatible documents remain, it returns `ErrMigrationRequired` and leaves
the v2 policy installed. Valid legacy records can still be updated directly to
valid v2 by the backfill; repair unsupported records deliberately, then retry
finalization. Errors never cause an automatic fallback to the permissive policy.

After expansion, compatible readers/new v2 writers, and the initial backfill,
the cutover follows this order:

```mermaid
flowchart TD
    R[Retire old v1 writers] --> I[Finalize installs the strict v2 validator]
    I --> A[Audit existing documents against v2]
    A --> Q{All documents satisfy v2?}
    Q -->|Yes| S[Finalize succeeds]
    Q -->|No| E[ErrMigrationRequired; keep the v2 validator]
    E --> F[Backfill valid v1 or explicitly repair unsupported data]
    F --> T[Retry Finalize without reinstalling compatibility policy]
    T --> I
```

Existing v1 rows can remain after the validator changes. They may still be
updated into valid v2 rows. This loop is a rollout/recovery sequence, not one
transaction over the collection.

`InstallCompatible` is an explicit rollout step, not startup housekeeping.
Calling it after finalization would reopen old writes. Repeating the backfill
does not reapply that earlier policy. The finite demo uses a fresh collection
for each complete teaching run so rollout phases do not reset shared policy.

## Ordinary tests and checks

These run without MongoDB or a container engine:

```sh
go -C extensions/03-mongodb test -count=1 -timeout=60s ./...
go -C extensions/03-mongodb vet ./...
go -C extensions/03-mongodb mod tidy -diff
```

Unit tests cover BSON/version compatibility, missing/null/type distinctions,
future-version errors, malformed data, input boundaries, and error causes.
With a C compiler available:

```sh
CGO_ENABLED=1 go -C extensions/03-mongodb test -race -count=1 -timeout=90s ./...
```

## Integration tests with Testcontainers

Start the engine. Tests own real authenticated MongoDB servers, wait for readiness,
and clean up their containers. Schema tests provision their own readWrite/dbAdmin
accounts. They never read `MONGODB_URI` or `MONGODB_SCHEMA_URI` and do not require
Compose or the manual schema-setup command.

For Rancher Desktop on macOS, select **dockerd (moby)**. With its home-directory
socket and ports forwarded to localhost, configure the test terminal:

```sh
export DOCKER_HOST="unix://$HOME/.rd/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1
```

Docker Desktop/local Docker usually uses automatic detection. A Rancher VM
networking setup may require its reachable VM address instead; inspect it with
`rdctl shell ip a show vznat`. Keep the host override nonempty. With clang or gcc:

```sh
docker info
CGO_ENABLED=1 go -C extensions/03-mongodb test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/03-mongodb vet -tags=integration ./...
```

The command runs all three stages. New cases check role boundaries, actual server
validation, malformed pre-existing data, explicit repair, a bounded first batch,
cancellation, reconnect/resume, concurrent backfill, unknown-field preservation,
future versions, and failed/passed final audits. They prove old writes stay
rejected after repeating the backfill and run the finite demo twice while
preserving another collection. A missing engine fails tagged tests.

Successful packages print `ok`; `[no test files]` is normal for command/demo
packages exercised by the integration suite. Successful vet/metadata checks
normally print nothing. Initial runs can download MongoDB and cleanup-helper images.

## Stop the manual database

After the Go processes finish:

```sh
docker compose -f extensions/03-mongodb/compose.yaml down
```

This retains MongoDB data and accounts. `down -v` deliberately discards this lab's
volume, including previous stages' data. The schema-setup service is one-shot
and removed by `run --rm`; existing volumes do not need to be deleted to add its user.

Read in order: [main.go](main.go), [demo.go](../internal/schemademo/demo.go),
[validation.go](../schemaevolution/validation.go), [reader.go](../schemaevolution/reader.go),
[migrate.go](../schemaevolution/migrate.go), and [tests](../integration/schema_test.go).

References: [JSON Schema validation](https://www.mongodb.com/docs/manual/core/schema-validation/specify-json-schema/),
[validation levels](https://www.mongodb.com/docs/manual/core/schema-validation/specify-validation-level/),
[changing validation](https://www.mongodb.com/docs/manual/core/schema-validation/update-schema-validation/),
and [update pipelines](https://www.mongodb.com/docs/manual/tutorial/update-documents-with-aggregation-pipeline/).
