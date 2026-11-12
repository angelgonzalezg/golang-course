# MongoDB fundamentals

Use the official MongoDB Go driver to insert, read, update, and delete real
documents. Start with structs, pointers, errors, context, and testing. SQL
experience helps with the comparison below, but the setup is complete here.

## Requirements and execution

Use Go 1.27 on PATH, a full repository checkout, and a running Docker-compatible
engine with Docker CLI and Compose v2. Run bash/zsh commands from the repository
root containing `go.mod`, `chapters`, and `extensions`. The first run downloads
pinned Go dependencies and the MongoDB image. Race checks also need clang or gcc.

```sh
go version
docker info
docker compose version
docker compose -f extensions/03-mongodb/compose.yaml up -d --wait
export MONGODB_URI='mongodb://mongo_lab:mongo_lab_local@127.0.0.1:27018/mongo_lab?authSource=mongo_lab'
go -C extensions/03-mongodb run ./01-fundamentals
```

This starts **MongoDB 8.0.38**, a real server in a local container, on
`127.0.0.1:27018`. Compose retains its data in a named volume. The Go module pins
the official **v2.10.0** driver; BSON and MongoDB types come from its `/v2` packages.

The image initializes an admin account and a `mongo_lab` user with read/write
access only to the `mongo_lab` database. The URL uses the latter account.
`authSource=mongo_lab` tells MongoDB where to authenticate that user. The Go
code explicitly selects `client.Database("mongo_lab").Collection("contacts")`.
This loopback teaching setup uses local credentials without transport TLS.

Initialization scripts run only for a new empty volume. Editing those credentials
later does not change users in an existing database. Keep the original lab
settings or deliberately recreate this lab's volume using the cleanup section.

The finite walkthrough prints five sections, then exits. Look for:

```text
Created ObjectID <24 hexadecimal characters>; omitted email is a missing field.
Stored empty text with $set, then removed email with $unset.
Duplicate name -> ErrDuplicate; canceled read -> context.Canceled.
Data belongs to MongoDB, rather than to one Go client.
Deleted the demo contact; repeated lookup -> ErrNotFound.
```

Each run uses a random name and attempts to remove only its own document. Other
contacts, the collection, and indexes remain. Go client shutdown does not delete
server-owned data. The demo opens a second client to show that ownership;
normal application queries should share one long-lived client.

## Learn in passes

| Pass | Read and run | Check your understanding |
| --- | --- | --- |
| 1 | Run the demo; read [main.go](main.go) and the first section of [demo.go](../internal/demo/demo.go). | Distinguish the Go client, database, collection, and document. |
| 2 | Follow `Contact`, `Create`, and `Get` in [store.go](../contactmongo/store.go). | Find the BSON field names and explain how `_id` reaches Go. |
| 3 | Follow `List`, `UpdateEmail`, and `Delete`. | Explain cursor ownership, `$set`, `$unset`, and a missing document. |
| 4 | Read `EnsureIndexes`, [validation](../contactmongo/input.go), and the tests. | Explain why concurrent creates need a server-side unique index. |

## From Go structs to BSON

BSON is MongoDB's binary document format. It has types beyond ordinary JSON,
including ObjectID. The `bson` struct tags control stored field names:

```go
type Contact struct {
    ID    bson.ObjectID `bson:"_id"`
    Name  string        `bson:"name"`
    Email *string       `bson:"email,omitempty"`
}
```

`bson.D` represents an ordered BSON document as key/value elements. For example,
`bson.D{{Key: "_id", Value: id}}` is the filter `{_id: id}`. In an index or sort
specification, the value `1` means ascending order; in a filter it is an actual
value to match. Those documents serve different purposes even with the same Go type.

`Create` generates an ObjectID before insertion. `ID.Hex()` formats it for
display; `bson.ObjectIDFromHex` parses a caller's hexadecimal ID and can fail.
An ObjectID is neither a sequential SQL integer nor a guaranteed global creation
order. The list method explicitly sorts by `_id` for a stable order of stored IDs.

| Go/stored state | BSON document shape | Decoded `Email` |
| --- | --- | --- |
| nil pointer with `omitempty` | `email` absent | nil |
| Pointer to `""` | `email: ""` | Pointer to empty string |
| Pointer to other text | `email: "ana@example.com"` | Pointer to that text |
| Explicit BSON null from another writer | `email: null` | nil |

Missing and null are distinct document states. This small `*string` model merges
them when decoding; inspect `bson.Raw` or use a presence-aware model when the
distinction matters. The [BSON unit test](../contactmongo/bson_test.go) shows both.
`UpdateEmail(nil)` deliberately removes the field with `$unset`; it does not
store BSON null. A nonnil pointer uses `$set`, including an empty string.

## Follow one operation

`Create(Input{Name: " Ana "})` follows this path:

1. Trim and validate the name in Go.
2. Build a typed `Contact` with a new ObjectID.
3. Encode the struct as BSON and call `InsertOne` with the caller's context.
4. Let MongoDB enforce the unique name index.
5. Return the created Go value or a wrapped error.

For valid input, follow the boundary between ordinary Go data and stored BSON:

```mermaid
sequenceDiagram
    participant A as Application
    participant S as contactmongo.Store
    participant M as MongoDB
    A->>S: Create with Input
    S->>S: Normalize, validate, assign ObjectID
    S->>M: InsertOne with BSON and caller context
    alt Insert acknowledged
        M-->>S: Success after checking unique index
        S-->>A: Created Go Contact
    else Name already exists
        M-->>S: Duplicate-key error
        S-->>A: ErrDuplicate with driver cause
    end
```

The return value is a Go copy. Later edits to it require a new database write
to change storage. Input, network, and context failures retain their own errors.

Strings are values in a typed document/filter; they are never concatenated into
a query language or accepted as caller-supplied BSON operators. Names are unique
after whitespace trimming, with case-sensitive matching under this lab's default
collection collation. Text is limited to 80 Unicode runes for names and 254 for
optional email; email is teaching text, not a complete address validator.

`FindOne(...).Decode(&contact)` reports missing IDs through `mongo.ErrNoDocuments`.
The store also exposes `ErrNotFound`, while preserving the driver cause for
`errors.Is`. Duplicate-key errors expose `ErrDuplicate` and retain their MongoDB
cause for `errors.As`/`mongo.IsDuplicateKeyError`.

`List` accepts limits 1–100, closes the cursor, and checks iteration errors after
`Next` finishes. Each decoded result owns its optional value; changing a returned
Go value does not update the stored document. `FindOneAndUpdate` uses
`ReturnDocument(options.After)` to return the changed document in one operation.
`DeleteOne` checks `DeletedCount` to distinguish removal from absence.

## Client, context, and schema ownership

[Open](../contactmongo/database.go) creates one reusable, goroutine-safe client
with a four-connection maximum **per server**, then calls Ping to verify reachability
and credentials. This lab-sized pool is a teaching choice. Connection creation
alone does not prove that a server accepted authentication.

The demo owns a 20-second operation budget and passes it through every database
call. It disconnects clients after their users finish. Document/cursor/client
cleanup gets a fresh, bounded context so an expired operation does not prevent
cleanup. A canceled or timed-out write can still have reached the server;
cancellation is not a general rollback guarantee.

MongoDB automatically creates collections when needed, but flexible documents
still require application rules. `EnsureIndexes` creates/reuses a named unique
index; it preserves existing documents and fails if they prevent index creation.
Index bootstrap is separate from a versioned data/schema migration strategy.

## Compare with PostgreSQL fundamentals

| PostgreSQL lesson | MongoDB lesson |
| --- | --- |
| `sql.DB` connection pool | Reusable `mongo.Client` with per-server pools |
| Table / row | Collection / BSON document |
| Integer primary key | ObjectID `_id` |
| SQL text with parameter values | Typed BSON filters, documents, and update operators |
| `Rows.Next`, `Scan`, `Rows.Err`, `Close` | `Cursor.Next`, `Decode`, `Cursor.Err`, `Close` |
| SQL NULL versus empty text | Missing field / BSON null / empty text |
| Unique constraint | Unique index |

The [SQL fundamentals](../../02-database-sql/01-fundamentals/README.md) provide
the comparison. This lesson's small contact model keeps persistence concepts
readable; later document modeling can introduce embedding, references, and queries.

### Repeating an operation and idempotency

Repeated `$set` of the same value or `$unset` of an absent field preserves that
field's resulting state. Repeated deletion returns `ErrNotFound` after the first
success. A repeated create with the same normalized name produces a uniqueness
conflict. These behaviors do not provide request-key replay or recovery of an
original creation response. The earlier PostgreSQL idempotency lesson teaches
that separate contract; it is not implied by a unique index.

## Ordinary tests and static checks

These commands need Go; a running MongoDB/container engine is unnecessary:

```sh
go -C extensions/03-mongodb test -count=1 -timeout=60s ./...
go -C extensions/03-mongodb vet ./...
go -C extensions/03-mongodb mod tidy -diff
```

Unit tests cover validation boundaries, BSON presence/round trips, and preserved
error causes. For the race detector, have a C compiler available:

```sh
CGO_ENABLED=1 go -C extensions/03-mongodb test -race -count=1 -timeout=90s ./...
```

## Integration tests with Testcontainers

Start the container engine. Testcontainers creates its own authenticated MongoDB
container, waits for readiness, and registers cleanup. Tests do not read
`MONGODB_URI` or connect to Compose. Each case uses an isolated test namespace;
the finite demo uses its fixed database only within the owned disposable server.

For Rancher Desktop on macOS, select **dockerd (moby)**. With its home-directory
socket and ports forwarded to localhost, configure the test terminal:

```sh
export DOCKER_HOST="unix://$HOME/.rd/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1
```

Docker Desktop/local Docker usually uses automatic detection. A Rancher VM
networking setup may need a reachable VM address instead; inspect it with
`rdctl shell ip a show vznat`. Never set an empty host override. Then run:

```sh
docker info
CGO_ENABLED=1 go -C extensions/03-mongodb test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/03-mongodb vet -tags=integration ./...
```

Integration cases cover real CRUD/BSON storage, cursor limits/order, repeated
index setup and field updates, twelve competing creates, cancellation/deadlines,
authentication failure, independent clients/results, and finite-demo cleanup
that preserves other documents. Every test scenario has a comment explaining
its purpose. A missing engine fails tagged tests instead of silently skipping.
The first run can download MongoDB and Testcontainers' cleanup-helper images.

Successful test packages print `ok`; `[no test files]` for the command/demo
packages is expected because the integration suite exercises them separately.
Successful vet and module metadata checks normally print nothing.

## Inspect or stop the manual database

The image includes mongosh, so a host-side shell installation is optional:

```sh
docker compose -f extensions/03-mongodb/compose.yaml exec mongodb \
  mongosh --quiet --username mongo_lab --password mongo_lab_local \
  --authenticationDatabase mongo_lab mongo_lab \
  --eval 'db.contacts.getIndexes()'
```

After the Go processes finish:

```sh
docker compose -f extensions/03-mongodb/compose.yaml down
```

This retains the MongoDB volume for another session. Use `down -v` only when
intentionally discarding this MongoDB lab's data and users. Testcontainers owns
its separate disposable containers; stopping Compose is unrelated to their cleanup.

Continue to [document modeling and queries](../02-document-modeling/README.md)
for embedded addresses/tags, filtered summaries, and updates to nested fields.

References: [official Go driver](https://www.mongodb.com/docs/drivers/go/current/),
[BSON](https://www.mongodb.com/docs/drivers/go/current/data-formats/bson/),
[context](https://www.mongodb.com/docs/drivers/go/current/context/),
[unique indexes](https://www.mongodb.com/docs/manual/core/index-unique/),
[MongoDB image](https://hub.docker.com/_/mongo), and
[Testcontainers MongoDB module](https://golang.testcontainers.org/modules/mongodb/).
