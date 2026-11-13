# Cache concurrency and failure policy

This independent console lab extends [basic cache-aside](../02-cache-aside/README.md)
with explicit runtime failure/cancellation policy. PostgreSQL still owns the
records; Valkey stores temporary copies. The strict `NewStore` and its stage-2
demo retain their original behavior. This stage uses `NewResilientStore`.

## Learn in passes

1. Follow GET failures and corrupt payloads into PostgreSQL; inspect diagnostics.
2. Follow `singleflight.DoChan`: several callers share a lookup for the same ID.
3. Cancel one caller while others wait; distinguish caller and shared-load contexts.
4. Follow a local reader/fill and writer through the same cancelable gate.
5. Observe a failed post-commit DEL, its stale hit, and targeted repair.
6. Read the two-instance integration case before assuming distributed consistency.

| Piece | Responsibility |
| --- | --- |
| `contactcache/resilient.go` | Bounded cache calls, SQL fallback, shared reads, context ownership, and shutdown. |
| `contactcache/resilient_writes.go` | Source writes, local ordering, post-commit invalidation, and targeted repair. |
| `internal/cachecontrol` | Explicit demonstration/test fault switches, channel barriers, and concurrent event collection. |
| `internal/resiliencedemo` | Run real SQL/Valkey scenarios, check values/counts, and clean up owned data. |

## Read policy

```mermaid
flowchart TD
    A[Get contact by ID] --> G[Register or join a singleflight lookup]
    G --> L[Shared load acquires its local gate]
    L --> V{Bounded Valkey GET}
    V -->|Valid HIT| H[Return the cached contact]
    V -->|MISS| P[Read PostgreSQL within the remaining load budget]
    V -->|Cache error or invalid payload| E[Record a cache diagnostic]
    E --> P
    P -->|Found| F[Attempt bounded SET with TTL]
    F --> R[Return valid SQL data even if cache fill failed]
    P -->|Not found or SQL error| X[Return source error without caching it]
```

This is **fail-open for cache reads/fills**, while SQL/domain failures still
propagate. Authentication, network, wrong-value-type, and payload errors remain
distinct diagnostics; they are not renamed to MISS. A malformed copy is replaced
after a successful source read/fill. Missing contacts are not negatively cached.
Hits leave TTL unchanged. A total load deadline still ends the operation with an
error, including when it expires after SQL returned but before fill completed.

The cache is an availability optimization, so this policy suits these public
teaching contacts. Other domains can need stricter freshness/error requirements.

## One shared load, independent callers

`golang.org/x/sync/singleflight` v0.24.0 coalesces concurrent lookups by contact
key in **one shared ResilientStore object**. A result channel delivers one value
and is not closed; callers receive once rather than ranging over it.

```mermaid
sequenceDiagram
    participant A as Caller A
    participant B as Caller B
    participant S as Shared store load
    participant P as PostgreSQL
    A->>S: Get starts a cold lookup
    S->>P: One authoritative read
    B->>S: Get joins the same in-flight lookup
    A->>A: Cancel request context
    S-->>A: context.Canceled for this waiter
    P-->>S: Contact within the shared load budget
    S->>S: Attempt cache fill
    S-->>B: Contact
```

The shared load uses the store's **application lifetime plus LoadTimeout**. Each
caller only controls its own wait. Even the initiating caller can leave without
canceling another caller's work. If all callers leave, the load can still finish
and populate the cache until its budget or owner cancellation ends it.

Request context values are not copied into a shared SQL read: it uses the owner
context. Authorization must run before a cached read, and namespace/keys must
identify the correct data set. This lab has no per-user contact access policy.

The owner must call `Close` before closing the borrowed SQL pool/client. Close
stops admission, cancels admitted loads/writes, and waits for admitted work. A
singleflight callback scheduled after closure is rejected before using a
dependency. Dependencies and observers must cooperate with this lifecycle.

## Local ordering of fills and writes

A shared load keeps a local gate from cache GET through SQL read and cache SET.
Replace/Patch/Delete use the same gate through SQL completion and DEL:

```mermaid
flowchart TD
    R[Local loader holds gate: SQL read and cache fill] --> W[Waiting writer acquires the released gate]
    W --> P[Writer commits its source change]
    P --> D[Writer invalidates the filled copy]
    D --> F[Retire the earlier flight and release the gate]
    F --> N[Subsequent Get starts a fresh lookup]
```

This prevents an older local loader from filling after that writer invalidates.
A read overlapping a write can return its earlier snapshot. After a successful
write/invalidation returns, a new Get cannot join the retired pre-write flight.
`Forget` leaves existing waiters alone; it can permit a newer flight alongside a
retired one. The gate still serializes their actual cache/SQL work.

There are 32 channel gates, selected by ID modulo 32. This bounds gate memory;
colliding IDs also serialize. Waiting on a gate is context-aware. This is a
simple teaching tradeoff, not a universal sharding/tuning recommendation.
Singleflight shares calls, not persistent results, and is not request idempotency.
The load deadline bounds waiting/work time; it does not provide a global request
admission limit or a distributed lock. SQL pools still have their own limits.

### Two objects/processes do not share these gates

The integration test deliberately reproduces this remaining race with two store
objects using the same namespace and real services:

```mermaid
sequenceDiagram
    participant A as Store instance A
    participant B as Store instance B
    participant P as PostgreSQL
    participant V as Valkey
    A->>P: Read old contact after a MISS
    P-->>A: Old snapshot, held by test barrier
    B->>P: Commit new contact values
    B->>V: DEL contact key
    V-->>B: Key absent
    A->>V: Fill with the earlier snapshot
    V-->>A: Old value stored with TTL
    B->>V: A later GET can hit the old copy
```

Another store object, another process, or a direct SQL writer bypasses this
instance's ordering. TTL limits the lifetime of an individual copy but does not
make this protocol strictly consistent. Distributed fencing/version protocols,
durable invalidation delivery, and stronger freshness requirements need a
separate design. This stage demonstrates the boundary rather than claiming to
solve it with a mutex or singleflight.

## Writes, partial success, and repair

Create goes directly to the source. Replace/Patch/Delete do not read cache
copies to decide what to write. Their source operation still uses the caller
context, linked to owner shutdown; source errors propagate without DEL.

After confirmed source success, invalidation gets a **fresh owner-context budget**
of CacheTimeout. Caller cancellation after commit does not immediately cancel
this DEL. Owner shutdown can still cancel it. SQL mutations are never automatically
retried, and there is no transaction spanning PostgreSQL and Valkey.

| Situation | Result |
| --- | --- |
| Source rejects input or fails | Source error; no success claimed or DEL attempted. |
| Source succeeds and DEL succeeds | Success; Replace/Patch return the committed contact. |
| Source succeeds and DEL fails | `*contactcache.InvalidationError`; Replace/Patch also return the committed value. |
| Retry `Invalidate(ctx, id)` | Delete only the exact key; no SQL mutation or request replay. |

A failed DEL can leave a readable old copy until repair/expiry, including after
reconnection. The demo intentionally reads it. Cache fallback alone does not
repair invalidation. Recovery here is an explicit bounded Invalidate call; there
is no persistent retry queue, automatic background repair, or circuit breaker.
An uncertain SQL commit remains different from confirmed success: use an
appropriate request idempotency protocol to resolve it, not a new blind write.

## Run the console lab

Use Go 1.27, a full checkout, and Docker CLI/Compose v2 with a running compatible
engine. Run the following bash/zsh commands from the repository root containing
`go.mod`, `chapters`, and `extensions`. First runs download pinned modules/images.

```sh
go version
docker info
docker compose version
```

For Rancher Desktop on macOS, select **dockerd (moby)**. With the home-directory
socket and localhost port forwarding, configure this terminal:

```sh
export DOCKER_HOST="unix://$HOME/.rd/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1
```

Docker Desktop/local Docker usually discovers its socket automatically. A different
Rancher VM network may need a reachable VM IP instead; inspect
`rdctl shell ip a show vznat` and keep the host override nonempty. The two
Testcontainers variables affect tests, not the demo's explicit service URLs.

```sh
docker compose -f extensions/02-database-sql/compose.yaml up -d --wait
docker compose -f extensions/04-valkey-cache/compose.yaml up -d --wait
export DATABASE_URL='postgres://sql_lab:sql_lab_local@127.0.0.1:5433/sql_lab?sslmode=disable'
export VALKEY_URL='valkey://default:cache_lab_local@127.0.0.1:6380/0'
go -C extensions/04-valkey-cache run ./03-concurrency-and-failures
```

PostgreSQL 18.6 listens on 5433 with its retained named volume; Valkey 9.0.7
listens on 6380 without snapshots/AOF. Both are bound to loopback with local lab
credentials and no transport TLS. Startup uses the existing golang-migrate
service schema and requires both services reachable. Fallback is a runtime
policy after client startup, not an optional-cache startup implementation.

The finite program creates one uniquely named contact and namespace, checks five
sections, then joins its work and removes its own row/key before closing clients.
It has a 40-second run budget; shared loads allow 5 seconds, cache calls 300ms,
and entries live for one minute. These are explicit teaching values. Migration
connection/statement/lock waits remain separately bounded by the SQL adapter.

Expected highlights:

```text
Cold MISS then warm HIT -> PostgreSQL contact loads=1
8 callers -> 1 shared SQL load; initiating caller canceled; 7 callers succeeded
Old local load finishes -> writer commits -> DEL -> next GET sees new phone
Controlled GET failure -> valid SQL result; cache_read_error observed
Controlled SET failure -> valid SQL result; cache_fill_error observed
Corrupt JSON -> SQL reload and replacement; cache_payload_error observed
SQL committed; DEL failed -> InvalidationError; old cached phone can still be read
Retry only Invalidate -> next GET reloads committed data; no SQL write retried
PostgreSQL contact loads=8; all controlled failures observed
```

The count measures source Get calls through the cache layer, not all SQL
statements, migrations, writes, or cleanup. Values and counts are asserted.
Barriers coordinate concurrency without fixed sleeps or timing assertions.

The Go fault switches deliberately fail selected commands while healthy calls
still use real Valkey. **The demo never stops your services.** Real connection
failure is tested separately by stopping only a Testcontainers-owned Valkey.

## Tests and checks

Ordinary tests need no servers; `-race` additionally needs clang/gcc:

```sh
go -C extensions/04-valkey-cache test -count=1 -timeout=60s ./...
CGO_ENABLED=1 go -C extensions/04-valkey-cache test -race -count=1 -timeout=90s ./...
go -C extensions/04-valkey-cache vet ./...
go -C extensions/04-valkey-cache mod tidy -diff
```

These cover routing, observable fallback, source/domain errors, canceled waiters
and writers, load/cache budgets, post-commit cancellation, shutdown, local write
ordering, failed invalidation, and targeted repair. The existing strict baseline
tests still verify its original error behavior. Each scenario explains its purpose.

For integration tests, keep the engine running. Testcontainers starts real
authenticated PostgreSQL/Valkey on discovered free ports, applies migrations,
and cleans up its own resources. It does not read DATABASE_URL/VALKEY_URL or use
Compose. Apply the Rancher variables above in the test terminal when applicable.

```sh
CGO_ENABLED=1 go -C extensions/04-valkey-cache test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/04-valkey-cache vet -tags=integration ./...
```

To focus on this stage:

```sh
CGO_ENABLED=1 go -C extensions/04-valkey-cache test -tags=integration -race -run '^TestCacheResilience$' -count=1 -timeout=5m -v ./integration
```

The new real-service suite proves shared loads with initiator cancellation,
local ordering, the deliberate cross-instance stale-fill race, runtime outage
fallback, committed writes with failed DEL, and repeated demo cleanup preserving
neighbor rows/keys/durable replay. Missing engines fail tagged tests. Expect `ok`
for tested packages and `[no test files]` for commands/helpers exercised indirectly.
Successful vet/tidy checks are normally quiet. CI's existing cache job includes
the new suite automatically.

`Observe` can feed logs/metrics; this stage's concurrent Probe records routing
and cache diagnostics. The event names include `cache_hit`, `cache_miss`,
`source_read`, `cache_read_error`, `cache_payload_error`, `cache_fill_error`, and
`invalidation_error`. `lookup_queued` and `write_wait` also coordinate the finite
scenarios. Observers must be fast, concurrent-safe, and must not reenter the store
or Close. No new OpenTelemetry exporter/collector is provisioned by this lab.

## Cleanup and next steps

After this program and other labs using the shared manual services finish:

```sh
docker compose -f extensions/04-valkey-cache/compose.yaml down
docker compose -f extensions/02-database-sql/compose.yaml down
```

The PostgreSQL volume remains, including other SQL/REST/gRPC data and saved
creation results. Valkey loses its disposable keys. Testcontainers owns separate
resources. Forced termination can leave this run's SQL row; its cache key has TTL.

Next come dedicated REST and gRPC cache labs, retaining the original database-only
labs for comparison. Guided exercises remain the final course block.

Read [resilient.go](../contactcache/resilient.go),
[resilient_writes.go](../contactcache/resilient_writes.go),
[unit scenarios](../contactcache/resilient_test.go),
[write scenarios](../contactcache/resilient_writes_test.go),
[demo](../internal/resiliencedemo/demo.go), and
[real-service scenarios](../integration/resilience_test.go).

References: [singleflight v0.24.0](https://pkg.go.dev/golang.org/x/sync@v0.24.0/singleflight),
[cache-aside limitations](https://learn.microsoft.com/en-us/azure/architecture/patterns/cache-aside),
and [Go context](https://pkg.go.dev/context).
