# Structured logging with slog

Practice `log/slog`: levels, structured attributes, output formats, reusable
metadata, groups, custom value representation, and handler composition. This
chapter 13 lesson builds on methods, interfaces, errors, concurrency, and I/O.
The [chapter 7 project](../../07-types-methods-and-interfaces/05-project-log-levels/README.md)
introduced a log-level type; this lesson uses the standard logging library.

## Run and validate

Run from the repository root:

```sh
go run ./chapters/13-standard-library/05-structured-logging
go test ./chapters/13-standard-library/05-structured-logging
go test -race -timeout=30s ./chapters/13-standard-library/05-structured-logging
go vet ./chapters/13-standard-library/05-structured-logging
```

Repeat the concurrent handler check with different scheduler settings:

```sh
go test -race -count=3 -cpu=1,2 -timeout=30s ./chapters/13-standard-library/05-structured-logging
```

The finite example uses stdout and needs no files, network, or environment
configuration. It mixes section labels, text logs, and JSON records for reading
as a tutorial; the entire output is not a JSON stream. Timestamps, source paths,
and source line numbers naturally vary. The observed write failure is intentional
and uses a fake destination. The command completes successfully afterward.

## Read the example

- [main.go](main.go) runs nine sections, including a deliberately eager debug
  argument, a controlled write failure, and optional source information.
- [logger.go](logger.go) chooses a handler, applies field policies, and defines
  the `Secret` and deferred `lazyValue` representations.
- [orders.go](orders.go) injects a logger at an application boundary and records
  one outcome per operation, preserving errors for the caller.
- [handler.go](handler.go) decorates a standard handler to count returned output
  errors while preserving filtering, metadata, groups, and concurrency support.
- [logger_test.go](logger_test.go) checks configuration, field policies, scopes,
  level changes, deferred evaluation, operation outcomes, and source selection.
- [handler_test.go](handler_test.go) checks standard handler conformance with
  `testing/slogtest`, error forwarding, and a shared counter under concurrent use.

## Choose the output and configuration scope

A `Logger` creates records with a timestamp, severity, message, and attributes.
A `Handler` filters and processes those records. The built-in handlers share
the same logging API:

| Choice | When to use it | Representation |
| --- | --- | --- |
| `TextHandler` | Convenient terminal reading and local diagnostics. | Structured `key=value` text, with groups such as `request.id`. |
| `JSONHandler` | Consumers expect typed fields or nested objects. | One JSON object per record, with groups such as `"request":{"id":"req-42"}`. |
| Local/injected `*slog.Logger` | A component's destination and metadata should be explicit and independently testable. | Construct or derive a logger and pass it to the component. |
| Global default | An application deliberately chooses process-wide logging configuration. | `slog.SetDefault` also redirects the standard `log` package through that handler. |

The constructor defaults to text and an INFO threshold; unsupported formats
return an error. It borrows its writer and leaves the global default unchanged.
In a service, direct structured records to the intended log stream and keep
human-facing output separate from that stream.

For a component such as the previous HTTP lesson's `http.Server`, its standard
logger field can be adapted without changing global logging:

```go
server.ErrorLog = slog.NewLogLogger(logger.Handler(), slog.LevelError)
```

## Choose severity by meaning

| Standard level | Value | Typical use |
| --- | --- | --- |
| `Debug` | -4 | Detailed diagnostic state, often disabled during normal operation. |
| `Info` | 0 | Normal operation milestones and outcomes. |
| `Warn` | 4 | An unexpected condition the operation can recover from or work around. |
| `Error` | 8 | An operation failed and needs an error record. |

Handlers accept records at or above their threshold. A fixed `slog.Level` gives
a fixed threshold; a shared `*slog.LevelVar` allows concurrent-safe changes.
Its zero value is INFO. Standard levels can be parsed with `UnmarshalText`;
check its error before using externally supplied configuration. `slog.Level`
also permits custom numeric levels and names with offsets.

These numbers differ from chapter 7's contiguous `iota` exercise. For example,
that project's Info is 2, while `slog.LevelInfo` is 0. Use an explicit mapping
when integrating such a type rather than casting its numbers. `slog` has no
built-in Fatal method, and `logger.Error` does not panic or exit. Application
control flow still decides whether to return, retry, or exit after cleanup.

## Attributes and groups express a record's structure

Use stable messages and named fields such as `order_id`, `items`, and `request_id`
instead of embedding every changing value in a formatted sentence. Alternating
key/value pairs are convenient, while typed `slog.String`, `Int`, `Bool`, and
`Duration` attributes make the intended type explicit. `LogAttrs` accepts only
attributes, which avoids malformed alternating argument lists and can reduce
conversion work. `go vet` also checks common slog call mistakes.

`With` binds recurring metadata to a derived logger. `WithGroup` scopes subsequent
attributes, while metadata attached before that group keeps its earlier scope.
The parent logger is unchanged. Built-in handlers can preformat bound attributes,
so this also avoids repeatedly formatting the same metadata at each call.

The example's `ReplaceAttr` renames root `level` to `severity`; nested domain
data such as `game.level` remains unchanged. Reserve the root logging keys
(`time`, `level`/`severity`, `msg`, and optional `source`) for the log schema. Reusing a
key in both bound and per-call attributes can emit duplicate names; use groups
or a clear field ownership rule instead of relying on parser-specific precedence.

The built-in JSON handler renders `slog.Duration` as integer nanoseconds; the
text handler uses readable durations. This is a slog representation choice,
distinct from the previous JSON lesson's v2 encoding of arbitrary Go values.
The JSON handler formats an error attribute using its error text; extra typed
error details should be added as explicit attributes when needed.

## Decide representation policy at the appropriate level

`Secret.LogValue` controls that type's representation wherever a structured
value is resolved. `ReplaceAttr` instead applies a destination-wide rule to
fields named exactly `token`, including inside groups. The example demonstrates
both with different fields so their responsibilities are visible.

These rules affect structured attributes. They do not inspect arbitrary message
strings or every field in arbitrary nested objects. Construct messages and field
types accordingly. `ReplaceAttr` receives borrowed group names; do not retain or
modify that slice. It processes individual resolved attributes, not whole groups.

## Control evaluation cost deliberately

Ordinary call arguments are evaluated before logging, including when the record
is below the threshold. The example's filtered eager call increments a counter.
The following `Enabled` guard skips its computation while DEBUG is disabled.
If the threshold changes concurrently between the check and emission, the final
log call can still be filtered; the guard is an optimization, not synchronization.

`lazyValue` uses `LogValuer` to defer computation until a handler resolves a value
on an enabled record. The visible build counts are 1 after the eager/guarded
calls, still 1 after a filtered deferred call, then 2 after enabling DEBUG.
This demonstrates evaluation rather than benchmarking an artificial workload.

Keep such deferred values on the record when that is the intended behavior:
`With` may resolve bound values immediately. Custom value implementations must
also be safe for their actual concurrent use. The sample counter closure runs
serially; it is not a shared counter for concurrent logging. Typed attributes
and deferred work do not guarantee that every log call has zero allocations;
measure the real application when logging cost matters.

## Inject metadata and log a failure once at the handling boundary

`validateOrder` returns a wrapped validation error without recording it.
`processOrder` is this example's application boundary: it records acceptance or
rejection, attaches operation metadata, and returns the cause. The caller handles
that already-recorded error without producing a duplicate failure record. Logging
alone does not handle an error; the returned value still drives control flow.

`LogAttrs` and `InfoContext` pass an available context to the handler. The built-in
handlers do not automatically copy arbitrary context values into attributes.
The example attaches `request_id` with `With`. Context cancellation should not
suppress the record; a canceled operation may be exactly what needs explaining.
Chapter 14 will explore propagation and context values in detail.

`AddSource` includes the logging call's source location when useful. It adds
capture work and record detail. Ordinary wrapper functions around `logger.Info`
report the wrapper's location unless they explicitly preserve the original
program counter; decorating a handler keeps the record's call-site metadata.

## Observe destination errors and own cleanup

Logger methods discard errors returned by `Handler.Handle`. The small decorator
observes those errors, increments a shared atomic counter, and returns the
original error. It implements all four handler methods so derived loggers retain
that behavior. `WithAttrs` and `WithGroup` return wrappers with derived downstream
handlers and the same counter, without copying an in-use atomic value.

This counts returned handler errors, such as destination write failures. It does
not establish durable delivery or count every possible formatting problem;
the standard JSON handler can represent value-encoding errors in its output.
A real application can choose a separate observation or fallback path according
to its needs. Reporting a failure through the same failing handler would recurse.

The wrapper forwards records unchanged and delegates synchronized output to the
standard handler. A wrapper modifying a record's attributes would first need
`Record.Clone`, because a plain record copy may still share internal storage.
Built-in handler derivatives coordinate complete record writes. Unrelated
handlers or other writers sharing the same destination need their own shared
synchronization policy; timestamps do not sort concurrent events automatically.

`slog.Logger` does not close files or flush a caller-supplied buffered writer.
The writer's owner stops concurrent logging, flushes buffering, and closes owned
resources with the appropriate error checks. A later flush failure is outside
the decorator's per-record `Handle` observation. The executable borrows stdout
and leaves it open.

References: [`log/slog`](https://pkg.go.dev/log/slog),
[structured logging introduction](https://go.dev/blog/slog),
[handler guide](https://go.dev/s/slog-handler-guide),
and [`testing/slogtest`](https://pkg.go.dev/testing/slogtest).
