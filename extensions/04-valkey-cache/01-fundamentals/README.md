# Valkey cache fundamentals

Connect Go to a real Valkey server, store expiring values, distinguish hits/misses
from failures, and serialize a small typed contact. Read structs, errors,
context, and testing first. All execution and test setup is included here.

## Requirements and execution

Use Go 1.27 on PATH, a full repository checkout, and a running Docker-compatible
engine with Docker CLI and Compose v2. Run bash/zsh from the repository root
containing `go.mod`, `chapters`, and `extensions`. Initial runs download pinned
Go dependencies and the Valkey image. Race checks also require clang or gcc.

```sh
go version
docker info
docker compose version
docker compose -f extensions/04-valkey-cache/compose.yaml up -d --wait
export VALKEY_URL='valkey://default:cache_lab_local@127.0.0.1:6380/0'
go -C extensions/04-valkey-cache run ./01-fundamentals
```

Compose starts **Valkey 9.0.7** on `127.0.0.1:6380`. The Go module pins
**valkey-go v1.0.79** and Testcontainers v0.45.0. The URL selects the default
password-protected user and logical database 0. These are local teaching
credentials over loopback without transport TLS.

The supplied server disables snapshots and append-only persistence. Restarting
it loses cached keys. This stage needs no PostgreSQL or MongoDB: authoritative
storage and cache-aside are the next block. Cached data has a different lifecycle
from the durable records in those earlier labs.

The finite demo runs five sections and exits. Look for:

```text
Absent key -> ErrMiss; SET/PX then GET -> hit with hello.
An empty string is a hit; PTTL distinguishes expiring and persistent keys.
Versioned JSON -> Contact 42/Ana; corrupt JSON -> ErrInvalidPayload, not ErrMiss.
The short-lived key expired on Valkey; GET now reports ErrMiss.
A second client reads the same cache; canceled GET stays a context error.
DEL -> true, repeat DEL -> false; cleanup removes only this run's exact keys.
```

The demo registers its exact randomly namespaced keys before writing, then
attempts to delete them using fresh bounded cleanup time. It never flushes the
database or scans/deletes another caller's namespace. Most example keys have a
one-minute TTL; a separate short-lived key demonstrates actual server expiration.

## Learn in passes

| Pass | Follow | Check your understanding |
| --- | --- | --- |
| 1 | Run the demo and read [main.go](main.go), [demo.go](../internal/demo/demo.go), and [client.go](../cache/client.go). | Identify the reusable client, remote server, key owner, and shutdown order. |
| 2 | Read `Set` and `Get` in [store.go](../cache/store.go). | Explain why value and expiration are sent in one command, and why an empty value can be a hit. |
| 3 | Follow `TTL`, deletion, and the expiration poll. | Interpret all PTTL states and explain why a fixed sleep is not an expiration assertion. |
| 4 | Read [contact.go](../cache/contact.go) and its tests. | Distinguish a cache miss, invalid JSON/version, and a server/network failure. |

## The client and the server

`cache.Open` parses an explicit standalone URL, opens a reusable concurrent-safe
client, and checks it with PING. The application owns `Close`; `Store` borrows
that client. Create one client for its intended lifetime rather than opening one
per query. The second demo client illustrates server ownership, not the normal
query pattern.

The lab forces standalone mode, disables automatic read retries, and disables
the SDK's separate client-side caching feature. Ordinary `Do` calls demonstrate
remote commands and server TTLs. It does not use `DoCache`. Later resilience
lessons can introduce explicit retry/fallback decisions.

Client construction has no context argument. Initial connections therefore have
separate three-second dial/network bounds; PING and commands use caller contexts.
The demo owns a 20-second operation context and separate bounded key cleanup.
Already-canceled calls are rejected before sending; cancellation during a command
does not guarantee that the server never executed it. This matters for writes.

The wrapper accepts host-bearing `valkey`/`valkeys` and `redis`/`rediss` URLs,
without query options, and fixes this lesson's client settings explicitly.
The Compose example uses plaintext `valkey://`. TLS URLs use verification and a
TLS 1.3 minimum; this lab's runnable local setup does not provision certificates.

## Keys, values, and hit/miss semantics

Valkey stores remote key/value data. A key such as
`course:fundamentals:<random>:contact:42` combines an application/lesson prefix,
an owned run identifier, and the record name. Key identity is exact; the wrapper
does not silently trim names.

Valkey's protocol supports binary data. This teaching API restricts keys to
1–200 UTF-8 bytes without NUL for readability; string values can contain arbitrary
bytes, including empty text. JSON serialization has its own UTF-8/model rules.

| Observation | Result |
| --- | --- |
| Existing key storing `"hello"` | Value and nil error: hit |
| Existing key storing `""` | Empty value and nil error: hit |
| Missing/expired key | `ErrMiss`, with the driver's nil-reply cause retained |
| Authentication, wrong server value type, network failure | Wrapped failure, not `ErrMiss` |
| Existing string with invalid contact JSON/version | `ErrInvalidPayload`, not `ErrMiss` |

Use `errors.Is(err, cache.ErrMiss)` for ordinary absence. A miss does not tell
you whether a key was never written, expired, evicted, deleted, or lost during
restart. This stage reports errors explicitly; the next cache-aside stage will
decide when/how to retrieve authoritative data instead.

## TTL and atomic SET

TTL means **time to live**. `Set` sends `SET key value PX milliseconds` so the
value and expiration are assigned together. Sending SET and EXPIRE separately
could leave a persistent key if the latter failed. The wrapper accepts positive
whole-millisecond durations and rejects fractional milliseconds instead of
silently truncating them to zero.

Every successful wrapper SET assigns a fresh TTL, including overwrites. Raw SET
without expiration normally removes an existing TTL; the demo uses one such
command to show a persistent key. The wrapper itself always requires expiration.

`TTL` uses PTTL for millisecond metadata:

| PTTL reply | `Expiration` |
| --- | --- |
| `-2` | `Exists=false` |
| `-1` | `Exists=true`, `HasTTL=false` |
| Zero or positive milliseconds | `Exists=true`, `HasTTL=true`, `Remaining` holds the duration |

Zero remaining milliseconds can describe a key about to expire. GET and PTTL
are separate observations, not one snapshot. Another write/deletion or expiration
can occur between them. A timer in Go does not control the remote server clock;
the demo/tests poll for a miss with a deadline rather than assuming one sleep
proves expiration.

DEL returns whether it removed the key. A repeat returns false with nil error.
The resulting absent state is stable, but this is not durable request-key replay.
Repeating SET also refreshes expiration, so it changes the key's lifetime even
when the stored text is identical.

## A typed JSON cache payload

Valkey strings hold serialized bytes. The contact helper uses this envelope:

```json
{"schema_version":1,"contact":{"id":42,"name":"Ana"}}
```

The small contact model requires a positive integer ID and a trimmed UTF-8 name
of 1–80 runes without NUL. Encoding and decoding use `encoding/json` with typed
integer fields. Additive unknown fields are tolerated, while unsupported payload
versions, bad JSON/types, or invalid values return `ErrInvalidPayload`.
Underlying JSON errors remain available through `errors.As`.

The cache format version is independent of a database schema/migration version.
Compatibility decisions, invalidation, and authoritative data access will belong
to later blocks; an invalid payload is not silently treated as a missing key here.

## Ordinary tests and static checks

These commands need no running cache or container engine:

```sh
go -C extensions/04-valkey-cache test -count=1 -timeout=60s ./...
go -C extensions/04-valkey-cache vet ./...
go -C extensions/04-valkey-cache mod tidy -diff
```

Unit tests cover input/TTL boundaries, PTTL states and duration overflow, typed
JSON round trips, unsupported/corrupt payloads, and error causes. With a C compiler:

```sh
CGO_ENABLED=1 go -C extensions/04-valkey-cache test -race -count=1 -timeout=90s ./...
```

## Integration tests with Testcontainers

Start the engine. Testcontainers starts a real authenticated Valkey instance,
waits for readiness, and registers cleanup. Tests never read `VALKEY_URL` or use
the Compose cache. They own their keys and stop only their disposable test server
to check an actual unavailable-server error.

For Rancher Desktop on macOS, select **dockerd (moby)**. With its home-directory
socket and forwarded ports reachable on localhost, set these in the test terminal:

```sh
export DOCKER_HOST="unix://$HOME/.rd/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1
```

Docker Desktop/local Docker usually uses automatic detection. If Rancher exposes
ports through its VM instead, use that reachable address; inspect the VZ interface
with `rdctl shell ip a show vznat`. Keep the host override nonempty. With clang/gcc:

```sh
docker info
CGO_ENABLED=1 go -C extensions/04-valkey-cache test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/04-valkey-cache vet -tags=integration ./...
```

The [integration cases](../integration/valkey_test.go) check real strings and
misses, empty/binary hits, expiring/persistent metadata, actual expiration,
JSON/server-type errors, independent clients, canceled/expired calls, failed
authentication, unavailable-server errors, and repeated demo cleanup preserving
another key. Each scenario has an explanatory comment. Missing engines fail
tagged tests rather than silently skipping; first runs may download server and
cleanup-helper images. CI includes ordinary checks, races, and this integration suite.

Expect `ok` for tested packages. `[no test files]` is normal for the command and
demo packages exercised by integration tests. Successful vet/metadata checks
normally print nothing.

## Inspect and stop the manual cache

The server image supplies the CLI; a host-side installation is optional:

```sh
docker compose -f extensions/04-valkey-cache/compose.yaml exec valkey \
  valkey-cli --no-auth-warning -a cache_lab_local ping
```

After Go processes finish:

```sh
docker compose -f extensions/04-valkey-cache/compose.yaml down
```

This lab configures no persistent cache dataset. Restarting/recreating the server
loses its keys; do not expect MongoDB/PostgreSQL-style retained records here.
Testcontainers manages its own separate test containers.

Read in order: [main.go](main.go), [demo.go](../internal/demo/demo.go),
[client.go](../cache/client.go), [store.go](../cache/store.go),
[contact.go](../cache/contact.go), [unit tests](../cache/contact_test.go),
and [integration tests](../integration/valkey_test.go).

Next, [cache-aside with PostgreSQL](../02-cache-aside/README.md) adds authoritative
records, observable cold/warm reads, and write invalidation. This fundamentals
program remains independent of PostgreSQL.

References: [Valkey](https://valkey.io/topics/introduction/),
[SET](https://valkey.io/commands/set/), [PTTL](https://valkey.io/commands/pttl/),
[Go client](https://github.com/valkey-io/valkey-go), and
[Testcontainers Valkey](https://golang.testcontainers.org/modules/valkey/).
