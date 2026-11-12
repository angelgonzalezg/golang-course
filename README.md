# Idiomatic Go Labs

Hands-on lessons and practice projects for learning idiomatic Go, from language
fundamentals to concurrency, REST, and Protobuf/gRPC. The core chapters are
organized around Jon Bodner's
[Learning Go, Second Edition](https://www.oreilly.com/library/view/learning-go-2nd/9781098139285/).

The directories under `chapters/` follow the book's chapter numbers. Lessons
within a chapter introduce the prerequisites used by its practice project.
Most lessons and projects are separate `main` packages, with tests alongside them.
Chapter 10 also introduces library packages and an isolated lab with nested modules.
Chapter 15 uses library packages with executable tests, examples, and benchmarks.
Chapter 16's optional cgo lab is another independent module and needs a C compiler.
The supplementary [Protobuf/gRPC lab](extensions/01-protobuf-and-grpc/README.md)
also has an independent module, with external dependencies kept outside the
main course module.

The [PostgreSQL/database/sql lab](extensions/02-database-sql/README.md) is another
independent module. Its first stage teaches SQL access and real database tests
with Testcontainers; running the demo and integration tests needs a container
runtime. Its unit tests can run without one.

The independent [MongoDB document lab](extensions/03-mongodb/README.md) introduces
the official Go driver, BSON, CRUD, unique indexes, and real MongoDB integration
tests. Its standalone guides include all runtime and test setup.

## Requirements

Use Go 1.27 or later, as declared in `go.mod`. Run the commands below from the
repository root. Start with the
[development environment guide](chapters/01-development-environment/README.md)
for installation, editor setup, your first run and test, and module boundaries.

## Run an example

```sh
go run ./chapters/02-declarations/01-values
go run ./chapters/05-functions/06-project-sales-order
go test -count=1 ./chapters/15-testing/01-unit-tests
```

Change the package path to run another lesson or project from the tables below.
The testing command runs an existing suite; chapter 15 teaches how to write one.

## Learning path

The [learning guide](docs/learning-guide.md) groups the lessons into stages and
explains when to revisit previews and optional deeper material.

1. Start with the small lessons in chapters 2–5: declarations, collections,
   control flow, and functions. In each `main.go`, read the helper definitions
   alongside the numbered examples in `main`.
2. Run an example, predict what will change when you edit one input, and run it
   again. Then try the practice project for that chapter. Projects combine the
   same fundamentals with validation and rules for managing shared data.
3. Follow each lesson's prerequisites before its extensions. Tests can check
   your changes now; chapter 15 explains how to write them.

For chapter 6, use this progression:
[pointer basics](chapters/06-pointers/01-pointers/main.go) →
[small contact lookup practice](chapters/06-pointers/04-project-contact-list/01-pointer-lookup/README.md) →
[complete contact book](chapters/06-pointers/04-project-contact-list/README.md).
Pointer performance and GC are optional deeper study after that route. The GC
pooling example also uses interfaces and concurrency from later chapters.

## Current lessons

| Book chapter | Topics, in reading order |
| --- | --- |
| 1. Setting up your Go environment | [Install Go, prepare your editor, run an example and a test, and understand this repository's modules](chapters/01-development-environment/README.md) |
| 2. Predeclared types and declarations | [Values](chapters/02-declarations/01-values/main.go), [variables](chapters/02-declarations/02-variables/main.go), [constants](chapters/02-declarations/03-constants/main.go) |
| 3. Composite types | [Arrays](chapters/03-composite-types/01-arrays/main.go), [slices](chapters/03-composite-types/02-slices/main.go), [slice internals](chapters/03-composite-types/03-slice-internals/main.go), [strings, bytes, and runes](chapters/03-composite-types/04-strings-bytes-and-runes/README.md), [maps](chapters/03-composite-types/05-maps/main.go), [structs](chapters/03-composite-types/06-structs/main.go) |
| 4. Blocks, shadows, and control structures | [Shadowing](chapters/04-blocks-and-control-flow/01-shadowing/main.go), [conditionals](chapters/04-blocks-and-control-flow/02-conditionals/main.go), [switch](chapters/04-blocks-and-control-flow/03-switch/main.go), [for loops](chapters/04-blocks-and-control-flow/04-for-loops/main.go), [goto](chapters/04-blocks-and-control-flow/05-goto/main.go) |
| 5. Functions | [Functions](chapters/05-functions/01-functions/main.go), [multiple return values](chapters/05-functions/02-multiple-return-values/main.go), [variadic functions](chapters/05-functions/03-variadic-functions/main.go), [function values, closures, and recursion](chapters/05-functions/04-function-values-closures-and-recursion/main.go), [defer](chapters/05-functions/05-defer/main.go) |
| 6. Pointers | [Pointers](chapters/06-pointers/01-pointers/main.go), [contact lookup with values and pointers](chapters/06-pointers/04-project-contact-list/01-pointer-lookup/README.md), [pointer performance](chapters/06-pointers/02-pointer-performance/README.md), [garbage collector](chapters/06-pointers/03-garbage-collector/README.md) |
| 7. Types, methods, and interfaces | [Enums](chapters/07-types-methods-and-interfaces/01-enums/main.go), [methods](chapters/07-types-methods-and-interfaces/02-methods/main.go), [interfaces](chapters/07-types-methods-and-interfaces/03-interfaces/main.go), [Stringer](chapters/07-types-methods-and-interfaces/04-stringer/main.go) |
| 8. Generics | [Generics](chapters/08-generics/01-generics/main.go) |
| 9. Errors | [Error handling](chapters/09-errors/01-error-handling/main.go), [panic and recover](chapters/09-errors/02-panic-and-recover/main.go) |
| 10. Modules, packages, and imports | [Package boundaries, exports, and internal helpers](chapters/10-modules-and-packages/README.md), [dependency versions and local modules](chapters/10-modules-and-packages/02-module-versions/README.md) |
| 11. Go tooling | [Builds and analysis, build tags, embedded resources, and code generation](chapters/11-go-tooling/README.md) |
| 12. Concurrency in Go | [Concurrency fundamentals, ownership, and shutdown](chapters/12-concurrency/01-concurrency/README.md), [fan-out, fan-in, cancellable pipelines, and backpressure](chapters/12-concurrency/02-concurrency-patterns/README.md) |
| 13. The standard library | [I/O, buffering, files, and filesystem interfaces](chapters/13-standard-library/01-io-and-files/README.md), [time, calendars, timers, and tickers](chapters/13-standard-library/02-time/README.md), [JSON, field semantics, and validation](chapters/13-standard-library/03-json/README.md), [HTTP handlers, clients, and shutdown](chapters/13-standard-library/04-http/README.md), [structured logging with slog](chapters/13-standard-library/05-structured-logging/README.md) |
| 14. The context | [Propagation, cancellation, deadlines, request values, and lifecycle](chapters/14-context/01-context/README.md) |
| 15. Writing tests | [Unit tests, subtests, cleanup, doubles, HTTP tests, fuzzing, benchmarks, and races](chapters/15-testing/README.md) |
| 16. Here be dragons: reflect, unsafe, and cgo | [Reflection, unsafe, and an optional cgo lab: runtime metadata, memory layout, and ownership across boundaries](chapters/16-reflect-unsafe-and-cgo/README.md) |

The concurrency lesson introduces `context`; chapter 14 extends it with deadlines,
propagation across API boundaries, request values, and explicit lifetime ownership.
Read that context chapter before the advanced concurrency pattern lesson in chapter 12.

## Practice projects

| Project | Book chapter | Example |
| --- | --- | --- |
| Sales order processor | 5. Functions | [Run after the function lessons](chapters/05-functions/06-project-sales-order/README.md) |
| Contact book | 6. Pointers | [Start with a small lookup](chapters/06-pointers/04-project-contact-list/01-pointer-lookup/README.md), then [build the complete book](chapters/06-pointers/04-project-contact-list/README.md) |
| Log levels | 7. Types, methods, and interfaces | [Run after Stringer](chapters/07-types-methods-and-interfaces/05-project-log-levels/README.md) |
| Math helpers with errors | 9. Errors | [Run after error handling](chapters/09-errors/03-project-math-helpers/README.md) |
| REST fundamentals | 13. The standard library | [Contacts API using in-memory storage](chapters/13-standard-library/06-project-rest-api/README.md); revisit after chapters 14–15. |
| Protobuf and gRPC contacts lab | Supplement after REST | [Contract and serialization foundation](extensions/01-protobuf-and-grpc/README.md), then [unary client/server application](extensions/01-protobuf-and-grpc/02-unary/README.md). |
| PostgreSQL and database/sql lab | Supplement after context and testing | [SQL fundamentals and Testcontainers](extensions/02-database-sql/README.md), [transactions and migrations](extensions/02-database-sql/02-transactions/README.md), [durable idempotency](extensions/02-database-sql/03-idempotency/README.md), then [sqlc comparison](extensions/02-database-sql/04-sqlc/README.md). |
| REST with PostgreSQL | Supplement after REST fundamentals and the SQL lessons | [Persistent contacts, atomic conditional writes, and durable keyed POST](extensions/02-database-sql/05-rest-postgres/README.md). |
| gRPC with PostgreSQL | Supplement after the gRPC and SQL lessons | [Persistent unary/streaming RPCs and durable keyed creation](extensions/01-protobuf-and-grpc/06-postgres/README.md). |
| MongoDB documents | Supplement after context and testing; compare with SQL fundamentals | [Fundamentals](extensions/03-mongodb/01-fundamentals/README.md), then [embedded documents, queries, projections, and nested updates](extensions/03-mongodb/02-document-modeling/README.md). |

## Validate the course

The [Go validation workflow](.github/workflows/go.yml) runs on pushes and pull
requests, and can also be started manually from GitHub Actions. It checks Go
formatting, tests, vet, and module metadata for all eight modules, with separate
race-detector, PostgreSQL integration, and MongoDB integration jobs. See [continuous integration](docs/validation.md#continuous-integration)
for its scope and how to reproduce the checks locally.

```sh
go test -count=1 -timeout=60s ./...
go vet ./...
gofmt -l chapters extensions
```

Use the [validation guide](docs/validation.md) for the complete procedure:
all independent modules, executable lessons, build tags, generated source,
race checks, fuzzing, benchmarks, unsafe instrumentation, and the optional cgo lab.

The root commands cover the main module. Chapter 10's three versioning modules
and chapter 16's cgo module have their own boundaries and explicit commands.
The Protobuf/gRPC and PostgreSQL labs also use explicit `go -C` commands from
their guides.
The main course also passes with `CGO_ENABLED=0`; the optional cgo lab needs a
C compiler.

## Review status

The original content updates and final review are complete. The
[review record](docs/validation.md#review-record) documents the checked toolchain,
original-lesson continuity, program inventory, results, and limits.

Individual "Further practice" sections describe optional extensions for the
reader. Use the [learning guide](docs/learning-guide.md) to choose a stage and
check your understanding before adding those extensions.

## Backend extensions

The [REST fundamentals](chapters/13-standard-library/06-project-rest-api/README.md) project
extends the HTTP lesson with resource URLs, CRUD operations, PUT versus PATCH,
paged and filtered collections, problem responses, and a reusable contact store.
It includes conditional requests with ETag and a served, checked OpenAPI
contract. Contacts live in process memory and disappear when the server stops.
Its default demo is finite; use `-serve` for interactive requests.
The agreed REST extension blocks are complete. The
[Protobuf/gRPC lab](extensions/01-protobuf-and-grpc/README.md) starts with a
versioned contact contract, generated bindings, local serialization examples,
and compatibility tests. Its unary stage implements the client/server transport
using the same contact rules, with status errors, deadlines, cancellation, and
integration tests. Its streaming stage adds ordered result streams, bounded
imports with partial commits, bidirectional lookup outcomes, and lifecycle
tests. Its infrastructure stage adds verified TLS, metadata/interceptors,
reader/writer access, health, reflection, and grpcurl exercises. Its resilience
stage adds message limits, bounded retries, keyed creation, and OpenTelemetry.
The five original gRPC stages and their [review](extensions/01-protobuf-and-grpc/05-resilience/review.md)
are complete. [gRPC with PostgreSQL](extensions/01-protobuf-and-grpc/06-postgres/README.md)
adds persistence after the SQL lessons. This lab is supplementary to the book's chapter mapping.

The [SQL lab](extensions/02-database-sql/README.md) begins the persistence
extension with standalone fundamentals, transactions/migrations,
[durable idempotency](extensions/02-database-sql/03-idempotency/README.md), and a
[manual/sqlc comparison](extensions/02-database-sql/04-sqlc/README.md), all covered
by real PostgreSQL integration tests. The
[REST with PostgreSQL](extensions/02-database-sql/05-rest-postgres/README.md) project
adds database-backed HTTP operations and durable keyed POST. The persistent
gRPC lesson uses the same SQL adapter and retained creation results. The earlier
in-memory lessons remain available for learning the underlying concepts.
The REST learning path is **REST fundamentals → SQL lessons → REST with PostgreSQL**.
Both REST projects share the contact rules and HTTP adapter.

The [MongoDB lab](extensions/03-mongodb/README.md) adds document persistence in a
separate module. Its first stage covers ObjectID/BSON, parameter-like typed
filters, bounded cursors, `$set`/`$unset`, a unique index, and client/context
ownership. Unit tests check BSON and validation without a database; tagged
Testcontainers tests use a real authenticated MongoDB server. Its
[second stage](extensions/03-mongodb/02-document-modeling/README.md) embeds
addresses/tags, queries city and array values, projects summaries, and updates
one nested field while preserving its neighbors. Further aggregation/schema
evolution and caching remain separate blocks, with guided exercises last.
