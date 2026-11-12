# References, aggregation, and explain

Keep companies as independent documents, reference them from profiles, and build
typed reports through MongoDB pipelines. Then inspect how an index changes a
query's access path. The [fundamentals](../01-fundamentals/README.md) and
[document-modeling lesson](../02-document-modeling/README.md) provide background;
all execution and test steps are included here for direct use.

## Requirements and execution

Use Go 1.27 on PATH, a full checkout, and a running Docker-compatible engine
with Docker CLI and Compose v2. Run bash/zsh from the repository root containing
`go.mod`, `chapters`, and `extensions`. First runs download pinned Go modules
and MongoDB 8.0.38. Race checks additionally need clang or gcc.

```sh
go version
docker info
docker compose version
docker compose -f extensions/03-mongodb/compose.yaml up -d --wait
export MONGODB_URI='mongodb://mongo_lab:mongo_lab_local@127.0.0.1:27018/mongo_lab?authSource=mongo_lab'
go -C extensions/03-mongodb run ./04-references-and-aggregation
```

Compose exposes real MongoDB at `127.0.0.1:27018` with a retained named volume.
The official Go driver is pinned at v2.10.0. This local teaching setup uses the
database-scoped `mongo_lab` readWrite account over loopback without transport TLS.
`authSource=mongo_lab` selects where that account authenticates; code explicitly
selects the database and collections.

This stage needs only `MONGODB_URI`. The schema account/setup from stage 3 is not
required: ordinary reads, writes, indexes, and explain use the data account.
The demo creates two random `report_demo_*` collections in `mongo_lab`, runs
five sections, and drops only those owned collections before disconnecting.
Other data remains. Operations share a 30-second context; cleanup has fresh
bounded time even after cancellation.

Look for:

```text
Directory keeps five profiles; missing targets do not remove their source rows.
One company rename appears in two profiles; deleting another company does not cascade.
City Guadalajara -> 3 profiles
Tag backend -> 3 profiles
Tag go -> 3 profiles
Empty tags add no tag rows; a profile can contribute to several tag groups.
```

The final section prints returned/examined counters for three access paths and
the automatic winning plan. Its exact tree/counters depend on the server/data;
there is no elapsed-time performance assertion.

## Learn in passes

| Pass | Follow | Check your understanding |
| --- | --- | --- |
| 1 | Run the demo and read `Company`/`Profile` in [store.go](../reporting/store.go). | Distinguish a stored ObjectID reference from a Go pointer or embedded company. |
| 2 | Follow `Directory` in [reports.go](../reporting/reports.go). | Explain the lookup array, preserving empty matches, and the typed read model. |
| 3 | Follow `CityTotals` and `TagTotals` stage by stage. | Explain grouping before limit and avoiding duplicate tag counts. |
| 4 | Read [explain.go](../reporting/explain.go) and the commented tests. | Compare hinted paths with the unhinted optimizer choice without assuming universal speedups. |

## References and document lifetime

The two collections have these shapes:

```javascript
// Company: shared, independently maintained state.
{ _id: ObjectId("..."), name: "Atlas" }

// Profile: city is a simple field in this separate reporting model.
{
  _id: ObjectId("..."), name: "Ana", city: "Guadalajara",
  tags: ["go", "backend"], company_id: ObjectId("...")
}
```

Two profiles can reference one company's ID. Renaming the company changes what
later lookups display, without copying the new name into each profile. Company
deletion leaves references behind: MongoDB does not automatically enforce a
foreign key or cascade deletes. This example deliberately permits dangling IDs
to make their read behavior visible. A production application must choose its
reference lifecycle and consistency policy; an existence check alone cannot
prevent a later concurrent deletion.

Embed data that belongs to its parent and is naturally read/updated together,
as the address in stage 2. Reference shared state with its own lifecycle, as this
company. References introduce another collection/query cost; they are not always
better than embedding and do not create a cross-query snapshot automatically.

`CompanyID *bson.ObjectID` represents optional presence in Go. BSON stores the
ObjectID value, not a memory address. Nil with `omitempty` omits the reference;
an explicit zero ID is rejected by the write API. Profile creation copies the
reference value and tags so the returned value does not alias caller-owned input.

Names/cities are trimmed, valid UTF-8 text of 1–80 runes without NUL. Tags have
at most eight input entries of 1–24 runes, are lowercased/deduplicated, and encode
as `[]` when absent. Imported data must follow the intended string/array model;
the earlier schema lesson shows how to add database-enforced rules. Companies
created here always have ObjectID identities.

The stored relationship is an ObjectID value shared by two references:

```mermaid
flowchart LR
    subgraph Profiles[Profiles collection]
        A[Ana: company_id is C1]
        B[Bea: company_id is C1]
    end
    subgraph Companies[Companies collection]
        C[Company with _id C1 and name Atlas]
    end
    A -.->|Match company_id to _id| C
    B -.->|Match company_id to _id| C
```

C1 stands for an ObjectID, not a Go memory address. The dashed edges describe
lookup matches; they do not enforce existence or cascade behavior.

## Follow the directory pipeline

| Stage | Purpose |
| --- | --- |
| `$match` | Optionally select an exact city; blank city means all profiles. |
| `$sort` | Order by name, then `_id` to break equal-name ties. |
| `$limit` | Bound source directory rows to 1–100. |
| `$lookup` | Match `company_id` to `_id` in the other collection, producing a company array. |
| `$unwind` | Turn the zero/one company array into a field, preserving empty matches. |
| `$project` | Return only the directory's ID/name/city/reference/company fields. |

The equality join uses the foreign `_id` index and produces at most one company
for each valid reference. Both collections use the same database. Keeping empty
matches makes this a left join instead of removing profiles without targets.

| Source state | `DirectoryEntry.CompanyID` | `DirectoryEntry.Company` |
| --- | --- | --- |
| Valid reference | ID pointer | Company pointer |
| Unassigned | nil | nil |
| Dangling reference | ID pointer | nil |

This distinction assumes the lesson's ObjectID-only company model. MongoDB treats
a missing local lookup field like null, so the foreign identity model matters.
The directory's pointers own decoded values; editing a returned company does
not update storage or another row's decoded company.

Limit precedes lookup because each foreign `_id` yields at most one match and
there is no post-join filter. Moving a company-name filter after that limit would
change the meaning of the query. Pipeline order is part of the report contract.

Follow each stage's output into the next. The empty-match path still produces
a directory row, with a nil Company in the Go read model:

```mermaid
flowchart TD
    P[Profiles] --> M["$match: optional exact city"]
    M --> S["$sort: name, then _id"]
    S --> L["$limit: source profiles"]
    L --> K["$lookup: company_id to company _id"]
    K --> U["$unwind: preserve empty company matches"]
    U --> R["$project: directory fields"]
    R --> G[Go DirectoryEntry values]
```

This limit bounds source directory rows before the join. The grouping reports
below intentionally place their limit after counting.

## Grouping into typed reports

An aggregation pipeline passes each stage's output into the next stage. `bson.D`
describes an ordered stage document, while `mongo.Pipeline` is the ordered slice
of stages. Field expressions such as `"$city"` read a field value; ordinary input
filter strings remain literal values supplied through the typed API.

`CityTotals` uses this progression:

```javascript
{ $group: { _id: "$city", profiles: { $sum: 1 } } }
{ $project: { _id: 0, city: "$_id", profiles: 1 } }
{ $sort: { profiles: -1, city: 1 } }
{ $limit: 10 }
```

The original city becomes a group key; projection names the result's `city`
field. Sorting uses descending count and ascending city for deterministic ties.
The code decodes results into `[]CityCount` with int64 counts. Group before limit
so a top-one report still counts the complete source, rather than its first row.

`TagTotals` optionally matches a city first, then:

```javascript
{ $project: { tags: { $setUnion: ["$tags", []] } } }
{ $unwind: "$tags" }
{ $group: { _id: "$tags", profiles: { $sum: 1 } } }
{ $project: { _id: 0, tag: "$_id", profiles: 1 } }
{ $sort: { profiles: -1, tag: 1 } }
{ $limit: 10 }
```

Set-union deduplicates each profile's array, including duplicate entries supplied
outside the Go writer. Unwind emits one row per remaining tag. Empty tag arrays
emit no tag row. The subsequent grouping counts distinct source profiles per
tag. A profile with two tags contributes to two groups, so adding tag counts
does not necessarily produce the number of profiles. Report ordering is explicit;
the order of set-union's intermediate array does not matter.

Results decode into `[]TagCount`. A small generic helper owns cursor iteration,
decoding, error checks, and fresh-context cleanup for all three typed read models.

In TagTotals, deduplication happens inside each profile before counting across
profiles. Both report pipelines group before applying their final row limit:

```mermaid
flowchart TD
    P[Profiles] --> M["$match: optional city"]
    M --> D["$project with $setUnion: distinct tags per profile"]
    D --> U["$unwind: one row per tag"]
    U --> G["$group: count profiles for each tag"]
    G --> R["$project: tag and profile count"]
    R --> S["$sort: count descending, then tag"]
    S --> L["$limit: returned groups"]
    L --> T[Go TagCount values]
```

For example, one profile with tags `go, go, backend` contributes one count to
go and one to backend. An empty array contributes no tag rows.

Empty reports are nonnil empty slices. Returned row limits do not bound all
upstream scans, grouping, sort memory, or join work; contexts bound operation time.
The report pipelines perform reads and do not write materialized results.

## Read explain without turning it into a benchmark

`ExplainCity` runs the same local find/filter/sort three times using
`verbosity: "executionStats"`:

1. Natural scan forced by `hint: { $natural: 1 }`.
2. The compound `{ city: 1, name: 1, _id: 1 }` index forced by name.
3. No hint, letting the optimizer choose.

It explains the directory's local selection/order, not the complete lookup or
grouping pipelines. `EnsureIndexes` must run first. The Go result retains:

| Value | Meaning |
| --- | --- |
| Returned | Matching documents produced by that find command |
| DocsExamined | Document examinations while executing the query |
| KeysExamined | Index-key examinations |
| WinningPlan | Raw BSON plan for inspection; its tree shape can vary |

The forced paths illustrate access methods; they do not prove the optimizer
should always use the index. The small demo does not measure representative
latency, and the three commands are not one transactional snapshot. Use stable
data when comparing them. Execution statistics run the query work, and explain
does not use/populate the normal plan cache. Tests assert equal returned counts
and scan behavior on a known fixture, not a time threshold or a fixed plan tree.

## Ordinary tests and static checks

These compile all four stages and run unit tests without a database:

```sh
go -C extensions/03-mongodb test -count=1 -timeout=60s ./...
go -C extensions/03-mongodb vet ./...
go -C extensions/03-mongodb mod tidy -diff
```

Unit tests cover input boundaries, optional reference/slice ownership, validation
before database access, and preserved driver/context errors. With a C compiler:

```sh
CGO_ENABLED=1 go -C extensions/03-mongodb test -race -count=1 -timeout=90s ./...
```

## Integration tests with Testcontainers

Start the engine. Testcontainers owns authenticated MongoDB instances, readiness,
and cleanup; tests never read `MONGODB_URI`/`MONGODB_SCHEMA_URI` or use Compose.
The new suite gives every case an isolated namespace in its disposable server.

For Rancher Desktop on macOS, select **dockerd (moby)**. With the home-directory
socket and ports forwarded to localhost, configure the same test terminal:

```sh
export DOCKER_HOST="unix://$HOME/.rd/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1
```

Docker Desktop/local Docker usually uses automatic detection. A Rancher VM
networking setup may need its reachable VM address instead; inspect it with
`rdctl shell ip a show vznat`. Keep the override nonempty. With clang or gcc:

```sh
docker info
CGO_ENABLED=1 go -C extensions/03-mongodb test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/03-mongodb vet -tags=integration ./...
```

The command tests all four stages, including references, left lookup, shared
company changes, missing targets, duplicate tag handling, empty arrays, grouped
limits/ties, literal criteria, explain counters, cancellation, and repeated demo
cleanup preserving other data. Every test scenario has a purpose comment.
CI includes these cases in its existing MongoDB job. Tagged tests fail when
the engine is missing; initial runs can download server/cleanup-helper images.

Expect `ok` for tested packages. `[no test files]` is normal for command/helper
packages exercised by integration tests. Successful vet/metadata checks are quiet.

## Stop the manual database

After the Go program finishes:

```sh
docker compose -f extensions/03-mongodb/compose.yaml down
```

The named volume remains. Use `down -v` only when deliberately discarding all
MongoDB lab data/accounts. Testcontainers manages its separate test containers.

Read code in order: [main.go](main.go), [demo.go](../internal/reportdemo/demo.go),
[store.go](../reporting/store.go), [reports.go](../reporting/reports.go),
[explain.go](../reporting/explain.go), [unit tests](../reporting/input_test.go),
and [integration tests](../integration/reporting_test.go).

References: [lookup](https://www.mongodb.com/docs/v8.0/reference/operator/aggregation/lookup/),
[unwind](https://www.mongodb.com/docs/v8.0/reference/operator/aggregation/unwind/),
[set-union](https://www.mongodb.com/docs/v8.0/reference/expression/setunion/),
[explain](https://www.mongodb.com/docs/v8.0/reference/command/explain/), and
[aggregation with Go](https://www.mongodb.com/docs/drivers/go/current/aggregation/).
