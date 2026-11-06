# Pointer performance

Read [pointer basics](../01-pointers/main.go) first. This lesson distinguishes
ownership, copying, and heap allocations with three payloads: 16 B, 4 KiB, and
1 MiB. Here, KiB means 1024 bytes and MiB means 1048576 bytes.

## Run and measure

Run from the repository root:

```sh
go run ./chapters/06-pointers/02-pointer-performance
go test ./chapters/06-pointers/02-pointer-performance
go test -run='^$' -bench=. -benchmem -count=5 ./chapters/06-pointers/02-pointer-performance
```

The runnable walkthrough shows which values are independent and which share
storage. Timing belongs to the benchmark runner, which calibrates the iteration
count. For a shorter experiment on one Go execution processor:

```sh
go test -run='^$' -bench=. -benchmem -cpu=1 -benchtime=100ms -count=3 ./chapters/06-pointers/02-pointer-performance
```

These benchmarks execute sequentially. Increasing `-cpu` changes `GOMAXPROCS`;
it does not automatically parallelize them. A parallel workload needs its own
benchmark design. Compare repeated runs with the same toolchain, machine,
workload, and settings, with other heavy work stopped.

## What each case measures

| Benchmark | Operation and ownership |
| --- | --- |
| `ExistingData/.../Value` | Assigns a copy of existing payload data to a global value sink. |
| `ExistingData/.../Pointer` | Assigns an alias to existing payload data to a global pointer sink. |
| `NewSmall/ValueRetained` | Constructs a fresh small value and assigns it to a global value sink. |
| `NewSmall/PointerRetained` | Constructs a fresh small object and retains its pointer globally. |
| `NewSmall/ValueLocal` | Constructs a value and consumes its endpoint bytes locally. |
| `NewSmall/PointerLocal` | Constructs a pointer result and consumes the same bytes locally. |

Returning an existing value provides independent array contents. Returning its
address shares mutable storage. The speed difference between those two cases
does not tell you which ownership model an application needs.

The retained pointer case intentionally makes each fresh object escape its
caller. Its allocation behavior describes that use site. The local pointer case
lets the compiler inline the constructor and potentially avoid a heap allocation.
Returning the address of a local variable is safe; the compiler arranges its
lifetime. Large values can also require heap storage, and copies can be optimized.
Neither `*T` nor `T` is a general promise about stack placement or speed.

## Read the output

| Column | Meaning |
| --- | --- |
| Iteration count | Number of measured loop iterations, chosen by the runner. |
| `ns/op` | Average elapsed time per operation. |
| `B/op` | Heap bytes allocated per operation, averaged over the run. |
| `allocs/op` | Heap allocations per operation, averaged over the run. |

`0 B/op` does not mean no data was copied or no memory was used. Assignment to
existing global storage can copy a large array without allocating a new heap
object. Reading a pointer can be cheap while the referenced object still has
allocation, lifetime, and GC costs elsewhere.

The benchmarks use `for b.Loop()` and `b.ReportAllocs()`. `Loop` excludes setup
before its first call, stops timing after the loop, and keeps work inside the
loop observable to the compiler. The global sinks additionally model retained
results; they deliberately affect escape behavior. The local cases need only a
scalar checksum after the loop.

`B.Loop` was added in Go 1.24, after the book's publication. It fits this course's
Go 1.27 requirement. Older examples use `for i := 0; i < b.N; i++` with explicit
timer management; use one loop style per benchmark.

For example, a local run with Go 1.27.0 on darwin/arm64, `-cpu=1`, 100 ms samples,
and three repetitions reported:

| Case | `B/op` | `allocs/op` |
| --- | --- | --- |
| `ExistingData/4KiB/Value` | 0 | 0 |
| `ExistingData/1MiB/Value` | 1048576 | 1 |
| `NewSmall/PointerRetained` | 16 | 1 |
| `NewSmall/PointerLocal` | 0 | 0 |

The 4 KiB case still copies data. The 1 MiB case also creates a heap temporary
for the large returned value, as reported by escape analysis at its benchmark
call site. The small pointer cases differ in lifetime. These observations
describe this compiler and benchmark shape; rerun them for your environment.

## Inspect escape analysis

```sh
go test -run='^$' -gcflags='-m=2' ./chapters/06-pointers/02-pointer-performance
```

Look for inlining decisions and reports such as `escapes to heap` or `does not
escape`. Read the call-site reports as well as the constructor's report: inlined
calls can have different allocation behavior. Formatting output also produces
escape diagnostics unrelated to the measured constructors. Compiler reports and
exact counts can change with Go releases; the tests check ownership and contents,
without asserting timing or a fixed allocation count.

Continue with the [garbage collector lesson](../03-garbage-collector/README.md)
for object graphs, buffer reuse, and GC observations. Further practice: add an
intermediate payload size or a realistic consumer and explain what its lifetime
requires before measuring it.

References: [Go GC guide](https://go.dev/doc/gc-guide),
[benchmark API](https://pkg.go.dev/testing#B), and
[`B.Loop`](https://pkg.go.dev/testing#B.Loop).
