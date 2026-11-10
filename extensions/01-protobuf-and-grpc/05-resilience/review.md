# Completed gRPC lab: review and validation

The five agreed stages form one progression. The core contact rules still live
in the course's REST project; gRPC adds a separate transport without moving
those rules or bringing external libraries into the root module.

| Stage | Concepts and executable | What to inspect |
| --- | --- | --- |
| 1. [Protobuf](../README.md#read-in-order) | Field numbers, presence, masks, binary/ProtoJSON, cloning, compatibility. | `01-protobuf`, versioned contract, checked-in generated code, reproducible generation. |
| 2. [Unary](../02-unary/README.md) | Six CRUD/query RPCs, statuses/details, versions, contexts, connection ownership. | Shared-store adapter, bufconn/TCP tests, REST interoperability, server lifecycle. |
| 3. [Streaming](../03-streaming/README.md) | Server/client/bidirectional shapes, item outcomes, partial commits, flow control. | Ordered page snapshot, bounded import, one sender/receiver, EOF/half-close, cancellation. |
| 4. [Infrastructure](../04-infrastructure/README.md) | TLS 1.3, call credentials, roles, metadata/interceptors/logs, health/reflection. | Certificate verification, protected methods, Watch shutdown, opt-in reflection, grpcurl. |
| 5. [Resilience](README.md) | Byte limits, bounded retries, keyed creation, OpenTelemetry and failure tests. | Concurrent deduplication, cache capacity, partial stream failures, transport metrics, trace propagation. |

## Execute all five examples

From the repository root:

```sh
go -C extensions/01-protobuf-and-grpc run ./01-protobuf
go -C extensions/01-protobuf-and-grpc run ./02-unary
go -C extensions/01-protobuf-and-grpc run ./03-streaming
go -C extensions/01-protobuf-and-grpc run ./04-infrastructure
go -C extensions/01-protobuf-and-grpc run ./05-resilience
```

Each command terminates automatically. Stages 2–5 use real local TCP calls;
stages 4–5 verify local TLS. Their expected error cases are assertions in the
demo, so an expected status does not make the program fail. Separate-terminal
server/client and grpcurl instructions remain in stages 2–4.

## Validate the module

```sh
go -C extensions/01-protobuf-and-grpc test -count=1 -timeout=30s ./...
go -C extensions/01-protobuf-and-grpc test -race -count=3 -cpu=1,2 -timeout=45s ./...
go -C extensions/01-protobuf-and-grpc vet ./...
go -C extensions/01-protobuf-and-grpc mod tidy -diff
gofmt -l extensions/01-protobuf-and-grpc
git diff --check
```

`ok` marks packages whose tests passed. `[no test files]` is expected for the
module's generation entry point, generated bindings, and generator command;
integration tests exercise generated types and clients through the service.
The source generator has its separate reproducibility check below.

The tests verify behavior through bufconn or loopback TLS/TCP, including real
serialization, statuses, cancellation, and transport limits. Controlled callbacks
and channels expose concurrency/cancellation behavior; repeated race checks
cover executed schedules rather than proving every possible interleaving.
The resilience fixture simulates a lost reply by returning an error after a
successful create/cache result, rather than dropping network packets.

With the [recorded protoc and includes](../README.md#regenerate-reproducibly):

```sh
go -C extensions/01-protobuf-and-grpc run ./internal/cmd/protogen -check
```

Ordinary runs/tests do not need protoc because generated files are checked in.
The two generated files must match the pinned compiler/plugins byte for byte.
Stage 5 adds SDK dependencies without changing those tools or the wire contract.

The root module's checks are independent; `./...` does not cross nested module
boundaries. The main course validation remains:

```sh
go test -count=1 -timeout=60s ./...
go vet ./...
```

See the [course validation guide](../../../docs/validation.md) for its other
independent modules, tags, fuzzing, benchmarks, and optional cgo checks. This
gRPC review does not replace those separate workflows.

## Completed scope and further practice

All five agreed gRPC stages are implemented. Their limits are explicit:

- The contact store and idempotency records are in memory. Stage-5 cache entries
  are bounded, retained for process lifetime, and not shared across replicas.
  Durable business writes and deduplication need a common atomic persistence policy.
- The typed retry helper covers reads and keyed creation, with three application
  attempts inside one two-second budget. Other writes and streams have no
  application replay. Transparent gRPC retries remain possible before dispatch.
- Authentication uses two static teaching identities over verified server TLS.
  Managed identity, token lifecycle, tenant authorization, and mutual TLS can be
  developed as separate exercises.
- Local OpenTelemetry collection demonstrates real propagation and metrics,
  with a bounded span-summary exporter. Collectors, production sampling,
  dashboards, alerting, and deployment-specific retention are further practice.
- Streaming is bounded at the application/message boundaries shown. It does not
  implement live subscriptions to contact changes, resumable imports, or a
  database cursor over an unbounded dataset.

Persistence, load balancing, gateways, deployment, and production identity
integration remain optional extensions beyond this completed course lab.
