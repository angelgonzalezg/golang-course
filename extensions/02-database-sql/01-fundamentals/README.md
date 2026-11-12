# PostgreSQL fundamentals

Run parameterized CRUD through Go's `database/sql`, distinguish SQL NULL from
empty text, and follow pool/context ownership. The complete background is in
the [SQL learning guide](../README.md); all execution steps are included here.

## Requirements and execution

Use Go 1.27 on PATH, a full repository checkout, and a running Docker-compatible
engine with the Docker CLI and Compose v2. Run bash/zsh commands from the root
containing `go.mod`, `chapters`, and `extensions`. The first run can download Go
dependencies/container images. Race tests additionally need clang or gcc.

```sh
go version
docker info
docker compose version
docker compose -f extensions/02-database-sql/compose.yaml up -d --wait
export DATABASE_URL='postgres://sql_lab:sql_lab_local@127.0.0.1:5433/sql_lab?sslmode=disable'
go -C extensions/02-database-sql run ./01-fundamentals
```

Compose starts the dedicated PostgreSQL database on `127.0.0.1:5433`, waits for
readiness, and retains data in a named volume. The URL uses its local teaching
credentials; `sslmode=disable` matches this loopback-only database setup.

The finite program creates its own table if needed and runs five sections. Look for:

```text
Stored empty text, then SQL NULL; presence stayed distinct.
Duplicate name -> ErrNameTaken; canceled read -> context.Canceled.
Data belongs to PostgreSQL, rather than to one Go connection pool.
Deleted the demo contact; repeated lookup -> ErrNotFound.
```

It attempts to remove only its own randomly named contact and closes its pools
before exiting. The table bootstrap is separate from the versioned migration
examples introduced in the next lesson. IDs can have gaps; use returned IDs.

## Ordinary tests

These commands compile/check the module without a running database:

```sh
go -C extensions/02-database-sql test -count=1 -timeout=60s ./...
go -C extensions/02-database-sql vet ./...
go -C extensions/02-database-sql mod tidy -diff
```

## Integration tests with Testcontainers

Start the container engine. Testcontainers creates a separate PostgreSQL
container, waits for readiness, and registers cleanup. Tests do not connect to
the Compose database or read `DATABASE_URL`. Race detection needs a C compiler.

For Rancher Desktop on macOS, select **dockerd (moby)**. With the home-directory
socket and ports forwarded to localhost, set these in the same test terminal:

```sh
export DOCKER_HOST="unix://$HOME/.rd/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1
```

Docker Desktop/local Docker usually uses automatic detection. If Rancher exposes
ports through its VM address instead, use that reachable address for the host
override; `rdctl shell ip a show vznat` shows the VZ interface. Never set an empty
host override. Then run:

```sh
docker info
CGO_ENABLED=1 go -C extensions/02-database-sql test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/02-database-sql vet -tags=integration ./...
```

The command runs the SQL module's implemented integration suites. The
fundamentals contracts cover CRUD, NULL/empty values, repeated assignments,
parameter binding, pool reopening, concurrent name uniqueness, cancellation,
and demo cleanup. Expect `ok` for tested packages; `[no test files]` for command
packages is normal. Successful vet/metadata checks normally print nothing.

## Stop the manual database

After the Go processes finish:

```sh
docker compose -f extensions/02-database-sql/compose.yaml down
```

The volume remains available for another session. Use `down -v` only when
intentionally discarding the shared SQL/REST/gRPC lab database and its retained
data/results. Testcontainers manages its separate disposable test resources.

Continue to [transactions and migrations](../02-transactions/README.md) for
multi-statement atomicity and versioned schema changes.
