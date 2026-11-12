# Embedded documents and queries

Model a profile with its own address and a small array of tags. Query the child
fields, return a projected summary, and update one nested field without replacing
its neighbors. This is stage 2 of the MongoDB lab; the
[fundamentals](../01-fundamentals/README.md) explain clients, BSON, and basic CRUD.
All runtime and test setup is included here for direct use.

## Requirements and execution

Use Go 1.27 on PATH, a full repository checkout, and a running Docker-compatible
engine with Docker CLI and Compose v2. Run bash/zsh from the repository root
containing `go.mod`, `chapters`, and `extensions`. First runs can download pinned
Go dependencies and container images. Race checks also need clang or gcc.

```sh
go version
docker info
docker compose version
docker compose -f extensions/03-mongodb/compose.yaml up -d --wait
export MONGODB_URI='mongodb://mongo_lab:mongo_lab_local@127.0.0.1:27018/mongo_lab?authSource=mongo_lab'
go -C extensions/03-mongodb run ./02-document-modeling
```

Compose starts real MongoDB 8.0.38 at `127.0.0.1:27018`, retains a named volume,
and creates the local application user with read/write access to `mongo_lab`.
The Go module pins the official driver v2.10.0. `authSource=mongo_lab` selects the
authentication database; the code explicitly selects `mongo_lab.contact_profiles`.
This loopback teaching setup uses local credentials without transport TLS.
Initialization scripts run only on a new empty volume; editing Compose
credentials later does not update users already stored in that volume.

The finite program prepares indexes, runs five sections, and exits. Look for:

```text
Stored three profiles; the Go query tag for this run is "go-<random suffix>".
City + tag filter -> one projected summary (limit=1), ordered by name and ID.
Tag filter -> two summaries; Get still returns the full profile with tags.
Updated address.city to Puebla; street, name, and tags stayed intact.
The original Guadalajara + tag query now matches only Beto.
Canceled search -> context.Canceled; missing update -> ErrNotFound.
Deleted only the three demo profiles; existing data is retained.
```

Every run uses its own tag to keep query counts independent of other profiles.
It tracks inserted IDs and attempts to remove only those documents, including
after later errors. Cleanup uses fresh bounded time before client shutdown.
Collections and indexes remain, and data stored by other callers is retained.

## Learn in passes

| Pass | Follow | Check your understanding |
| --- | --- | --- |
| 1 | Run the demo and read `Address`, `Profile`, and `Input` in [store.go](../contactdoc/store.go). | Find the embedded document and the array in stored BSON. |
| 2 | Follow `Search` with both a city and a tag. | Explain dot notation, array membership, AND, sort, limit, and projection. |
| 3 | Compare `Summary` with `Profile`, then follow `UpdateCity`. | Explain why a summary is a read model and why changing one child preserves its neighbors. |
| 4 | Read [normalization](../contactdoc/input.go) and the commented tests. | Explain slice ownership, bounded embedding, and the concurrent child-field test. |

## The document and its Go model

One stored profile has this shape:

```javascript
{
  _id: ObjectId("..."),
  name: "Ana",
  address: { street: "Rio 1", city: "Guadalajara" },
  tags: ["go", "backend"]
}
```

`Address` is a nested Go struct with BSON tags; it becomes an embedded document.
`Tags` is a `[]string`; it becomes a BSON array. `normalize` trims required text,
counts Unicode runes, lowercases/deduplicates tags, and copies the input slice.
It always produces a nonnil tags slice. An input with no tags is encoded as `[]`.

The related fields live inside the same stored document:

```mermaid
flowchart TD
    subgraph Document[One profile BSON document]
        P[Profile with its own _id and name]
        P --> A[Embedded address]
        A --> S[street]
        A --> C[city]
        P --> T[Embedded tags array, at most 8 inputs]
    end
    U[UpdateCity] -->|set address.city only| C
```

Inside the box, arrows mean field containment, not extra queries or Go pointers.
The city update targets one child field in that document and preserves street.

| Field | Teaching rule |
| --- | --- |
| Name / city | Required trimmed text, at most 80 Unicode runes |
| Street | Required trimmed text, at most 120 Unicode runes |
| Tags | At most 8 input entries, each 1–24 runes after trimming; lowercase and deduplicated in first-seen order |
| Text | Valid UTF-8, without embedded NUL |
| Identity | Nonzero ObjectID; different profiles may have the same name |

Cities and names retain case. Query tags use the same lowercase rule as stored
tags; this is not locale-aware or accent-insensitive searching. Validation applies
to these Go entry points. External database writers can create different shapes;
the [schema-evolution lesson](../03-schema-evolution/README.md) introduces
server-side validation and a controlled rollout for existing documents.

Copying `Address` copies its string fields. Copying `Profile` still shares the
Tags slice's backing array. `Create` therefore returns tags independent of the
caller's input, and each decoded result owns its slice. Use `slices.Clone` if
you copy a returned Profile and want to edit its tags independently.

### When to embed and when to reference

| Relationship | Suitable starting model | Why |
| --- | --- | --- |
| A profile owns one address, usually read together | Embed `Address` | One document contains the small related state and supports atomic child updates. |
| A profile has a small bounded set of labels | Embed a tags array | The values are simple and can be matched by array membership. |
| Many profiles share a company with its own lifecycle | Reference a company document by ID | The shared company can be updated independently of each profile. |
| An ever-growing history | Separate documents referencing the profile | The parent document does not accumulate an unbounded array. |

The executable demonstrates embedding. References are a modeling decision,
not a Go pointer or an automatically enforced foreign key. The
[references/reporting stage](../04-references-and-aggregation/README.md)
demonstrates dereferencing, missing targets, and cross-collection lookup.

## Follow the query

The store accepts `Query{City: " Guadalajara ", Tag: " GO ", Limit: 2}`.
After normalization, it sends this BSON filter:

```javascript
{ "address.city": "Guadalajara", tags: "go" }
```

Both criteria must match. Dot notation selects one nested field; scalar equality
on `tags` matches a value inside the array. Optional blank city/tag criteria mean
no filter for that field. The caller supplies values in this typed query, while
field names and operators are fixed by the Go code.

The options add:

```javascript
sort:       { name: 1, _id: 1 }
limit:      2
projection: { name: 1, address: 1 }
```

`1` means ascending in the sort specification and included in the projection.
An inclusion projection also retains `_id` by default. Unlisted fields such as
tags do not travel back in the document. The result decodes into `Summary`, a
separate read model containing ID, name, and the complete address. `Get` still
returns a full `Profile`. Treating a partial read as a full replacement could
erase data the caller never loaded.

Names may repeat, so `_id` breaks ties deterministically within the result set.
The required limit is 1–100; no matches produce an empty nonnil slice. A limit
bounds returned data, not the number of documents MongoDB may inspect or sort.
The store closes its cursor and checks iteration errors. Request contexts bound
operation time; the demo gives its database operations one 20-second budget.

`EnsureIndexes` creates a compound city/name/ID index and a tags index. An index
on an array is called a multikey index. Query plans vary with criteria and data;
these examples do not claim every filter/sort is index-covered. Inspecting plans
and measuring workloads can follow with more advanced queries.

## Update one child field

`UpdateCity(id, "Puebla")` uses:

```javascript
{ $set: { "address.city": "Puebla" } }
```

This changes city while preserving street, other address fields, name, and tags.
Compare it with setting the whole embedded address:

```javascript
{ $set: { address: { city: "Puebla" } } }
```

The second form replaces the address value and removes its other fields. Choose
the operation that matches the intended editing contract. The lab's city method
uses dot notation and returns the resulting document with
`FindOneAndUpdate` and `ReturnDocument(options.After)`.

A single-document update is atomic; it does not create a transaction across
other documents. Concurrent writes to city and street can both survive because
they target different fields. Writers targeting the same field still need an
explicit conflict policy if overwriting is unacceptable. Cancellation/timeouts
are not general guarantees that a write was rolled back. Repeated city assignment
preserves that field's state but does not provide request-key replay.

## Ordinary tests and static checks

These commands compile both stages and run unit tests without a database:

```sh
go -C extensions/03-mongodb test -count=1 -timeout=60s ./...
go -C extensions/03-mongodb vet ./...
go -C extensions/03-mongodb mod tidy -diff
```

With a C compiler available, run ordinary tests with the race detector:

```sh
CGO_ENABLED=1 go -C extensions/03-mongodb test -race -count=1 -timeout=90s ./...
```

Unit tests cover nested BSON/array round trips, Unicode boundaries, tag
normalization and slice ownership, query validation, IDs, and error causes.

## Integration tests with Testcontainers

Have the engine running. Tests create their own authenticated MongoDB servers,
wait for readiness, and register cleanup. They never read `MONGODB_URI` or use
Compose. Each test case has an isolated namespace; demo tests use `mongo_lab`
only inside their disposable server. Tagged tests fail if the runtime is missing.

For Rancher Desktop on macOS, select **dockerd (moby)**. With its home-directory
socket and ports forwarded to localhost, configure the test terminal:

```sh
export DOCKER_HOST="unix://$HOME/.rd/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1
```

Docker Desktop/local Docker usually uses automatic detection. If Rancher exposes
ports through a VM, use that reachable address instead; inspect the VZ interface
with `rdctl shell ip a show vznat`. Keep the override nonempty. With clang or gcc:

```sh
docker info
CGO_ENABLED=1 go -C extensions/03-mongodb test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/03-mongodb vet -tags=integration ./...
```

The command runs both MongoDB stages. The
[new integration cases](../integration/modeling_test.go) verify stored BSON,
city/tag AND filters, scalar array matching, projection results, sort ties,
limits, literal values, missing profiles, cancellation/deadlines, and repeated
city assignments. They also check concurrent city/street writes, preservation
of an unknown sibling field, independent result slices, and two demo runs that
preserve an existing profile and a fundamentals contact. CI runs these commands
through the existing MongoDB jobs.

Expect `ok` for tested packages; `[no test files]` is normal for commands and
demo packages exercised by the integration suite. Successful vet/metadata checks
normally print nothing. First runs can pull the server and cleanup-helper images.

## Inspect and stop the manual database

Inspect the collection's indexes using mongosh supplied by the image:

```sh
docker compose -f extensions/03-mongodb/compose.yaml exec mongodb \
  mongosh --quiet --username mongo_lab --password mongo_lab_local \
  --authenticationDatabase mongo_lab mongo_lab \
  --eval 'db.contact_profiles.getIndexes()'
```

After Go processes finish, stop the manual database:

```sh
docker compose -f extensions/03-mongodb/compose.yaml down
```

The volume remains for another session. Use `down -v` only when intentionally
discarding this MongoDB lab's data and users, including both stages' collections.
Testcontainers manages its separate disposable containers.

Read the code in order: [main.go](main.go), [demo.go](../internal/modelingdemo/demo.go),
[store.go](../contactdoc/store.go), [input.go](../contactdoc/input.go),
[unit tests](../contactdoc/input_test.go), [BSON tests](../contactdoc/bson_test.go),
and [integration tests](../integration/modeling_test.go).

References: [document modeling](https://www.mongodb.com/docs/manual/data-modeling/best-practices/),
[dot notation](https://www.mongodb.com/docs/manual/core/document/#dot-notation),
[query filters](https://www.mongodb.com/docs/drivers/go/current/crud/query/query-document/),
and [projections](https://www.mongodb.com/docs/drivers/go/current/crud/query/project/).
