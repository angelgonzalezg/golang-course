# Validate the course

Run these commands from the repository root with Go 1.27. The commands are
grouped by what they verify; ordinary tests, race instrumentation, fuzzing, and
benchmarks exercise different properties. The optional cgo group also requires
a matching C compiler and `CGO_ENABLED=1`.

## Continuous integration

The [Go validation workflow](../.github/workflows/go.yml) is a GitHub Actions
configuration. A **workflow** starts on an event, a **job** runs checks on a
runner machine, and a **step** performs one operation inside that job. Each
push and pull request starts these checks. Once the workflow is on the default
branch, you can also select **Actions → Go validation → Run workflow** in GitHub.

The workflow uses Ubuntu 24.04 and reads the Go version from the module files.
Its module matrix creates one job for each of the seven `go.mod` boundaries;
one module's failure does not cancel the other module jobs.

| Job | Checks |
| --- | --- |
| Go formatting | Runs `gofmt -l chapters extensions` and fails if any file is listed. |
| Tests and vet, per module | Runs uncached tests, `go vet`, and `go mod tidy -diff` for the course, all three versioning modules, cgo, gRPC, and SQL. |
| Course job, extra steps | Checks the `diagnostic` build and confirms that chapter 11 generation reproduces the committed file. |
| SQL job, extra step | Uses the pinned sqlc generator to compare expected bindings with the checked-in Go files. |
| Race detector, all modules | Runs each module's tests with `-race`, separately from ordinary tests. |
| PostgreSQL integration | Runs the SQL lab with `-tags=integration -race`, using Testcontainers and a real PostgreSQL server. |

Ordinary jobs use `CGO_ENABLED=0`, except for the cgo lab's job. The race job uses
`CGO_ENABLED=1` with the runner's C compiler. The cgo lab remains optional for
learners, while CI includes it. Actions are pinned to full commit IDs, with their
release versions noted beside them. Go dependencies are cached for the gRPC and
SQL jobs using their checked-in `go.sum` files. Ordinary SQL tests do not need
Docker; the dedicated integration job requires a working engine and fails if
database startup fails.

Open a failed job and then the failed step to find the command and its output.
A formatting failure lists the files to format; a test failure identifies the
package and test. `mod tidy -diff` reports metadata differences and fails without
rewriting the module files. Apply an intentional dependency update locally and
review its diff before committing it.

### Reproduce a module job locally

Run from the repository root in a POSIX-compatible shell. Start with the course:

```sh
module_dir=.
CGO_ENABLED=0 go -C "$module_dir" test -count=1 -timeout=60s ./...
CGO_ENABLED=0 go -C "$module_dir" vet ./...
CGO_ENABLED=0 go -C "$module_dir" mod tidy -diff
```

Set `module_dir` to one of the module paths in the workflow to check that lab.
Use `CGO_ENABLED=1` for cgo. To reproduce a race check, use a supported platform
with a matching C compiler:

```sh
CGO_ENABLED=1 go -C "$module_dir" test -race -count=1 -timeout=90s ./...
```

For the format gate, `gofmt -l chapters extensions` must print nothing. Run the
commands under [alternate builds and generated source](#alternate-builds-and-generated-source)
to reproduce the course job's extra steps. The workflow's Bash snippets can also
be copied directly into a Bash terminal at the repository root.

CI provides this repeatable baseline on Linux. The specialized checks later in
this guide also cover executable lessons, active fuzzing, benchmarks, checkptr,
cgo pointer instrumentation, Protobuf regeneration with the pinned compiler,
and the optional live OpenAPI validator. Those remain explicit local checks;
ordinary CI tests already run fuzz seeds and exercise the REST and gRPC demos
through their tests. A green CI run describes the executed checks on that runner.

When adding another module, update both the ordinary-test matrix and the race
job's module list. After editing the workflow, check its YAML and GitHub Actions
expressions with the pinned actionlint tool:

```sh
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/go.yml
```

This command may download the tool and its dependencies into Go's cache; the
versioned invocation does not add it to the course's `go.mod`. Consult the official
[workflow syntax](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax)
and [setup-go inputs](https://github.com/actions/setup-go) when changing the CI
configuration.

## PostgreSQL/SQL module

The [SQL lab](../extensions/02-database-sql/README.md) is the seventh module.
Its first four stages use pgx through `database/sql` for fundamentals,
transactions/migrations, durable idempotency, and a manual/sqlc comparison.
Ordinary checks need no running database:

```sh
go -C extensions/02-database-sql test -count=1 -timeout=60s ./...
go -C extensions/02-database-sql vet ./...
go -C extensions/02-database-sql mod tidy -diff
go -C extensions/02-database-sql test -race -count=1 -timeout=90s ./...
```

Check generated bindings using the same command as CI:

```sh
go -C extensions/02-database-sql run ./internal/cmd/sqlgen -check
```

The wrapper runs pinned sqlc v1.31.1's `diff` command without rewriting source.
It needs no database; the first run can download/build the generator separately
from the SQL module's dependencies. Regenerate with
`go -C extensions/02-database-sql generate .` after changing schema/query inputs.
See the [comparison guide](../extensions/02-database-sql/04-sqlc/README.md).

With a Docker-compatible engine running, explicitly execute the real database
tests (the race command also needs a C compiler):

```sh
docker info
go -C extensions/02-database-sql test -tags=integration -count=1 -timeout=5m -v ./...
go -C extensions/02-database-sql test -tags=integration -race -count=1 -timeout=5m ./...
```

These tests create and remove their own PostgreSQL container; they do not use
`DATABASE_URL` or a learner's existing database. Without the `integration` tag,
that package is excluded. With the tag, an unavailable engine fails the test.
Repeated email assignments are checked with nonempty text, empty text, and NULL:
the contact identity and stored values remain stable and no extra row appears.
Repeated deletion and concurrent name uniqueness are also covered. These
fundamentals contracts run against both the manual and sqlc-backed stores,
including shared validation/error causes and independent optional result values.
Generated query transaction binding and the comparison demo are also tested. Those
fundamentals differ from durable request-key replay; see the lab's
[idempotency explanation](../extensions/02-database-sql/README.md#idempotency-state-uniqueness-and-request-keys)
and [implemented durable lesson](../extensions/02-database-sql/03-idempotency/README.md).
The transaction suite also checks upgrade preservation, repeated/concurrent
migrations with `golang-migrate`, dirty/future state rejection, per-file migration
rollback, explicit legacy adoption, isolated down/up cycles, atomic batch
inserts, contact/history atomicity, stale writers, and cancellation after an
UPDATE has run. The durable-idempotency suite adds caller/key/payload binding,
concurrent retries across pools, business/result rollback, cancellation while
waiting and after insertion, historical replay after edits/deletion, and
lost-reply recovery in a second OS process. Each top-level suite owns a separate
PostgreSQL container; test-only table resets are confined to that container.
See the lab guide for Compose/demo commands and Rancher Desktop configuration.

## Root module and formatting

```sh
go test -count=1 -timeout=60s ./...
go vet ./...
gofmt -l chapters extensions
```

Formatting should list no files. `-count=1` requests an uncached test run. Tests
also execute documentation examples and fuzz seeds; active fuzzing needs its
own command below. Root commands stop at nested `go.mod` boundaries.

Confirm the main course also works without cgo:

```sh
CGO_ENABLED=0 go test -count=1 -timeout=60s ./...
```

## Independent module-versioning lab

The three modules use local replacements and need no dependency downloads:

```sh
go -C chapters/10-modules-and-packages/02-module-versions/app test -count=1 ./...
go -C chapters/10-modules-and-packages/02-module-versions/greeting-v1 test -count=1 ./...
go -C chapters/10-modules-and-packages/02-module-versions/greeting-v2 test -count=1 ./...
go -C chapters/10-modules-and-packages/02-module-versions/app vet ./...
go -C chapters/10-modules-and-packages/02-module-versions/greeting-v1 vet ./...
go -C chapters/10-modules-and-packages/02-module-versions/greeting-v2 vet ./...
go -C chapters/10-modules-and-packages/02-module-versions/app mod tidy -diff
go -C chapters/10-modules-and-packages/02-module-versions/greeting-v1 mod tidy -diff
go -C chapters/10-modules-and-packages/02-module-versions/greeting-v2 mod tidy -diff
go -C chapters/10-modules-and-packages/02-module-versions/app run .
```

`mod tidy -diff` checks metadata without applying changes. An empty diff is the
expected result for unchanged examples. See the [module lab](../chapters/10-modules-and-packages/02-module-versions/README.md).

## Supplementary Protobuf/gRPC module

The [gRPC lab](../extensions/01-protobuf-and-grpc/README.md) is another independent
module. Its first stage runs local serialization examples; its second stage
implements the unary transport, its third adds three streaming call shapes,
its fourth adds TLS, metadata/interceptors, access control, and standard services,
and its fifth adds resilience, OpenTelemetry, and final review.
Initial use downloads its pinned Go
dependencies, kept outside the main course module:

```sh
go -C extensions/01-protobuf-and-grpc run ./01-protobuf
go -C extensions/01-protobuf-and-grpc run ./02-unary
go -C extensions/01-protobuf-and-grpc run ./03-streaming
go -C extensions/01-protobuf-and-grpc run ./04-infrastructure
go -C extensions/01-protobuf-and-grpc run ./05-resilience
go -C extensions/01-protobuf-and-grpc test -count=1 -timeout=30s ./...
go -C extensions/01-protobuf-and-grpc vet ./...
go -C extensions/01-protobuf-and-grpc mod tidy -diff
go -C extensions/01-protobuf-and-grpc test -race -count=3 -cpu=1,2 -timeout=45s ./...
```

Generated Go files are checked in, so ordinary runs/tests need no protoc.
To verify regeneration, install the lab's recorded protoc 35.1 with its standard
include files, then run:

```sh
go -C extensions/01-protobuf-and-grpc generate .
go -C extensions/01-protobuf-and-grpc run ./internal/cmd/protogen -check
```

The check compares newly generated files in temporary storage with the
checked-in outputs. The Go plugins are module tool dependencies; their versions
are pinned in the lab's `go.mod`. Its README documents custom include layouts.

## Alternate builds and generated source

```sh
go test -count=1 -tags=diagnostic ./chapters/11-go-tooling/...
go vet -tags=diagnostic ./chapters/11-go-tooling/...
go run -tags=diagnostic ./chapters/11-go-tooling/02-build-tags
go generate ./chapters/11-go-tooling/04-go-generate
git diff --exit-code -- chapters/11-go-tooling/04-go-generate/status_names_gen.go
```

The tagged run should print `build mode: diagnostic`. Generation refreshes the
tracked output; with unchanged input and generator, its bytes and Git diff
should remain unchanged. Review intentional changes to that file separately.
See [Go tooling](../chapters/11-go-tooling/README.md) for building, inspecting,
installing, and cross-compiling demonstration binaries in a temporary directory.

## Execute the lessons

Run package paths from the [index](../README.md), for example:

```sh
go run ./chapters/02-declarations/01-values
go run ./chapters/06-pointers/04-project-contact-list/01-pointer-lookup
```

For a bulk pass in a POSIX-compatible shell:

```sh
lesson_packages=$(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./...) || exit
for lesson_package in $lesson_packages; do
    case "$lesson_package" in */internal/cmd/*) continue ;; esac
    go run "$lesson_package" || exit
done
```

The internal generator command is checked through `go generate` and its tests.
Chapter 15's library packages run through `go test`. The independent command
modules use the explicit `go -C` run commands in this guide. Lessons run
finite demonstrations, including local HTTP servers that shut down themselves;
I/O examples own temporary storage and cleanup.

The [REST project](../chapters/13-standard-library/06-project-rest-api/README.md)
also runs a finite demo by default and is included in the bulk pass. Its
optional `-serve` flag keeps a server running for manual requests; omit that
flag in automated validation. To validate this extension on its own:

```sh
go run ./chapters/13-standard-library/06-project-rest-api
go test -count=1 -timeout=30s ./chapters/13-standard-library/06-project-rest-api/...
go test -race -count=3 -cpu=1,2 -timeout=30s ./chapters/13-standard-library/06-project-rest-api/...
go vet ./chapters/13-standard-library/06-project-rest-api/...
```

## Race detector

On a platform supported by Go's race detector:

```sh
go test -race -count=1 -timeout=60s ./...
go run -race ./chapters/12-concurrency/01-concurrency
go run -race ./chapters/12-concurrency/02-concurrency-patterns
```

Race checks cover executed paths. Tests use explicit coordination and virtual
time where appropriate; a successful run does not cover every possible schedule.
For an investigation, the concurrency, HTTP, and context lesson guides also
show targeted repetitions and CPU-count variations.

## Active fuzzing

```sh
go test -fuzz='^FuzzNormalize$' -fuzztime=5s -parallel=2 ./chapters/15-testing/04-fuzzing
```

This is a bounded exploration of additional inputs. Keep a minimized failing
input under `testdata/fuzz` after fixing a discovered bug. Ordinary tests run
checked-in seeds; generated coverage inputs may also live in Go's fuzz cache.
See the [testing chapter](../chapters/15-testing/README.md) for properties,
coverage reports, and reproducing individual failures.

## Unsafe pointer checks

```sh
go test -count=1 -gcflags=all=-d=checkptr=2 ./chapters/16-reflect-unsafe-and-cgo/02-unsafe
go run -gcflags=all=-d=checkptr=2 ./chapters/16-reflect-unsafe-and-cgo/02-unsafe
```

This instrumentation changes allocation behavior. A passing run complements
review of allocation bounds, alignment, ownership, and lifetimes. See the
[unsafe lesson](../chapters/16-reflect-unsafe-and-cgo/02-unsafe/README.md).

## Optional cgo module

```sh
CGO_ENABLED=1 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo test -count=1 ./...
CGO_ENABLED=1 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo vet ./...
CGO_ENABLED=1 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo mod tidy -diff
CGO_ENABLED=1 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo run .
CGO_ENABLED=1 GOEXPERIMENT=cgocheck2 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo test -count=1 ./...
CGO_ENABLED=1 GOEXPERIMENT=cgocheck2 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo run .
CGO_ENABLED=1 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo test -race -count=1 -timeout=30s ./...
```

The ordinary API tests compile and call the C implementation. Stronger cgo
pointer checks supplement the documented borrowing and cleanup contracts.
Go's race detector does not instrument this lab's C source for C memory races,
and these checks do not prove absence of native leaks. See the
[cgo guide](../chapters/16-reflect-unsafe-and-cgo/03-cgo/README.md) for prerequisites,
ownership, conversion limits, and the tests' allocation-failure limits.

## Short benchmark samples

Run this group after other CPU-intensive checks finish, with normal compiler
settings and without race, checkptr, or cgocheck2 instrumentation:

```sh
go test -p=1 -run='^$' -bench=. -benchmem -benchtime=100ms -count=3 ./chapters/06-pointers/02-pointer-performance ./chapters/06-pointers/03-garbage-collector ./chapters/15-testing/05-benchmarks-and-races ./chapters/16-reflect-unsafe-and-cgo/02-unsafe
CGO_ENABLED=1 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo test -run='^$' -bench=. -benchmem -benchtime=100ms -count=3 ./native
```

These samples check that the benchmark workloads run and report meaningful
metrics. Timing and allocation results are machine- and compiler-dependent;
tests do not assert fixed performance thresholds. Read each lesson's ownership
contract before comparing its results with another workload.

## Review record

The final review on **2026-11-09** used **Go 1.27.0, darwin/arm64**, with
Apple clang 21.0.0 available for the optional cgo module. It checked the content
through `abe5818`, together with this review's documentation and comment
clarifications. The agreed improvement sequence is complete.

This snapshot predates the later REST project and supplementary Protobuf/gRPC
lab. Its package and program counts describe the reviewed baseline, not those
additions.

| Area | Result |
| --- | --- |
| Main module | All 57 packages passed uncached tests and vet; test suites also passed with the race detector and with cgo disabled. |
| Runnable lessons | All 49 root lesson commands exited successfully, as did the independent versioning app and cgo app: 51 learner programs in total. |
| Independent modules | All four passed tests, vet, and `mod tidy -diff`; metadata required no changes. |
| Tooling | Standard and diagnostic configurations passed; regeneration reproduced the tracked file exactly. Builds, linker version metadata, embedding outside the source directory, and installation to a temporary `GOBIN` worked. |
| Cross-build | The tooling example built for Linux/amd64 with cgo disabled; its metadata was inspected. That foreign binary was not executed. |
| Concurrent demonstrations | Both concurrency programs completed under the race detector. |
| Unsafe and cgo | Unsafe tests and its walkthrough passed `checkptr=2`; cgo tests and its walkthrough passed `cgocheck2`, and its Go tests passed the race detector. |
| Fuzzing | The five-second active fuzzing budget completed without a failing input. |
| Benchmarks | Four root benchmark packages and the cgo benchmark package completed three 100 ms samples per case, separately from instrumented checks. |
| Documentation and formatting | All 233 local Markdown targets and the linked heading anchor resolved across 27 documents; Go formatting and whitespace checks were clean. |

The continuity check mapped all **33 original runnable entries** at `c067e69`
to current locations: 29 lessons and four projects. Eight basic source files
remain byte-identical to that baseline: variables, constants, arrays,
conditionals, switch, basic functions, variadic functions, and defer.
Other lessons and projects incorporate the corrections and extensions described
in their examples. The small contact lookup explicitly preserves the
introductory comparison between returned values and stored-element pointers.

This is a host-specific validation record. It records successful executed
checks, rather than proving every input, schedule, memory-lifetime condition,
or target platform. Native out-of-memory conditions were not induced, and Go's
race checks do not instrument this lab's C source for C memory races. Benchmark
results are observations of their stated workloads. Individual "Further
practice" items remain optional exercises beyond the agreed improvement plan.

### REST extension validation

The later contacts REST project was checked on the same Go 1.27.0 darwin/arm64
toolchain. Its finite demo completed, all root-module tests and `go vet ./...`
passed, and its three packages passed the race detector with `-count=3 -cpu=1,2`.
Tests also exercised interactive-mode cancellation, an active request during
shutdown, concurrent HTTP requests, and HEAD on a real connection. Formatting,
whitespace, and local Markdown path checks passed. The independent modules and
the earlier review's specialized checks were unaffected and were not rerun for
this extension.

The subsequent PATCH extension passed the finite PUT/PATCH demonstration,
uncached root-module tests, `go vet ./...`, and the same repeated REST race
checks. Its added tests cover member presence, null removal, optional JSON
member omission, PUT/PATCH differences, all-or-nothing rejection, cancellation,
media types, body limits, OPTIONS discovery, and simultaneous patches to
different fields. Formatting, whitespace, and local Markdown paths were also
checked after this extension.

The collection-query extension passed its finite demonstration, uncached
root-module tests, `go vet ./...`, and the repeated REST race checks. Added tests
cover bounded defaults, query validation, filtering before pagination, Unicode
matching, deterministic ordering, metadata, escaped navigation links, empty and
out-of-range pages, snapshot independence, cancellation, and concurrent writes.
Formatting, whitespace, and local Markdown paths were checked. Conditional
requests with ETag and the OpenAPI contract were the next REST blocks at that stage.

The remaining REST blocks are now complete. The final demonstration exercises
304 revalidation, a successful conditional edit, a rejected stale write with
412, and conditional deletion. Root-module tests and vet passed; REST tests also
passed `-race -count=3 -cpu=1,2`. They check entity-tag grammar, strong and weak
comparison, wildcards and lists, normal error precedence, transformed PUT
validator rules, page tags, actual HEAD behavior, and competing writers.

The OpenAPI 3.1.1 document passed `openapi-spec-validator` 0.9.0. The optional
Python check built a temporary binary, served it outside the source directory,
and validated 37 live HTTP responses, required headers, successful request
fixtures, and empty HEAD/204/304 bodies against the contract using JSON Schema
2020-12. It stopped its owned server cleanly. These are selected fixtures, not
exhaustive protocol coverage. The check's Python dependencies are isolated from
the standard-library-only Go module. Formatting, whitespace, and local Markdown
paths were checked. Persistence, authentication, and deployment remain optional
extensions beyond the completed REST scope.

### Protobuf/gRPC foundation validation

The supplementary module's first stage passed its serialization demonstration,
uncached tests, vet, and `mod tidy -diff` with Go 1.27.0 on darwin/arm64. Its
recorded protoc 35.1 and pinned Go plugins regenerated both checked-in binding
files exactly, verified by the generator's `-check` mode. Tests cover binary
round-trips, independent clones, optional presence, masked clearing, ProtoJSON,
a fixed wire-compatibility fixture, unknown binary fields, malformed input,
and the stable service identity. Formatting and local Markdown links passed.

The main module's dependency file is unchanged. At that stage only local
serialization ran. The unary transport described below implements RPCs;
at that point streaming, service infrastructure, and resilience remained later
stages. Generated interfaces alone do not implement an application.

### Unary gRPC validation

Stage 2 passed the finite loopback TCP demo, uncached module tests, vet, and
`mod tidy -diff`. The whole lab also passed `-race -count=3 -cpu=1,2 -timeout=45s`.
New tests exercise all six RPCs through bufconn, field-mask semantics, typed
BadRequest details, default/filtered pages, rejected multi-field updates,
stale versions, and competing writers using the same version.

Controlled in-flight calls confirmed deadline and cancellation propagation to
the store. TCP tests covered the demo and standalone server/client functions,
lazy channel construction, a controlled unavailable transport, active-call
graceful draining, and forced-stop cancellation with handler completion. A
shared-store integration test made a REST write visible through gRPC and
invalidated an earlier RPC version. Internal-error details were logged but
redacted from client statuses. Formatting and local Markdown paths passed.

The root dependency file and shared contact implementation are unchanged;
the lab uses a local module replacement to import them. The protobuf contract,
generated bindings, and compiler/plugin pins did not change during stage 2.
Streaming and infrastructure/resilience were subsequent stages; streaming
validation follows below.

### Streaming gRPC validation

Stage 3 passed the finite loopback TCP demo, the earlier Protobuf/unary demos,
uncached module tests, vet, and `mod tidy -diff` with Go 1.27.0 on darwin/arm64.
The complete lab passed `-race -count=3 -cpu=1,2 -timeout=45s`, including the
streaming context and lifecycle tests. Regeneration check mode matched both
generated files with the recorded compiler and pinned plugins. Formatting,
whitespace, and local Markdown paths in the updated guides passed.

Bufconn tests covered filtered ordered pages, empty streams, invalid queries,
import summaries, the 100-create cap, partial commits before validation errors,
bidirectional correlation, item failures, and a terminal error after a delivered
reply. Controlled operations confirmed cancellation/deadline propagation for
all three stream shapes and preserved earlier import commits when canceled.
A gated Send verified sequential production, snapshot ownership, released store
locks, and cancellation while sending; it models backpressure without claiming
to measure HTTP2 window sizes. Client tests verified final-status reading after
send EOF and joining a blocked sender after receive failure. TCP tests covered
the default demo, repeated standalone clients, and graceful/forced shutdown of
an active stream.

The three streaming methods and their messages were added without changing
existing field numbers or unary method identities. Dependency files, tool pins,
and the shared contact implementation are unchanged. The server stream selects
one bounded page; imports commit individually rather than transactionally.
At that stage, service infrastructure and resilience remained stages 4 and 5.

### gRPC infrastructure validation

Stage 4 passed its finite verified-TLS demo, uncached module tests, vet, and
`mod tidy -diff` with Go 1.27.0 on darwin/arm64. The whole module passed
`-race -count=3 -cpu=1,2 -timeout=45s`, retaining the earlier examples and
server lifecycle tests. The root and nested module dependency files, contact
rules, Protobuf contract, generated bindings, and compiler/plugin pins are unchanged.

Real loopback TLS tests rejected untrusted CAs and wrong hostnames, while call
credentials rejected plaintext channels. Tests exercised reader/writer access,
unary and stream authentication failures, blocked mutations, ignored role-spoof
headers, response metadata, invalid request IDs, generated IDs, and completion
logs without tokens or contact payloads. Handler probes preserved identity,
request IDs, TLS peer information, deadlines, and cancellation through both
interceptor chains. Certificate tests checked name/expiry verification, private
file permissions, refusal to replace existing keys, and CA-only client setup.

Health tests covered Check/List/Watch, public Check versus protected discovery/
subscriptions, readiness transitions, and NOT_SERVING notification before
graceful/forced shutdown with an open Watch. Reflection tests checked explicit
opt-in and access policy for v1 and v1alpha. Standalone tests generated local
TLS files and ran repeated finite clients against the TLS server.

An independent temporary-server check used grpcurl v1.9.4 over verified TLS for
list/describe, unary creation/retrieval, all three streaming shapes, public
health, missing-token and reader-write rejection, and reader calls using the
local schema. It stopped its owned server and confirmed that completion logs
contained neither fixture credentials nor contact payloads. grpcurl was run as
a versioned external command; it was not added to the lab's dependencies.
Formatting, whitespace, and local Markdown paths were checked. At stage 4,
message limits, retries/idempotency, observability, and final review remained.

### gRPC resilience and final lab review

Stage 5 passed its finite verified-TLS failure demo, module tests, vet, and
`mod tidy -diff`. The full lab passed `-race -count=3 -cpu=1,2 -timeout=45s`,
including the earlier finite demos and unary/streaming/infrastructure cases.
The root module also passed uncached tests and vet. Regeneration check mode
still matched both generated Go files using the pinned compiler/plugins.

The new tests verified three-attempt recovery/exhaustion, nonretryable statuses,
one shared retry deadline, cancellation during backoff, and stable copied
idempotency metadata. In-process cache tests covered concurrent duplicates,
subject/payload binding, isolated response copies, retained terminal errors,
capacity rejection without eviction, and cancellation of a waiting duplicate.
TLS integration tests combined lost-reply injection with concurrent callers and
confirmed that only one underlying create ran. Missing/malformed keys, reused
keys with changed payloads, reader writes, and keys on unsupported RPCs were rejected.

Transport tests exercised client send, server receive, server send, and client
receive limits. Smaller pages and many individually small streamed responses
succeeded. Later oversized stream messages preserved earlier import commits
and delivered replies. OpenTelemetry tests linked client/server spans, correlated
trace/request IDs in completion logs, observed transport rejection before handler
dispatch, excluded keys/credentials/payloads from logs, and enforced the bounded
span-summary exporter with independent snapshots.

OpenTelemetry API/SDK v1.45.0 was added only to the supplementary module, matching
the requirements of its pinned gRPC v1.85.0. The root dependency files, shared
contact rules, Protobuf schema, generated bindings, and generator pins are
unchanged. All five agreed stages are implemented; the
[final lab review](../extensions/01-protobuf-and-grpc/05-resilience/review.md)
records their validation commands, in-memory/idempotency limits, and optional
production-oriented exercises. Formatting, whitespace, and local Markdown paths
were checked.
