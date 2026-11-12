# Cache-aside with PostgreSQL and Valkey

Run this lesson after [Valkey fundamentals](../01-fundamentals/README.md) and
basic SQL/context/testing. It is a finite console program using **two real
services**. PostgreSQL owns contacts; Valkey keeps temporary copies for reads by
ID. There is no HTTP or gRPC server in this stage.

## What runs where

| Component | Responsibility |
| --- | --- |
| `02-cache-aside/main.go` | Read configuration and bound the demo's lifetime. |
| `internal/asidedemo` | Open/close resources, create isolated examples, show routes, and count contact reads. |
| `contactcache` | Decide whether a contact comes from cache or the authoritative source. |
| Existing SQL `contactpg` adapter | Validate contacts, run generated SQL, and commit updates/deletes. |
| PostgreSQL 18.6 | Store authoritative records in the existing `sql_service_contacts` table. |
| Valkey 9.0.7 | Store versioned JSON strings with TTL; it never queries PostgreSQL itself. |

The module reuses the course's complete contact model: ID, name, email, and
phone. The two-field contact from fundamentals remains a smaller independent
serialization example. Both stages use remote commands, with valkey-go's
client-side caching disabled.

## Learn in passes

1. Follow `contactcache.Get`: GET Valkey, load a miss from PostgreSQL, SET/PX.
2. Observe the cold and warm read counters. A warm hit skips the source's Get.
3. Follow Replace/Patch/Delete: SQL succeeds first, then DEL invalidates the copy.
4. Observe expiration: the key disappears, but the database row remains.
5. Read the error/consistency limits before using this version in a service.

```mermaid
flowchart TD
    A[Get contact by ID] --> B{Valkey GET}
    B -->|HIT| C[Return cached contact]
    B -->|MISS| D[Read PostgreSQL]
    D -->|Found| E[SET JSON with TTL]
    E --> F[Return contact]
    D -->|Absent| G[Return ErrNotFound]
```

| Operation | PostgreSQL | Valkey |
| --- | --- | --- |
| Create | Insert and return the assigned ID. | Leave empty; the first read populates it. |
| Get with HIT | No contact Get call. | Decode and return the full contact; TTL is not renewed. |
| Get with MISS | Read current contact. | Cache a found contact with TTL. |
| Replace (PUT semantics) | Replace every editable field; omitted optional fields become empty. | DEL after successful SQL completion. |
| Patch (PATCH semantics) | Merge selected fields while holding the SQL row lock. | DEL after successful SQL completion. |
| Delete | Delete and commit. | DEL after successful SQL completion. |

The application calls both services. A cache miss is a normal absent-key reply;
it is different from an unavailable cache or invalid JSON. Missing contacts are
not cached, so repeated missing-ID reads still reach PostgreSQL. IDs are never
reused. Collection queries remain outside this lesson's cache.

### Writes commit before invalidation

Replace, Patch, and Delete use this order. The source adapter owns the SQL
transaction; the cache layer calls DEL only after that adapter reports success:

```mermaid
sequenceDiagram
    participant C as Caller
    participant A as contactcache.Store
    participant P as PostgreSQL via contactpg
    participant V as Valkey
    C->>A: Replace, Patch, or Delete
    A->>P: Write through the source adapter
    alt Source confirms success
        P-->>A: Write committed
        A->>V: DEL contact key
        alt DEL succeeds, including an absent key
            V-->>A: Key absent
            A-->>C: Success
        else DEL fails
            V-->>A: Cache error
            A-->>C: InvalidationError, source already committed
        end
    else Source returns an error
        P-->>A: Source error, success not confirmed
        A-->>C: Source error, no DEL attempted
    end
```

Create writes only to PostgreSQL and leaves cache population to the next Get.
The sequence does not make the two services one transaction; read the
[error and consistency limits](#errors-and-consistency-limits) below.

### Expiration removes the copy, not the row

This is the successful path for the demo's short-lived contact:

```mermaid
sequenceDiagram
    participant C as Caller
    participant A as contactcache.Store
    participant V as Valkey
    participant P as PostgreSQL
    Note over V,P: Cached copy exists, source row remains durable
    V->>V: TTL elapses and key becomes absent
    C->>A: Get contact
    A->>V: GET contact key
    V-->>A: MISS
    A->>P: Get contact by ID
    P-->>A: Current contact
    A->>V: SET JSON with a fresh TTL
    V-->>A: Stored
    A-->>C: Contact
```

The initial read diagram and these sequences assume successful decoding and
cache fills. Cache failures are propagated in this stage; they do not silently
take the MISS branch.

## Prerequisites and manual services

Use a full checkout, Go 1.27 on PATH, and Docker CLI/Compose v2 with a running
Docker-compatible engine. Run all commands below in bash/zsh **from the repository
root** containing `go.mod`, `chapters`, and `extensions`. Initial runs download
pinned Go dependencies and container images.

```sh
go version
docker info
docker compose version
```

Docker Desktop/local Docker normally discovers its socket automatically. For
Rancher Desktop on macOS, select **dockerd (moby)**. With its home-directory
socket and localhost port forwarding, set:

```sh
export DOCKER_HOST="unix://$HOME/.rd/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1
```

A different Rancher VM networking setup may require a reachable VM address;
inspect `rdctl shell ip a show vznat`. Keep the host override nonempty. The two
`TESTCONTAINERS_` variables configure tagged tests, not the manual demo URLs.

Start the existing PostgreSQL Compose service and the Valkey Compose service:

```sh
docker compose -f extensions/02-database-sql/compose.yaml up -d --wait
docker compose -f extensions/04-valkey-cache/compose.yaml up -d --wait

export DATABASE_URL='postgres://sql_lab:sql_lab_local@127.0.0.1:5433/sql_lab?sslmode=disable'
export VALKEY_URL='valkey://default:cache_lab_local@127.0.0.1:6380/0'

go -C extensions/04-valkey-cache run ./02-cache-aside
```

PostgreSQL listens on `127.0.0.1:5433` and retains its named volume. Valkey listens
on `127.0.0.1:6380`, requires the local teaching password, and disables snapshots
and AOF. This loopback setup uses no transport TLS. If a port is occupied by
another instance, use that service only if its credentials/data are appropriate,
or change the published port and the matching URL.

Startup applies the **existing golang-migrate migrations**, through
`contactpg.Migrate`; there is no second schema or custom migration runner here.
It preserves existing contacts and durable idempotency records. Migration
connection, statement, and lock waits have separate bounded settings because
golang-migrate does not expose a context API. The demo's normal operations use
the caller's context and a 30-second budget.

The program creates two contacts with random names and a random cache namespace.
It deletes those contacts and exact keys on return, using a fresh cleanup context,
then closes its clients/pool. It does not flush Valkey or truncate SQL tables.
There is no long-running `-serve` mode. After a forced process termination, a demo
row can remain; known demo cache entries still expire automatically.

## Expected output and what it proves

The seven sections print routes like these; names/IDs in JSON vary per run:

```text
CREATE -> PostgreSQL; cache empty; PostgreSQL contact reads=0
First GET -> MISS -> PostgreSQL -> fill Valkey; PostgreSQL contact reads=1
Second GET -> HIT -> Valkey only; PostgreSQL contact reads=1
REPLACE -> PostgreSQL committed -> DEL; omitted email cleared
GET after replacement -> MISS -> PostgreSQL -> fill Valkey; PostgreSQL contact reads=2
PATCH -> PostgreSQL committed -> DEL; phone preserved
GET after partial update -> MISS -> PostgreSQL -> fill Valkey; PostgreSQL contact reads=3
GET short-TTL contact -> MISS -> PostgreSQL -> fill Valkey; PostgreSQL contact reads=4
Valkey key expired; PostgreSQL row remains
GET after expiration -> MISS -> PostgreSQL -> fill Valkey; PostgreSQL contact reads=5
DELETE -> PostgreSQL committed -> DEL
GET after delete -> MISS -> PostgreSQL -> ErrNotFound; PostgreSQL contact reads=6
```

The counter records calls to the real SQL adapter's **contact Get**, including
not-found lookups. It excludes migrations, connection PING, writes, cleanup,
and SELECTs inside write transactions. This demonstrates routing, not total SQL
traffic or a performance benchmark. The demo checks values and expected counts;
a routing mistake returns an error instead of merely printing a success message.

Normal entries have a one-minute TTL. A separate example contact uses 120ms to
show actual server expiration, observed by bounded polling rather than a fixed
sleep. A cache hit does not extend TTL. No timing/latency threshold is asserted.

## Keys and complete payloads

Keys look like `course:cache-aside:<namespace>:v1:contact:<id>`. The namespace
identifies one database/data set; two databases can assign the same ID to
different people and must not share a namespace. A service should use a stable,
deployment-specific namespace; this demo uses a random one for isolation.

The versioned JSON includes every field:

```json
{"schema_version":1,"id":42,"name":"Ana","email":"ana@example.com","phone":"555"}
```

Typed int64 decoding preserves IDs above 2^53. Decoding checks format version,
shared contact validation/normalization, and the expected key ID. Empty optional
fields and allowed string bytes round-trip without changing the contact's
content version. Cache payload version and SQL migration version are independent.

## Errors and consistency limits

This baseline deliberately **propagates cache errors**. A failed GET or malformed
payload does not automatically fall back to SQL. A failed cache fill returns an
error even though the source read succeeded. The next failure-policy lesson will
add availability choices and prove them separately.

After successful Replace/Patch/Delete, a failed DEL returns
`*contactcache.InvalidationError`. Use `errors.As` to identify it and `errors.Is`
to inspect its cause. The source has **already confirmed the write**; that error
does not mean rollback. Replace/Patch also return the committed contact value.
Do not blindly retry the whole write. An ordinary source error is propagated
without claiming a successful commit or invalidating the cache.

Invalidation uses the caller's context, so cancellation after SQL success can
prevent DEL and produce the same partial-success error. There is no atomic
transaction spanning PostgreSQL and Valkey. This lesson does not yet retry or
repair failed invalidations.

TTL plus invalidation does **not** guarantee strict consistency. A reader can
load an old SQL value, race with a writer's commit/DEL, and repopulate the cache
with the old value. Writes made directly through another lab also bypass this
layer's invalidation and may remain invisible until expiry. Hits trust the cached
copy; they do not verify the current SQL row. Use sequential runs to study this
baseline, then continue to the concurrency/failure block. Read-after-write tests
here describe sequential successful operations, not a concurrency guarantee.

This contact cache is separate from request idempotency: expiring a contact copy
does not erase a durable creation result. HTTP validators, authorization,
conditional writes, and keyed creation will be covered when integrating the
shared behavior into dedicated REST and gRPC labs.

## Ordinary tests: no services

```sh
go -C extensions/04-valkey-cache test -count=1 -timeout=60s ./...
go -C extensions/04-valkey-cache vet ./...
go -C extensions/04-valkey-cache mod tidy -diff
```

These tests use the existing in-memory contact source and a small cache double
to verify routing, no negative caching, commit-before-invalidation ordering,
rejected writes, errors/cancellation, and complete payload validation. They do
not prove real SQL or TTL behavior. With clang/gcc installed, also run:

```sh
CGO_ENABLED=1 go -C extensions/04-valkey-cache test -race -count=1 -timeout=90s ./...
```

## Integration tests: real PostgreSQL and Valkey

Keep the engine running. You do **not** need to start Compose or export either
service URL. Testcontainers creates authenticated Valkey and PostgreSQL servers,
discovers free mapped ports, applies migrations, and cleans up its own resources.
It never reads `DATABASE_URL` or `VALKEY_URL`. Configure the Rancher variables
above in this terminal if needed.

```sh
docker info
CGO_ENABLED=1 go -C extensions/04-valkey-cache test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/04-valkey-cache vet -tags=integration ./...
```

To focus on this new stage:

```sh
CGO_ENABLED=1 go -C extensions/04-valkey-cache test -tags=integration -race -run '^TestCacheAside$' -count=1 -timeout=5m -v ./integration
```

The real suite proves cold/warm routing by counting the SQL adapter's Get calls,
hits through a second independent client, expiring keys reloading durable rows,
full and partial updates invalidating cached values, deleted/missing rows,
corrupt payloads, cancellation, and repeated demo cleanup preserving neighboring
SQL rows and Valkey keys. The fundamentals suite additionally verifies cache
authentication/type/connection errors and raw TTL states. Missing engines fail
tagged tests. Initial runs can pull both server images and the cleanup helper.

Expect `ok` for packages with tests and `[no test files]` for commands/helpers;
the latter simply have no direct `_test.go` files. Successful vet/tidy checks are
normally quiet. CI runs ordinary/race checks and the real two-service suite.

## Cleanup and next stages

After Go processes finish, stop the manual services:

```sh
docker compose -f extensions/04-valkey-cache/compose.yaml down
docker compose -f extensions/02-database-sql/compose.yaml down
```

PostgreSQL's named volume remains, including other SQL/REST/gRPC lab data. These
commands stop the shared manual services, so finish other labs using them first.
Valkey keys disappear on restart. Testcontainers cleans its separate resources.

Next: concurrency/failure policy, then dedicated REST and gRPC cache integration
labs reusing this transport-independent layer. Guided exercises remain last.

References: [cache-aside pattern and limitations](https://learn.microsoft.com/en-us/azure/architecture/patterns/cache-aside),
[Valkey SET](https://valkey.io/commands/set/),
[Testcontainers PostgreSQL](https://golang.testcontainers.org/modules/postgres/),
and [Testcontainers Valkey](https://golang.testcontainers.org/modules/valkey/).
