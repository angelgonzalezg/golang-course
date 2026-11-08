# Writing tests

Learn one technique at a time using a small label-normalization library. These
examples are library packages: run them with `go test`, and read their
`_test.go` files alongside the implementation. They use only the standard library.

Read functions and [errors](../09-errors/01-error-handling/main.go) first.
The later examples build on [interfaces](../07-types-methods-and-interfaces/03-interfaces/main.go),
[HTTP](../13-standard-library/04-http/README.md),
[context](../14-context/01-context/README.md), and
[concurrency](../12-concurrency/01-concurrency/README.md).

## Reading order and when to use each technique

| Lesson | Read | Use it for |
| --- | --- | --- |
| 1. Unit tests | [labels.go](01-unit-tests/labels.go), [labels_test.go](01-unit-tests/labels_test.go), [example_test.go](01-unit-tests/example_test.go) | Known inputs, expected results, boundaries, and error contracts. |
| 2. Test doubles | [save.go](02-test-doubles/save.go), [save_test.go](02-test-doubles/save_test.go) | Dependency failures and interactions that are hard to arrange with a real service. |
| 3. HTTP tests | [handler.go](03-http-tests/handler.go), [handler_test.go](03-http-tests/handler_test.go) | Handler behavior directly, then requests through a local server. |
| 4. Fuzzing | [normalize_test.go](04-fuzzing/normalize_test.go) | Properties that should hold across generated strings, including malformed UTF-8. |
| 5. Benchmarks and races | [counter.go](05-benchmarks-and-races/counter.go), [counter_test.go](05-benchmarks-and-races/counter_test.go), [benchmark_test.go](05-benchmarks-and-races/benchmark_test.go) | Measured operation costs and concurrent reads/writes of shared state. |

The shared `Normalize` contract trims surrounding Unicode whitespace, lowercases
text, and accepts 1–40 UTF-8 runes afterward. Internal whitespace and punctuation
remain. Counts are runes, rather than bytes or user-perceived characters.
Failure returns an empty string and `ErrInvalidLabel`.

## Run the lessons

From the repository root:

```sh
go test -v ./chapters/15-testing/...
go vet ./chapters/15-testing/...
go test -race -count=3 -cpu=1,2 -timeout=30s ./chapters/15-testing/...
```

Run one named table case, or check the external documentation example:

```sh
go test -v -run='^TestNormalizeCases$/^unicode$' ./chapters/15-testing/01-unit-tests
go test -v -run='^ExampleNormalize$' ./chapters/15-testing/01-unit-tests
```

`-run` selects tests; `-count=1` requests an uncached run. Add `-v -shuffle=on`
to vary test order and display a seed; pass that number to `-shuffle` to reproduce
the order. This helps expose accidental shared state. See the official
[`go test` flags](https://pkg.go.dev/cmd/go#hdr-Testing_flags).

## 1. Start with observable results

`TestNormalize` starts with one input and result. The table then adds Unicode,
the rune limit, empty input, and invalid UTF-8. Expected outputs are explicit;
the test does not compute them by calling a second copy of the implementation.

`t.Run` gives each case a name. These pure cases use `t.Parallel` because they
share no mutable resources. Choose sequential tests when they must coordinate
shared state. Process-wide changes such as `t.Setenv` are incompatible with
parallel tests or parallel ancestors. The Go 1.27 loop variables already have
per-iteration scope. See [subtests](https://go.dev/blog/subtests) and the
[`testing` API](https://pkg.go.dev/testing).

Use `t.Helper` in assertion helpers. `Fatalf` stops a test when a prerequisite
fails; `Errorf` records a failure and allows further checks. Call `Fatalf` from
the test goroutine; collect worker errors and join the workers there.
Inspect error causes with `errors.Is` instead of matching presentation text.

`LoadFile` tests use `t.TempDir` for isolated files and automatic removal. The
checked-in [testdata/label.txt](01-unit-tests/testdata/label.txt) is a reusable
fixture. Tests in `package labels` can see internals; `package labels_test`
exercises the exported API. `ExampleNormalize` has an `Output` comment, so
`go test` checks its output as executable documentation.

## 2. Control a dependency with a small interface

`SaveLabel` receives a `LabelStore`; the consumer defines the single operation
it needs. Its spy records the normalized value, call count, and request context.
The count matters here because the contract promises one storage call on success
and none for invalid input or a request already canceled at entry.

The `storeFunc` stub returns a chosen failure, letting a test check that wrapping
preserves its cause. Both doubles live in `_test.go` files and belong to their
individual tests. No real database or mocking framework is needed for these
tests. A fake could implement a lightweight in-memory store; integration tests
would still be needed to check a real storage implementation's behavior.

The store must observe cancellation that occurs during its operation. Injecting
a context does not interrupt a dependency that ignores it. See the earlier
[context tests](../14-context/01-context/reports_test.go) for coordinated in-flight
cancellation and cleanup.

## 3. Choose the HTTP boundary

`NewRecorder` and `NewRequestWithContext` exercise routing, status, headers, and
bodies without binding a port. Inspect `recorder.Result()` for the committed
response. `NewServer` starts a real local server on an available port, allowing
the standard client and transport to participate. The HEAD test needs that
boundary because the server suppresses response bodies on the wire.

`t.Cleanup` closes response bodies before the server, using reverse registration
order. Register cleanup as soon as a resource is acquired so early failures also
release it. `t.Context()` is canceled before cleanup begins. The client timeout
bounds a failed request; the tests use no sleeps for synchronization.
See [`httptest`](https://pkg.go.dev/net/http/httptest) and the larger
[chapter 13 HTTP example](../13-standard-library/04-http/README.md).

## 4. Fuzz properties beyond the table

Normal `go test` runs the seed corpus, including the
[checked-in invalid-UTF-8 input](04-fuzzing/testdata/fuzz/FuzzNormalize/invalid-utf8).
Active fuzzing generates more inputs. Run one target in one package with an
explicit budget and a small worker count:

```sh
go test -fuzz='^FuzzNormalize$' -fuzztime=5s -parallel=2 ./chapters/15-testing/04-fuzzing
go test -v -run='^FuzzNormalize$/^invalid-utf8$' ./chapters/15-testing/04-fuzzing
```

The properties check valid output, canonical form, idempotence, surrounding
whitespace, and failure contracts. They complement the table's known expected
results. A function that rejects every input still needs to fail those ordinary
success tests. Keep each fuzz invocation deterministic and independent of shared
mutable state.

On failure, Go tries to minimize the input and writes it under
`testdata/fuzz/FuzzNormalize`. Diagnose the bug, fix it, rerun the failing seed,
and keep that file as a regression test. Generated coverage-expanding inputs
also live in the fuzz cache; they are distinct from checked-in seeds. A short
successful session does not establish correctness. See the
[Go fuzzing guide](https://go.dev/doc/security/fuzz/).

## 5. Measure performance and exercise shared state

```sh
go test -run='^$' -bench=. -benchmem -benchtime=100ms -count=3 -cpu=1,2 ./chapters/15-testing/05-benchmarks-and-races
```

The sequential benchmarks use `b.Loop`, available since Go 1.24. It manages
timing around the loop and keeps the direct loop's measured calls observable.
`ReportAllocs` adds heap allocation metrics; `SetBytes` reports input throughput.
Read `ns/op`, `B/op`, and `allocs/op` as measurements of this workload and compiler,
without asserting a universal speed or a fixed allocation count.

`RunParallel` distributes a total of `b.N` increments across workers sharing
one counter. It uses `pb.Next`, with explicit timing around that API, and does
not combine with `b.Loop`. `-cpu` varies `GOMAXPROCS`; it only creates a parallel
workload when the benchmark does so. Parallel `ns/op` describes aggregate
throughput, not individual request latency. Compare repeated runs under the
same conditions; race-instrumented timings are unsuitable as a normal baseline.
See [benchmark loop guidance](https://go.dev/blog/testing-b-loop) and
[`RunParallel`](https://pkg.go.dev/testing#B.RunParallel).

`TestCounterConcurrent` exercises reads and writes, starts workers through a
channel, and joins them before checking the final total. The mutex protects
both operations. A passing total alone cannot establish the absence of races.
Run with `-race`; the detector can only report races exercised by that run.
Different CPU counts and repetitions vary scheduling without guaranteeing every
possible interleaving. See the [race detector guide](https://go.dev/doc/articles/race_detector).

## Coverage and further practice

```sh
go test -coverprofile=/tmp/golang-course-testing.cover ./chapters/15-testing/...
go tool cover -func=/tmp/golang-course-testing.cover
```

Coverage shows executed statements, helping locate missing scenarios. It does
not establish assertion quality, branch completeness, or correctness. Check
the target package's report when an importing lesson executes its functions.
Use `-coverpkg` explicitly when you want coverage of dependencies as well.

1. Add a label rule and write one success and one failure case before changing
   the implementation. Observe the failing test, implement the rule, and rerun it.
2. Add an in-memory fake store and a separate contract test for a real store.
3. Replace the counter's mutex with an atomic operation and repeat the behavior,
   race, and benchmark runs. Explain what the measurements cover.

For larger shared setup, prefer per-test helpers and cleanup first. `TestMain`
is available when a package truly needs process-level setup. Keep ordinary tests
self-contained so they can run alone, repeatedly, and in different orders.
