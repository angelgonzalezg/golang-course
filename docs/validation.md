# Validate the course

Run these commands from the repository root with Go 1.27. The commands are
grouped by what they verify; ordinary tests, race instrumentation, fuzzing, and
benchmarks exercise different properties. The optional cgo group also requires
a matching C compiler and `CGO_ENABLED=1`.

## Root module and formatting

```sh
go test -count=1 -timeout=60s ./...
go vet ./...
gofmt -l chapters
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
Chapter 15's library packages run through `go test`. The two independent command
modules use the explicit `go -C ... run .` commands in this guide. Lessons run
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

This snapshot predates the later REST project and planned gRPC extension. Its
package and program counts describe the reviewed baseline, not those additions.

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
