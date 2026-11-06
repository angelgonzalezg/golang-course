# Garbage collector and allocation experiments

Read [pointers](../01-pointers/main.go) and
[pointer performance](../02-pointer-performance/README.md) first. The pooling
section previews interfaces and concurrency, which later chapters explain in
more detail. All builder counts in this lesson are nonnegative sample sizes.

## Run and measure

Run from the repository root:

```sh
go run ./chapters/06-pointers/03-garbage-collector
go test ./chapters/06-pointers/03-garbage-collector
go test -race ./chapters/06-pointers/03-garbage-collector
go test -run='^$' -bench=. -benchmem -count=5 ./chapters/06-pointers/03-garbage-collector
```

For a shorter sequential benchmark run, add `-cpu=1 -benchtime=100ms -count=3`
instead of `-count=5`. Setup is outside `b.Loop`; each measured operation performs
the full task named by its benchmark. Timing and allocation counts vary with the
toolchain and execution environment. Tests check behavior, not timing thresholds.

## Reachability and observations

The GC follows references from roots, including live stack and global references,
to find heap objects that must remain alive. Stack-allocated values are not heap
garbage, but stack pointers can keep heap objects reachable. The compiler decides
placement using escape analysis and other constraints. GC work depends on live
data and pointers to scan as well as the rate of new allocations.

The first example retains buffers in a global slice, observes memory, then drops
that reference and explicitly collects. Later examples clear object pointers in
a still-live slice. `runtime.KeepAlive` keeps the original object graph alive
through the first observation, and keeps the cleared container alive through the
second. This prevents compiler liveness decisions from obscuring the comparison.

`clear(slice)` clears only its visible elements. Reslicing an owned container to
its capacity before clearing also covers hidden slots. Setting the length to
zero alone leaves those references in the backing array. Clearing references
does not free the array itself or any object that remains reachable elsewhere.

| Observation | Interpretation |
| --- | --- |
| `HeapAlloc` | Heap bytes currently allocated, including objects not yet reclaimed. |
| `HeapObjects` | Allocated heap objects, including those still awaiting reclamation. |
| `NumGC` | Completed GC cycles. |
| `NextGC` | Target heap size for the next collection. |
| `TotalAlloc` delta | Cumulative heap bytes allocated during the interval, even if freed. |
| `Mallocs` delta | Cumulative heap object allocations during the interval. |

These are process-wide observations and include runtime activity. A forced GC
helps demonstrate reachability; application code normally lets the runtime
schedule collections. Reclaiming Go heap objects does not guarantee an immediate
reduction in process RSS or a return of every freed byte to the operating system.

## Choose value or pointer elements

| Representation | When to use it | Performance considerations |
| --- | --- | --- |
| `[]T`: slice of values, such as `[]Receipt` | Start here for small records stored and processed together when per-object identity and nil entries are unnecessary. | Contiguous element storage; copying elements copies their fields. This pointer-free `Receipt` array has no element pointers for the GC to scan. |
| `[]*T`: slice of pointers, such as `[]*Receipt` | Use for shared objects, identity preserved across slice growth, or nil entries. | Adds references to scan and may create separate heap objects. For large structs, measure these costs against copying values. |

Both are mutable slices and can share backing storage after assignment. You can
update `receipts[i]` in `[]Receipt`; mutation alone does not require pointer
elements. Copying a struct also copies any slice/map references it contains.
`Receipt` contains only scalar fields, making the value-copy example self-contained.

## Compare equivalent work

- Receipt benchmarks build the same fields using the two storage layouts above.
- Slice and map benchmarks produce the same IDs and entries with or without
  preallocation. Slice capacity and the map's size hint have different contracts;
  the map hint does not fix its capacity or guarantee an exact allocation count.
- Message processing benchmarks build and synchronously checksum the same
  `request-id:N` messages. One starts a fresh buffer each time; the other resets
  a single buffer. Both return the checksum and keep no message views.
  The compiler may already keep the fresh small buffers off the heap.
- `buildMessagesWithNewBuffers` has a different contract: it returns every
  message with independent byte storage. It is demonstrated separately from the
  streaming comparison. Keeping views of a repeatedly overwritten buffer would
  corrupt retained results.

An operation in the receipts, IDs, lookup, or message-processing benchmark builds
or processes **1000 items**. Its `ns/op` and allocation metrics cover that whole
batch. `MessageChecksum` measures one message per operation and also reports byte
throughput.

In a local Go 1.27.0 darwin/arm64 run with `-cpu=1`, both message-processing
variants reported zero heap allocations. The compiler already avoided heap
allocation for those small temporary buffers. In the separate checksum benchmark,
the 2176-byte message required one allocation with a local buffer and reported
zero allocations with the warmed pool. These observations illustrate why the
workload and compiler matter; they are not guaranteed counts for future runs.

## Pool ownership and retention

`sync.Pool` is an optional reuse mechanism for temporary objects. `Get` may ignore
stored items or create a fresh one through `New`; items may disappear at any
time. Correctness cannot depend on getting a previous object back. Pooling can
trade allocation work for retained memory and synchronization overhead, so the
lesson benchmarks it against a local buffer doing the same checksum work.

The pool stores `*messageBuffer` rather than a slice value, avoiding repeated
boxing of a slice descriptor into an interface. A borrower gets exclusive use
of the object, resets its length, appends data, consumes the bytes synchronously,
and returns the object. It returns a scalar checksum, never an alias into pooled
storage. Access after `Put` would violate that ownership rule. The pool's methods
support concurrent callers; an individual borrowed buffer is not shared.

`prepareForPool` resets buffers of at most 64 KiB capacity. Larger buffers drop
their backing-array reference and are discarded. This bounds the capacity of
each returned buffer, not the total memory retained by the pool. Resetting length
does not erase previous byte contents. Tests check this policy without assuming
that `Get` returns any particular object, and exercise concurrent borrowers with
the race detector.

## GOGC and the soft memory limit

The program reads the configured settings through `runtime/metrics`, preserving
the environment chosen for the experiment. Try separate runs:

```sh
GOGC=50 go run ./chapters/06-pointers/03-garbage-collector
GOGC=200 go run ./chapters/06-pointers/03-garbage-collector
GOMEMLIMIT=128MiB go run ./chapters/06-pointers/03-garbage-collector
GODEBUG=gctrace=1 go run ./chapters/06-pointers/03-garbage-collector
```

The growth target is approximately `live heap + (live heap + roots) * GOGC/100`.
A lower GOGC generally trades more GC CPU work for less heap growth; a higher
value generally permits more heap growth. This is a target, and the soft memory
limit can cause collections earlier. `GOGC=off` disables growth-triggered GC;
explicit `runtime.GC` calls in this lesson still collect, and a configured memory
limit can still trigger GC.

`GOMEMLIMIT` applies to Go runtime-managed memory, represented by
`MemStats.Sys - MemStats.HeapReleased`. It covers more than the heap while excluding
some process memory, such as C allocations and mappings outside the runtime.
It is a soft target, not a process RSS cap, and can be exceeded. Reachable data
cannot be collected to meet a smaller limit. At an application boundary,
`debug.SetGCPercent` and `debug.SetMemoryLimit` can change these settings at runtime;
this walkthrough observes the selected configuration instead.

Further practice: compare a larger message distribution, inspect an allocation
profile, or add a consumer that must retain messages and account for the necessary
copies. Choose the required behavior first, then measure its cost.

References: [Go GC guide](https://go.dev/doc/gc-guide),
[`sync.Pool`](https://pkg.go.dev/sync#Pool),
[`runtime.KeepAlive`](https://pkg.go.dev/runtime#KeepAlive),
[`runtime/metrics`](https://pkg.go.dev/runtime/metrics), and
[`debug.SetMemoryLimit`](https://pkg.go.dev/runtime/debug#SetMemoryLimit).
