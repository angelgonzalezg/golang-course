# Valkey cache lab

Learn remote caching in an independent Go 1.27 module with real Valkey, the
valkey-go client v1.0.79, and Testcontainers v0.45.0. Start with typed values,
errors, context, and testing, then connect the cache to authoritative PostgreSQL
records with an application-managed cache-aside layer.

| Stage | Status |
| --- | --- |
| 1. Fundamentals | Implemented: [standalone execution/test guide](01-fundamentals/README.md), SET/PX, GET/DEL, hits/misses, TTL states, real expiration, typed JSON, client ownership, and real Valkey tests. |
| 2. Cache-aside with PostgreSQL | Implemented: [standalone execution/test guide](02-cache-aside/README.md), cold/warm reads, SQL read counters, complete contact payloads, TTL reloads, and commit-before-invalidation. |
| 3. Concurrency and failures | Implemented: [standalone execution/test guide](03-concurrency-and-failures/README.md), observable fallback, bounded shared loads, independent caller cancellation, local fill/write ordering, and failed-invalidation repair. |
| 4. REST with cache | Planned: dedicated lab reusing the SQL-backed API and shared cache layer. |
| 5. gRPC with cache | Planned: dedicated lab reusing the same contact cache behavior. |

## Run directly

Use Go 1.27 on PATH, a full checkout, and a running Docker-compatible engine
with Docker CLI/Compose v2. Run bash/zsh from the repository root containing
`go.mod`, `chapters`, and `extensions`. Initial runs download pinned modules/images.

```sh
go version
docker info
docker compose version
docker compose -f extensions/04-valkey-cache/compose.yaml up -d --wait
export VALKEY_URL='valkey://default:cache_lab_local@127.0.0.1:6380/0'
go -C extensions/04-valkey-cache run ./01-fundamentals
```

Compose starts Valkey 9.0.7 on `127.0.0.1:6380` with a local password and no
snapshots/AOF persistence. This is a loopback teaching setup without transport
TLS. The finite program prints five sections, deletes only its randomly named
keys, and closes its clients. Look for `Absent key -> ErrMiss`, the empty-string
hit, versioned JSON, observed server expiration, and repeated deletion output.
It needs no PostgreSQL or MongoDB. The [stage guide](01-fundamentals/README.md)
explains each operation, payload version, and error distinction.

After Go processes finish, stop the manual cache:

```sh
docker compose -f extensions/04-valkey-cache/compose.yaml down
```

Restarting this cache loses its keys; durable application data belongs in an
authoritative store. Testcontainers owns its separate disposable resources.

## Run cache-aside with PostgreSQL

From the same repository root and engine setup above, start both services:

```sh
docker compose -f extensions/02-database-sql/compose.yaml up -d --wait
docker compose -f extensions/04-valkey-cache/compose.yaml up -d --wait
export DATABASE_URL='postgres://sql_lab:sql_lab_local@127.0.0.1:5433/sql_lab?sslmode=disable'
export VALKEY_URL='valkey://default:cache_lab_local@127.0.0.1:6380/0'
go -C extensions/04-valkey-cache run ./02-cache-aside
```

The finite console demo applies the existing golang-migrate service migrations,
creates its own contacts, and shows CREATE, cold/warm GET, REPLACE, PATCH, expiry,
and DELETE. The first GET increments the PostgreSQL contact-read counter to 1;
the second is a Valkey hit and leaves it at 1. Writes go directly to PostgreSQL;
successful updates/deletes then invalidate the key. Only this demo's contacts
and keys are cleaned up. Existing REST/gRPC lab data/replay results remain.

This baseline exposes cache failures; it does not yet provide outage fallback
or strict consistency under concurrent writes. Read the
[complete stage guide](02-cache-aside/README.md) for payloads, error outcomes,
expected output, and the concurrency limits. It starts no HTTP/gRPC server.

After the demo and any other labs using these shared services finish:

```sh
docker compose -f extensions/04-valkey-cache/compose.yaml down
docker compose -f extensions/02-database-sql/compose.yaml down
```

The PostgreSQL named volume remains. Valkey is disposable.

## Run concurrency and failures

With the same Compose services and DATABASE_URL/VALKEY_URL above:

```sh
go -C extensions/04-valkey-cache run ./03-concurrency-and-failures
```

The finite demo coalesces eight callers into one SQL load, cancels the initiating
caller without canceling the others, orders a local fill/write, and exercises
controlled GET/SET/DEL failures plus corrupt JSON against real data. Fail-open
reads keep valid SQL results; failed post-commit invalidation remains explicit
and can be repaired by deleting only the key. The program closes/joins its work
and cleans only its own contact/key. It never stops the manual services.

The [stage guide](03-concurrency-and-failures/README.md) includes full setup,
diagrams, expected counters, policy choices, and tests. Local gates/singleflight
are not distributed consistency; a real two-instance test reproduces the remaining
stale-fill race. The strict stage-2 implementation remains available for comparison.

## Verify directly

Ordinary tests need Go alone; race checks additionally need clang or gcc:

```sh
go -C extensions/04-valkey-cache test -count=1 -timeout=60s ./...
go -C extensions/04-valkey-cache vet ./...
go -C extensions/04-valkey-cache mod tidy -diff
CGO_ENABLED=1 go -C extensions/04-valkey-cache test -race -count=1 -timeout=90s ./...
```

For integration tests, start the engine. Testcontainers starts/waits for its own
authenticated Valkey and PostgreSQL servers and cleans them up; it does not use
`VALKEY_URL`, `DATABASE_URL`, or Compose. One fundamentals case stops only its
owned Valkey server to verify failure classification.

For Rancher Desktop on macOS, select **dockerd (moby)**. With the home-directory
socket and localhost port forwarding, set these in the test terminal:

```sh
export DOCKER_HOST="unix://$HOME/.rd/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1
```

Docker Desktop/local Docker usually uses automatic detection. A different Rancher
VM networking setup may need a reachable VM address instead; inspect it with
`rdctl shell ip a show vznat`. Keep the override nonempty. With a C compiler:

```sh
docker info
CGO_ENABLED=1 go -C extensions/04-valkey-cache test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/04-valkey-cache vet -tags=integration ./...
```

Unit tests cover JSON, input boundaries, TTL/error mapping, cache-aside routing,
and write/invalidation ordering. Real server tests cover strings, expiration,
metadata, independent clients, authentication/type/connection failures, contexts,
cache-aside source-read counts, SQL writes/deletes, shared-load cancellation,
local/cross-instance races, real cache-outage fallback, and demo cleanup preserving
other rows/keys/replay results. Initial runs can pull both servers and the cleanup-helper image;
missing engines fail tagged tests.
Expect `ok` for test packages and `[no test files]` for command/helper packages.
Successful vet/metadata checks are normally quiet.

CI includes this ninth module in ordinary/race checks and runs its integration
suite separately. Dedicated REST/gRPC cache labs remain later blocks; guided
exercises remain the final course block.
