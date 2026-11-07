# Concurrency fundamentals

Practice goroutines, channels, explicit completion, cancellation, and shared-state
synchronization. These examples build on functions, pointers, and errors.
Concurrency organizes independently progressing tasks; parallel execution also
depends on the workload, scheduler, and available execution resources. The tiny
computations here illustrate coordination rather than a speed improvement.

## Run and validate

Run from the repository root:

```sh
go run ./chapters/12-concurrency/01-concurrency
go run -race ./chapters/12-concurrency/01-concurrency
go test -race -timeout=30s ./chapters/12-concurrency/01-concurrency
go vet ./chapters/12-concurrency/01-concurrency
```

To exercise more scheduling combinations:

```sh
go test -race -count=5 -cpu=1,2 -timeout=60s ./chapters/12-concurrency/01-concurrency
```

`-cpu` repeats the tests with different `GOMAXPROCS` settings. These tests already
start goroutines; the flag changes their scheduling environment. The race detector
observes accesses on executed paths, so use it alongside review of ownership and
shutdown protocols. A clean run does not establish correctness for every possible
execution.

## Read the example

- [main.go](main.go) runs fifteen small sections, joining each section's work
  before starting the next one.
- [channels.go](channels.go) implements finite producers, selection, a cancellable
  sender, a context preview, and a finite worker pool.
- [synchronization.go](synchronization.go) shows registration and completion with
  `WaitGroup`, a locked counter, typed atomics, and concurrent initialization.
- [main_test.go](main_test.go) checks results, closure, cancellation at different
  blocking points, and shared-state behavior.

No `Sleep` is used as coordination. Channels and waits establish completion.
The timeout section uses a real timer, while tests inject ready expiration events
or use explicit release signals. Five-second test timers are watchdogs for broken
protocols, not guesses about how long a worker should take.

The first goroutine's print may occur before or after `main keeps running`.
The two selected messages can arrive in either order, and worker assignment may
vary. Neither `select` nor the worker pool promises case priority or fair job
distribution. Output from a completed section does not spill into the next one.

## Choose the coordination tool

| Tool | When to use it | Responsibility |
| --- | --- | --- |
| Goroutine | Work should progress independently of the caller. | Define how it returns and how its owner observes completion. |
| `WaitGroup` | The caller must join a finite set of tasks. | Register before launch, call `Done` on return, and wait before consuming dependent results. |
| Channel | Tasks exchange values, transfer ownership, or signal an event. | Define senders, receivers, capacity, closure, and behavior if a receiver stops. |
| Closed stop channel | One owner broadcasts cancellation to several tasks. | Workers observe it at blocking points; the owner joins them afterward. |
| `context.Context` | Work participates in a caller's cancellation/deadline tree. | Propagate the context and observe `Done`; cancellation still needs acknowledgment. |
| `Mutex` | Shared mutable state or a group of related fields needs consistent access. | Protect every relevant read and write with the same lock. |
| `atomic.Int64` | A single independent numeric state needs atomic updates and reads. | Use atomic operations consistently; separate atomics do not make a multi-field invariant atomic. |
| `Once` | Concurrent callers need one initialization whose result is published to all of them. | Use the same `Once`; it is not a reload or retry mechanism. |

The `Counter` keeps its mutex and value together, and its `Value` method also
locks. A `WaitGroup` alone would not protect concurrent increments. The workers
writing different result-slice elements need no shared counter lock, and the
caller reads those elements after `Wait` establishes completion.

`WaitGroup`, `Mutex`, `Once`, and typed atomic values must not be copied after
first use. The examples pass pointers or keep shared instances in one scope.
`ConfigLoader.Load` waits for initialization through `Once.Do`; only the first
supplied initializer runs, even if later calls supply a different function.

The source keeps explicit `Add`/`Done` calls to show the accounting used by the
book. Go 1.25 added `WaitGroup.Go`, which combines registration, starting the
goroutine, and completion for a function that must not panic. It is another
option under this course's Go 1.27 requirement. Neither form provides cancellation.

## Channel state and ownership

| Channel state | Send | Receive |
| --- | --- | --- |
| Open, unbuffered | Waits for a receiver. | Waits for a sender. |
| Open, buffered | Can proceed while capacity is available; otherwise waits. | Drains queued values; otherwise waits. |
| Closed | Panics. | Drains queued values, then returns the element's zero value with `ok == false`. |
| Nil | Blocks indefinitely. | Blocks indefinitely; its case is disabled inside `select`. |

Closing a nil or already closed channel panics. Closing communicates that no
more values will be sent; it is not a general resource-cleanup requirement. The
one-message greeting therefore needs no close. A `range` consuming a stream to
exhaustion finishes when that channel is closed and all queued values are drained.

The sole sender, or a coordinator that knows all senders have returned, owns
closing the stream. Creating a channel does not by itself determine closing
ownership. A receiver should not close a shared data channel to stop its senders;
use a separate cancellation protocol. `chan<- T` and `<-chan T` restrict the
operations a function can perform, but the API contract still assigns ownership.

Check the receive's `ok` flag to distinguish a real zero value from exhaustion.
The example prints a queued zero with `ok=true`, then zero with `ok=false` after
draining. Closed channels remain ready to receive. `collectMessages` sets a
closed input's **local variable** to nil, disabling that select case while the
other input is drained. This does not close or change the caller's channel value.

Buffered channels are bounded queues, not a guarantee against deadlocks or leaks.
The nonblocking send deliberately rejects a third message when capacity is full.
Use a blocking send when backpressure is intended. Adding a `default` branch to
a retry loop can create a busy loop; choose nonblocking behavior explicitly.

Sending copies the channel element. `Job` and `JobResult` contain only integers,
so their message fields are independent values. A pointer, slice, or map element
can still refer to shared data after sending. Define who may access that data,
copy it when independent storage is needed, or protect continued sharing.

## Stop requests, timeouts, and completion

A goroutine does not stop because a caller returns or a receive times out.
`showTimeout` keeps its work gate unreleased, making expiration the only ready
outcome. It then closes the stop channel and waits for the worker before
continuing. `sendWhenReleased` observes cancellation both while waiting to start
and while sending its result, so either blocking point can be released.

Closing a stop channel broadcasts to all receivers, including ones that start
receiving later. That is a request. `Wait` or a completion acknowledgment confirms
that user work has returned. Parent function return, context cancellation, and
`Timer.Stop` do not join goroutines.

If work and cancellation are both ready, `select` may choose either. These
examples do not promise that no in-flight value can be produced after cancellation
becomes ready. Tests arrange only one ready outcome when checking a specific
branch. An unreleased work gate and an unbuffered handshake exercise the two
cancellation points without timing assumptions.

The context preview passes a context to work and checks `ctx.Err()` when `Done`
is observed. The caller invokes its cancel function and separately waits for
completion. Chapter 14's [context lesson](../../14-context/01-context/README.md)
adds deadlines, propagation, and appropriate context values; this introduction
uses cancellation without competing timers.

## Finite pipeline and worker pool

The pipeline's producer closes its output, the square stage closes its own
output, and the caller drains the final stream and joins both stages. This
version requires full consumption. Breaking out of the final range early would
need a cancellation protocol through every blocking stage.

The worker pool processes a finite batch with a positive number of workers:

1. The feeder sends jobs and owns closing the job stream.
2. Workers receive jobs and send one result per job. Individual workers never
   close the shared result stream.
3. A coordinator waits for every worker, then closes results.
4. The caller drains results concurrently with that work and joins the coordinators.

Waiting for workers before receiving their unbuffered results would deadlock.
The function drains internally and returns only after finishing the batch. Its
caller keeps the input slice unchanged until return. Invalid worker counts are
rejected before starting goroutines; an empty batch still completes normally.
Numeric examples use small bounded integers and ordinary Go arithmetic.
Worker count bounds simultaneous work, while the returned result slice still
grows with the batch. A streaming result API would also need a consumer-stop protocol.

Results are sorted by job ID only for display. A worker may receive several
jobs or none; tests check each result exactly once, not assignment or arrival
order. Later pattern lessons will extend these finite examples with fan-out,
fan-in, cooperative early exit, error propagation, and backpressure.

References: [Go memory model](https://go.dev/ref/mem),
[channels](https://go.dev/ref/spec#Channel_types),
[`select`](https://go.dev/ref/spec#Select_statements),
[`sync`](https://pkg.go.dev/sync), [`sync/atomic`](https://pkg.go.dev/sync/atomic),
and [`context`](https://pkg.go.dev/context).
