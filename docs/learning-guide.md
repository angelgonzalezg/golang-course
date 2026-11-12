# Learning guide

The chapter numbers organize selected examples around *Learning Go, Second
Edition*. Use the route below to follow dependencies while keeping a first pass
focused on fundamentals. The [main index](../README.md) lists every lesson.

## Read, predict, run, and change

Read a small helper alongside the numbered section of `main` that calls it.
Predict its output, run the package, then change one input and explain the
result. Once that behavior is familiar, apply it in the chapter's practice
project. Projects add validation and rules for managing data; each builds on
the smaller examples.

Most lessons are independent commands, run from the repository root with
`go run ./chapters/...`. Run package paths so companion files are included.
The testing chapter uses library packages and `go test`. The module-versioning
and optional cgo labs have their own `go.mod` files and use `go -C`.

## Suggested route

| Stage | Read and practice | Check your understanding |
| --- | --- | --- |
| 0. Prepare your environment | [Development environment](../chapters/01-development-environment/README.md). | Confirm the Go version and active module, run one lesson and one test, and explain why independent modules use separate commands. |
| 1. Values and control flow | Chapters 2–4 in the [index](../README.md). | Explain zero values, `len` versus `cap`, map lookup with `ok`, byte versus rune, and when a slice shares storage. |
| 2. Functions | Chapter 5, then the [sales order project](../chapters/05-functions/06-project-sales-order/README.md). | Trace inputs, returned values, early returns, closures, and deferred work. Distinguish a valid zero result from a failed lookup. |
| 3. Pointers and ownership | [Pointer basics](../chapters/06-pointers/01-pointers/main.go), [small contact lookup](../chapters/06-pointers/04-project-contact-list/01-pointer-lookup/README.md), then the [contact book](../chapters/06-pointers/04-project-contact-list/README.md). | Explain which object changes, when a returned pointer aliases storage, and why the expanded book uses snapshots and stable IDs. |
| 4. Types and reusable behavior | Chapters 7–9: methods, interfaces, generics, and errors. Complete [log levels](../chapters/07-types-methods-and-interfaces/05-project-log-levels/README.md) and [math helpers](../chapters/09-errors/03-project-math-helpers/README.md). | Explain value/pointer method sets, choose an interface or constraint, and inspect wrapped errors with `errors.Is` and `errors.As`. |
| 5. Package and build boundaries | [Modules and packages](../chapters/10-modules-and-packages/README.md), the [versioning lab](../chapters/10-modules-and-packages/02-module-versions/README.md), and [Go tooling](../chapters/11-go-tooling/README.md). | Explain exports, `internal`, module replacements, build tags, embedding, and when generated source is refreshed. |
| 6. Concurrent work and APIs | [Concurrency fundamentals](../chapters/12-concurrency/01-concurrency/README.md), then chapter 13's I/O, time, JSON, HTTP, and logging lessons. | Identify shared state, channel/close ownership, resource cleanup, API validation, and caller-controlled time budgets. |
| 7. Cancellation across stages | [Context](../chapters/14-context/01-context/README.md), then return to [concurrency patterns](../chapters/12-concurrency/02-concurrency-patterns/README.md). | Trace fan-out, fan-in, bounded queues, failure propagation, cancellation, and joining every started stage. |
| 8. Testing techniques | [Writing tests](../chapters/15-testing/README.md). Start its unit-test examples after functions and errors; return for HTTP, concurrency, and fuzzing prerequisites. | Choose observable contracts, isolated fixtures, doubles, generated-input properties, and meaningful concurrent checks. |
| 9. Measurement and GC | Revisit [pointer performance](../chapters/06-pointers/02-pointer-performance/README.md) and [GC](../chapters/06-pointers/03-garbage-collector/README.md). | Separate copying from allocations, explain reachability and reuse, and describe exactly what each benchmark measures. |
| 10. Advanced boundaries | [Reflection, unsafe, and optional cgo](../chapters/16-reflect-unsafe-and-cgo/README.md). | Justify runtime inspection or memory access, document lifetimes and bounds, and name the owner of each foreign allocation. |

The optional cgo lab needs a C compiler. The rest of the course can be tested
with `CGO_ENABLED=0`.

After stages 6–8, complete the
[REST fundamentals](../chapters/13-standard-library/06-project-rest-api/README.md).
This first REST project keeps contacts in process memory; restarting the server
starts an empty collection. After the SQL lessons, continue to
[REST with PostgreSQL](../extensions/02-database-sql/05-rest-postgres/README.md)
for persistent data and durable keyed creation.
Use its [learning passes](../chapters/13-standard-library/06-project-rest-api/README.md#learn-in-passes):
trace one create/read before moving to updates, failures, collection queries,
conditions, and OpenAPI. Trace a request from HTTP to the contact store and back.
Explain POST's Location, PUT's full replacement versus PATCH's selected changes,
omitted fields versus null, idempotency, and how shared data stays consistent
under simultaneous requests. The earlier pointer project remains the
introductory contact exercise; this one combines later concepts. Continue with
[collection queries](../chapters/13-standard-library/06-project-rest-api/collection-queries.md):
filter before counting and paging, follow returned links, and explain how a
mutation between requests can move results. Then read
[conditional requests](../chapters/13-standard-library/06-project-rest-api/conditional-requests.md)
and the [OpenAPI contract](../chapters/13-standard-library/06-project-rest-api/openapi.md).
Explain the difference between 304 and 412, why compare-and-write must be
atomic, and which runtime constraints go beyond a schema. These REST blocks
are complete. Continue with the supplementary
[Protobuf/gRPC lab](../extensions/01-protobuf-and-grpc/README.md): start with the
[foundation lesson](../extensions/01-protobuf-and-grpc/01-protobuf/README.md).
Follow messages to bytes and back, then optional presence, field masks, and
independent clones. Its guide leads into compatibility and generation after
those basic examples. Then run the
[unary application](../extensions/01-protobuf-and-grpc/02-unary/README.md), trace
RPC-to-domain conversion, and explain status details, channel/context ownership,
atomic versions, and shutdown with an active call. Continue with
[streaming](../extensions/01-protobuf-and-grpc/03-streaming/README.md): distinguish
request half-close from final status, partial import commits from rollback, and
item failures from terminal errors. The [streaming passes](../extensions/01-protobuf-and-grpc/03-streaming/README.md#learn-in-passes)
introduce server messages first, then an import summary, then the two-way exchange.
Trace concurrent sending/receiving and sender
cleanup after cancellation. Then run the
[infrastructure stage](../extensions/01-protobuf-and-grpc/04-infrastructure/README.md)
and trace transport credentials, call credentials, interceptor order, identity
context, and header/trailer ownership. Compare authentication with authorization,
readiness with connectivity, and reflection access with method access. Finish with
[resilience](../extensions/01-protobuf-and-grpc/05-resilience/README.md): explain
safe retries, payload-bound keys, partial stream failure, and transport versus
handler observations. Inspect metrics, trace propagation, and correlated logs.
Use the [stage-4 passes](../extensions/01-protobuf-and-grpc/04-infrastructure/README.md#learn-in-passes)
to follow one permitted/rejected call before setting up separate processes.
Then use the [stage-5 passes](../extensions/01-protobuf-and-grpc/05-resilience/README.md#learn-in-passes):
first understand the retry and saved-result examples, then message limits,
and finally logs/metrics/traces. Each guide defines its new terms beside the example.
The lab uses its own module; all five stages and the
[final review](../extensions/01-protobuf-and-grpc/05-resilience/review.md) are complete.

For persistence, continue to the independent
[PostgreSQL/database/sql lab](../extensions/02-database-sql/README.md) after
context and testing. Its first stage follows a parameterized insert through
`Scan`, then bounded result sets, NULL/empty values, error causes, and pool-wait
cancellation. Run the integration-tagged tests to observe a real PostgreSQL
container with owned cleanup. Compare repeated email assignments and deletion
with unique-name rejection; the fundamentals store has no request-key records.
Then run [transactions and migrations](../extensions/02-database-sql/02-transactions/README.md):
trace a batch commit and rollback, the common commit of a contact and its
history, and the rejection of a stale version. Then study a `golang-migrate`
schema upgrade and its dirty-state handling. Migration recovery and adoption
of the previous runner's history are optional operations. The cancellation test
observes the server's lock wait before canceling the writer. Next, follow
[durable idempotency](../extensions/02-database-sql/03-idempotency/README.md):
bind a key to a caller and the original fields, commit the contact and result
together, then recover that snapshot through concurrent retries and a new
process. Then [compare manual and sqlc queries](../extensions/02-database-sql/04-sqlc/README.md):
trace named SQL into generated parameters/scanning, follow the adapter's shared
validation and error rules, and bind queries to a caller-owned transaction.
Run the shared PostgreSQL contracts and generation check. Then follow
[REST with PostgreSQL](../extensions/02-database-sql/05-rest-postgres/README.md): keep
the HTTP behavior, lock rows while checking validators, and bind keyed POST to
a trusted caller and a saved creation snapshot. Test replay after deletion and
across processes. PostgreSQL integration for gRPC follows later. The lab's
[stage plan](../extensions/02-database-sql/README.md)
separates implemented behavior from those future blocks.

## First pass through pointers

Sections 1–5 of the pointer lesson cover address/dereference, nil, `new`,
parameters, and structs. Sections 8–10 connect those mechanics to maps, slices,
and the final choice rule. Sections 6–7 preview interface destinations and JSON
presence; revisit them after the interface and JSON lessons.

The small contact practice keeps the caller's slice structure stable while a
pointer is used. The full book adds a name index, unique normalized names,
stable IDs, and operations that maintain those relationships. These are two
editing contracts to compare. Copying `Contact` provides independent fields
because its fields are scalar values and strings.

For other models, copying a struct can still share referenced slices or maps.
A read-only operation may use a value or pointer receiver. Shared identity,
mutation, noncopyable state, copying costs, and method-set requirements guide
that choice. See the [method examples](../chapters/07-types-methods-and-interfaces/02-methods/main.go)
and the official [receiver guidance](https://go.dev/wiki/CodeReviewComments#receiver-type).

## Toolchain and API versions

Use the Go version declared by the module, currently Go 1.27. The examples use
current APIs while preserving the underlying language concepts. In particular:

- The pointer lesson uses `encoding/json` for destination and optional-value
  examples. Chapter 13 compares it with `encoding/json/v2` and `jsontext`.
- Benchmarks use `testing.B.Loop`; the testing chapter explains its timing model.
- Time and cancellation tests use `testing/synctest` to control virtual time.
- Reflection uses `reflect.TypeFor` to obtain type metadata directly.

Consult the [Go 1.27 release notes](https://go.dev/doc/go1.27) and each lesson's
references when comparing API spelling or behavior with another Go version.
The content is a selection of language and library topics; further-practice
items invite extensions beyond the implemented examples.

## Decide when to continue

Before moving on, explain one successful path and one rejected or missing-input
path. For code that shares data, identify who owns it and who may mutate it.
For concurrent code, explain how each goroutine finishes and who waits for it.
For performance claims, identify the workload, measurement, and limitations.

Use the [validation guide](validation.md) to check changes, including the
separate modules and optional instrumentation.
