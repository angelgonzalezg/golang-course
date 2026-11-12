# gRPC with PostgreSQL

Connect the existing Protobuf service to the persistent contacts adapter used by
[REST with PostgreSQL](../../02-database-sql/05-rest-postgres/README.md).
Keep the RPC contract and add durable creation results that survive server,
pool, and process lifetimes.

Read the [five preceding gRPC lessons](../README.md), then the SQL lessons on
[transactions/migrations](../../02-database-sql/02-transactions/README.md),
[durable idempotency](../../02-database-sql/03-idempotency/README.md), and
[sqlc](../../02-database-sql/04-sqlc/README.md). The earlier in-memory examples
remain useful for learning each transport concept before adding persistence.

## Run the finite walkthrough

Requirements: Go 1.27 on PATH and a full repository checkout, including the
locally referenced course and SQL modules. Run bash/zsh commands from the root
containing `go.mod`, `chapters`, and `extensions`. Demo/server modes also need a
running Docker-compatible engine, Docker CLI, and Compose v2. Race checks need
clang or gcc. Generated bindings are supplied; the first run can download pinned
Go dependencies and container images. The finite demo prepares TLS/credentials
itself and collects telemetry locally.

```sh
go version
docker info
docker compose version
docker compose -f extensions/02-database-sql/compose.yaml up -d --wait
export DATABASE_URL='postgres://sql_lab:sql_lab_local@127.0.0.1:5433/sql_lab?sslmode=disable'
go -C extensions/01-protobuf-and-grpc run ./06-postgres
```

The program migrates the service schema, creates temporary TLS material and
credentials in memory, and performs real TCP RPCs. Compose exposes its dedicated
database on `127.0.0.1:5433`, using the teaching credentials in the URL and a
retained volume. Look for:

```text
Lost reply -> two attempts, one committed creation; replay=true.
New server/pool -> original ID, fields, and version from PostgreSQL.
Stale version -> Aborted; partial import keeps its first item; stream ends at EOF.
Deleted contact stays absent; its original creation/version still replays.
```

The last section prints locally collected OpenTelemetry metrics and bounded
trace summaries. The first server/channel/pool close before the second set
opens. Separate-process recovery is also checked by the integration tests.

Each finite run uses random name/key prefixes and removes only those demo rows
and its exact saved key. Other callers' data is retained. Cleanup has a fresh
budget, and every channel/server/provider/pool has an owner. The standalone
server below retains committed data and request records.

Apply only the shared service migrations with:

```sh
go -C extensions/01-protobuf-and-grpc run ./06-postgres -migrate-only
```

It reports `Service schema version: 1`. Both persistent transports use the
`sql_service_*` tables and the SQL module's golang-migrate adapter. Startup
rejects dirty/future migration state and verifies connectivity before publishing
startup health. Health is not a continuous database-dependency probe.

After stopping the Go servers/clients, stop the manual database with:

```sh
docker compose -f extensions/02-database-sql/compose.yaml down
```

This retains the volume. Use `down -v` only to intentionally discard the shared
SQL/REST/gRPC lab database, including contacts and replay records.

## Learn in passes

| Pass | Follow | Check your understanding |
| --- | --- | --- |
| 1 | Ordinary get/list/update/delete through the existing service. | Locate the interface where PostgreSQL replaces the memory store. |
| 2 | Authenticated keyed creation. | Trace verified subject, key, decoded fields, transaction, and saved result. |
| 3 | A lost reply and the bounded retry client. | Explain why the second attempt replays rather than inserting again. |
| 4 | A new server/pool and then a new process. | Identify which state lives in PostgreSQL and which resources are rebuilt. |
| 5 | Concurrent writers, cancellation, and streams. | Distinguish a canceled duplicate, rollback of its original writer, and partial stream commits. |
| 6 | Standalone credentials, discovery, limits, and telemetry. | Explain who owns each resource and which protections run before replay. |

## Follow one create

```text
Client: verified TLS + bearer credential + x-idempotency-key
  observer -> authorization -> operation deadline -> durable-key middleware
  rpcapi.CreateContact: require input, convert known fields to contacts.Input
  rpcpostgres.Store.Create: attach the verified subject/key to CreateOnce
  contactpg.CreateOnce:
    BeginTx(ReadCommitted), acquire the transaction advisory lock
    read the result in a fresh statement after the key wait
    replay matching fields, reject changed fields, or insert contact + result
    Commit, then return the original Contact snapshot
  rpcapi: construct the Protobuf Contact and opaque content version
Client: Contact plus x-idempotency-replayed trailer
```

The finite demo models a reply lost after commit by returning `Unavailable`.
Retry keeps the identity and input stable, and authorization runs again:

```mermaid
sequenceDiagram
    participant C as TLS client
    participant S as gRPC server
    participant A as rpcpostgres.Store
    participant P as PostgreSQL
    C->>S: CreateContact with credential and stable key
    S->>S: Authenticate, authorize, check key
    S->>A: Create with verified subject and key
    A->>P: CreateOnce locks key, inserts contact and result
    P-->>A: Commit confirmed with original snapshot
    A-->>S: Contact with replay=false
    S-->>C: Unavailable because demo discards reply
    C->>S: Retry same credential, key, and original fields
    S->>S: Authenticate, authorize, check key again
    S->>A: Create with the same identity
    A->>P: CreateOnce locks key, reads saved result
    P-->>A: Read transaction committed with original snapshot
    A-->>S: Contact with replay=true
    S-->>C: Contact and replay trailer
```

These are the successful database paths behind two RPC attempts. A fresh server
or pool can read the same retained snapshot; neither request depends on an
in-process cache. The retry client still obeys its existing attempt/deadline limits.

Authorization runs on every invocation, including replay. The caller cannot
choose a subject using an identity/role header. The lab policy maps verified
tokens to `reader-client` or `writer-client`; only the writer may mutate or use
reflection. Credential rotation must keep the intended caller identity stable
if it should retain access to earlier keys. This two-role teaching policy is
not a multi-user identity provider or per-contact authorization system.

### Creation rules and RPC outcomes

| Situation | Outcome |
| --- | --- |
| Unary create without exactly one valid key | `InvalidArgument`. Keys require 1–64 ASCII letters, digits, `_`, or `-`. |
| Missing/invalid credential or insufficient permission | `Unauthenticated` / `PermissionDenied`, before accessing a saved result. |
| Fresh successful creation | Contact and result commit together; trailer `x-idempotency-replayed: false`. |
| Same subject/key and original decoded fields | Original ID, name/email/phone, and version; trailer `true`. |
| Same subject/key with changed valid fields | `FailedPrecondition`; no second creation. |
| Name conflict | `AlreadyExists`; rollback leaves no saved key. |
| Invalid fields or absent input | `InvalidArgument`, with the service's structured validation details. |
| Stale expected version | `Aborted`; read current state before deciding on another mutation. |
| Cancellation or expired caller deadline | `Canceled` / `DeadlineExceeded`; SQL waits observe the context. |
| Unexpected database failure | Redacted `Internal`; no SQL/connection data in the public status. |

Keys apply only to unary `CreateContact`. Other unary methods and every stream
reject supplied keys. `ImportContacts` continues to commit each valid item
independently; a later failure retains earlier rows and returns no final summary.
`StreamContacts` remains one finite page. `LookupContacts` retains its ordered,
correlated contact/failure outcomes. Call the configured server to exercise its
middleware; direct Go calls to service/store methods do not run interceptors.

### Compare the two idempotency lessons

| Policy | Stage 5 memory cache | Stage 6 PostgreSQL |
| --- | --- | --- |
| Retained state | First result or terminal error, until process exit. | Committed successful creation snapshots, retained in PostgreSQL. |
| Request binding | Whole decoded request via `proto.Equal`, including unknown fields. | Original known name/email/phone values before trimming, matching the persistent REST policy. |
| Coordination | In-process mutex and per-entry completion channel. | Transaction advisory lock and exact subject/key database primary key. |
| Failure before commit | Generic middleware cannot undo an underlying operation. | SQL transaction rolls back both contact and result. |
| Capacity/retention | Bounded cache rejects new keys when full. | No automatic expiry; deleting saved records forgets their keys. |

Plain proto3 strings give the same known field values for omitted and explicitly
empty email/phone. Changing field whitespace still changes request identity.
Unknown Protobuf fields have no business meaning in this typed storage policy
and do not change its fingerprint. Introducing a new meaningful field requires
deliberately versioning the shared request identity and its compatibility rules.

REST and gRPC can share a saved operation when an application deliberately maps
the same trusted subject and sends the same key/known original fields. Their
default lab identities differ: REST uses `local-user`, gRPC uses `writer-client`.
Sharing tables alone does not merge those caller namespaces. Integration tests
exercise an explicit shared identity and cross-transport version invalidation.

A saved result is historical. It survives edits and deletion, and replay does
not recreate a deleted contact. A lost response or uncertain commit must be
resolved with the same key and original fields. A different key describes new
work. The client retries only `Unavailable`, at most three application attempts,
with one two-second budget covering attempts and backoff. Native configured
retries are disabled; gRPC can still perform its documented transparent retries.
Other mutations/streams receive no automatic replay policy. Call options such
as `grpc.Trailer` apply to each sequential attempt and expose the final outcome.

## Run separate processes

Create certificates in a new directory under a temporary parent:

```sh
export GRPC_PG_LAB_DIR="$(mktemp -d)"
go -C extensions/01-protobuf-and-grpc run ./06-postgres \
  -init-tls "$GRPC_PG_LAB_DIR/tls"
export GRPC_LAB_READER_TOKEN='postgres-reader-demo'
export GRPC_LAB_WRITER_TOKEN='postgres-writer-demo'
```

These are local teaching credentials. The helper creates owner-only files,
requires a new certificate directory, and issues certificates valid for 24 hours.

Start the persistent server with `DATABASE_URL` still configured:

```sh
go -C extensions/01-protobuf-and-grpc run ./06-postgres \
  -serve -tls-dir "$GRPC_PG_LAB_DIR/tls" -reflection
```

The default address is `127.0.0.1:50053`; `-addr` can change it. In another
terminal, first set the TLS directory to the actual path printed by `-init-tls`
and set the same writer credential. Exported variables are local to each shell;
replace the example directory below with that printed path:

```sh
export GRPC_PG_TLS_DIR='/absolute/path/printed/by/init-tls'
export GRPC_LAB_WRITER_TOKEN='postgres-writer-demo'
go -C extensions/01-protobuf-and-grpc run ./06-postgres \
  -target 127.0.0.1:50053 -tls-dir "$GRPC_PG_TLS_DIR" \
  -key create-persistent-1 -name 'Persistent contact' -email 'ana@example.com'
```

The first success prints `replay=false`; a repeat prints `replay=true` with the
same ID/version. Stop the server with Ctrl+C, restart it, and run that same
client command again. Its saved result remains available. Choose your own name
and key if those examples already identify another operation in your lab DB.
Changing the name/email/phone while reusing a successful key is a conflict.
The standalone client needs the server address, public CA, and writer credential,
not `DATABASE_URL` or the server's private key.

For optional command-line inspection, install grpcurl on PATH and keep the
server running with `-reflection`. In the client terminal, using the TLS path
and writer credential configured above:

```sh
grpcurl -cacert "$GRPC_PG_TLS_DIR/ca.pem" \
  -H "authorization: Bearer $GRPC_LAB_WRITER_TOKEN" \
  -d '{"service":"contacts.v1.ContactService"}' \
  127.0.0.1:50053 grpc.health.v1.Health/Check
grpcurl -cacert "$GRPC_PG_TLS_DIR/ca.pem" \
  -H "authorization: Bearer $GRPC_LAB_WRITER_TOKEN" \
  127.0.0.1:50053 list
grpcurl -cacert "$GRPC_PG_TLS_DIR/ca.pem" \
  -H "authorization: Bearer $GRPC_LAB_WRITER_TOKEN" \
  -rpc-header 'x-idempotency-key: grpcurl-create-1' \
  -d '{"input":{"name":"grpcurl contact"}}' \
  127.0.0.1:50053 contacts.v1.ContactService/CreateContact
```

Health Check is public over TLS; this grpcurl command supplies credentials for
reflection, which discovers its message schema and requires the writer role.
Other health methods require authentication. `-H` sends authentication to both
reflection and the requested RPC. `-rpc-header` sends the idempotency key only to
CreateContact, because reflection is a stream and this lab rejects keys on
streaming RPCs. See [grpcurl's flag definitions](https://github.com/fullstorydev/grpcurl/blob/master/cmd/grpcurl/grpcurl.go)
for that distinction. Repeat the keyed create with the same fields to replay it.
The Go client works without grpcurl; the
[infrastructure guide](../04-infrastructure/README.md) provides further discovery examples.

### Bounds and ownership

RPC TLS uses a minimum of **TLS 1.3** with CA/hostname verification. PostgreSQL
connection security is configured separately through `DATABASE_URL`; the supplied
Compose URL uses `sslmode=disable` for its loopback teaching database.
The service keeps stage 5's **1,024-byte per-message** send/receive bounds.
Large unary pages can therefore need a smaller page size or server streaming.
Each individual streamed Contact must still fit that message budget.
The endpoint caps unary work at three seconds and finite streams at fifteen
seconds; an earlier caller deadline wins. These budgets include SQL pool/lock
waits. They are separate from migration connection/statement/lock limits.

The SQL pool, channels, server, and local telemetry providers are application-owned.
Shutdown drains/joins handlers before closing SQL; expired cleanup budgets
force transport cancellation and still join the server. Metrics/traces describe
RPCs, without installing global providers or claiming SQL query instrumentation.
Startup health confirms completed setup; it does not continuously probe PostgreSQL.
Collection search retains the shared full-snapshot Go paging behavior and costs
explained in [REST with PostgreSQL](../../02-database-sql/05-rest-postgres/README.md#preserve-go-string-and-collection-behavior).

## Read the code in order

1. [Existing service interface](../rpcapi/service.go) and
   [persistent store](../../02-database-sql/contactpg/store.go).
2. [Scope/store adapter](../internal/rpcpostgres/store.go): metadata checks,
   verified identity, keyed unary creation versus ordinary stream-item creation.
3. [Server composition](../internal/rpcpostgres/server.go): TLS, authorization,
   context budgets, durable policy, standard services, limits, and telemetry.
4. [Finite demo](../internal/postgresapp/demo.go),
   [server owner](../internal/postgresapp/server.go),
   [client](../internal/postgresapp/client.go), and [main.go](main.go).
5. [Integration fixture](../integration/database_test.go) and
   [contracts](../integration/postgres_test.go), then
   [durability](../integration/durability_test.go),
   [concurrency](../integration/concurrency_test.go),
   [cancellation](../integration/context_test.go),
   [REST sharing](../integration/rest_test.go),
   [process recovery](../integration/process_test.go), and
   [infrastructure/demo checks](../integration/infrastructure_test.go).

Code and test comments explain phases, resource ownership, and each scenario's
purpose. SQL remains generated by the pinned sqlc tooling in the SQL module;
the existing Protobuf contract/generated bindings are reused.

## Validate

Ordinary tests compile the new lesson without a database or protoc:

```sh
go -C extensions/01-protobuf-and-grpc test -count=1 -timeout=60s ./...
go -C extensions/01-protobuf-and-grpc vet ./...
go -C extensions/01-protobuf-and-grpc mod tidy -diff
```

Integration tests start/wait for their own PostgreSQL containers and register
cleanup. They require a running container engine and a C compiler for race
detection; Compose and `DATABASE_URL` are not used by the tests.

For Rancher Desktop on macOS, select **dockerd (moby)**. With its home-directory
socket and ports forwarded to localhost, set these in the test terminal:

```sh
export DOCKER_HOST="unix://$HOME/.rd/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1
```

Docker Desktop/local Docker usually works with automatic detection. Rancher VM
networking may need its reachable VM address instead; inspect it with
`rdctl shell ip a show vznat`. Never set an empty host override. Then run:

```sh
docker info
CGO_ENABLED=1 go -C extensions/01-protobuf-and-grpc test -tags=integration -race -count=1 -timeout=5m -v ./...
go -C extensions/01-protobuf-and-grpc vet -tags=integration ./...
```

Tagged tests own their database/container and never read the learner's
`DATABASE_URL`. Missing Docker is a failure, not a silent skip. Tests compare
memory/PostgreSQL unary and streaming semantics, then cover twelve concurrent
retries across independent servers/pools, competing payloads, conditional and
partial writes, rollback/redaction, cancellation/deadlines, caller permissions,
TLS/discovery/message limits/telemetry, cross-transport replay, and finite cleanup.

One child-process test discards the first process's successful reply and recovers
the same creation in a new process. This models a lost response after commit;
it does not inject real network packet loss. Cancellation tests observe actual
`pg_stat_activity` lock waits before canceling. `TestPostgresProcessHelper` is
the child entry point and simply returns in the normal parent run. Packages
showing `[no test files]` still compile; their database behavior is exercised by
the explicitly tagged integration package. CI runs both SQL and gRPC PostgreSQL
suites with the race detector.

This completes the planned PostgreSQL integration for both service transports.
Optional document/cache labs and the final guided exercises remain separate blocks.

References: [gRPC interceptors](https://grpc.io/docs/guides/interceptors/),
[retry behavior](https://grpc.io/docs/guides/retry/), and
[cancellation](https://grpc.io/docs/guides/cancellation/).
