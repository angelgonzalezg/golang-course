# Valkey cache lab

Learn remote caching in an independent Go 1.27 module with real Valkey, the
valkey-go client v1.0.79, and Testcontainers v0.45.0. Start with typed values,
errors, context, and testing; authoritative persistence and cache-aside follow.

| Stage | Status |
| --- | --- |
| 1. Fundamentals | Implemented: [standalone execution/test guide](01-fundamentals/README.md), SET/PX, GET/DEL, hits/misses, TTL states, real expiration, typed JSON, client ownership, and real Valkey tests. |
| 2. Cache-aside with PostgreSQL | Proposed: application-managed lookup/loading from the source of truth and explicit write invalidation. |
| 3. Concurrency and failures | Proposed: duplicate-load suppression/cache stampede, cancellation, and defined cache-outage behavior. |

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

## Verify directly

Ordinary tests need Go alone; race checks additionally need clang or gcc:

```sh
go -C extensions/04-valkey-cache test -count=1 -timeout=60s ./...
go -C extensions/04-valkey-cache vet ./...
go -C extensions/04-valkey-cache mod tidy -diff
CGO_ENABLED=1 go -C extensions/04-valkey-cache test -race -count=1 -timeout=90s ./...
```

For integration tests, start the engine. Testcontainers starts/waits for an
authenticated Valkey server and cleans it up; it does not use `VALKEY_URL` or
Compose. One case stops only that owned server to verify failure classification.

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

Unit tests cover JSON, input boundaries, and TTL/error mapping. Real server tests
cover strings, expiration, metadata, independent clients, authentication/type/
connection failures, contexts, and demo cleanup preserving other keys. Initial
runs can pull Valkey and the cleanup-helper image; missing engines fail tagged tests.
Expect `ok` for test packages and `[no test files]` for command/helper packages.
Successful vet/metadata checks are normally quiet.

CI includes this ninth module in ordinary/race checks and runs its integration
suite separately. Cache-aside and concurrency/failure policy remain later blocks;
guided exercises remain the final course block.
