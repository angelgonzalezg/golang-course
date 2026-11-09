# Optional cgo lab: a small Go/C boundary

Use cgo when a native library or system interface supplies something your Go
program needs. This lab teaches the boundary using local C11 code and the C
standard library. It has its own [go.mod](go.mod), module path
`example.com/course/cgo-lab`, and no downloaded dependencies or root imports.
Root `go test ./...` stops at this module boundary, so run its checks separately.

Read [modules](../../10-modules-and-packages/README.md),
[pointers](../../06-pointers/01-pointers/main.go),
[errors](../../09-errors/01-error-handling/main.go),
[testing](../../15-testing/README.md), and [unsafe](../02-unsafe/README.md) first.

## Requirements and commands

The lab needs Go 1.27, `CGO_ENABLED=1`, and a C compiler for the current target,
such as clang or gcc selected by `CC`. Inspect the environment first:

```sh
go version
go env CGO_ENABLED CC GOOS GOARCH
```

From the repository root, explicitly select this module:

```sh
CGO_ENABLED=1 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo run .
CGO_ENABLED=1 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo test -v ./...
CGO_ENABLED=1 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo vet ./...
```

Use command-scoped environment settings rather than changing global Go defaults.
Setting `CGO_ENABLED=1` still requires an installed compiler. With cgo disabled,
files importing `C` are excluded; this optional module requires those files.
Cross-compiling it also requires a matching C cross-compiler. See the official
[cgo build instructions](https://pkg.go.dev/cmd/cgo#hdr-Using_cgo_with_the_go_command).

The main course remains independently runnable without cgo:

```sh
CGO_ENABLED=0 go test ./...
```

## Read the boundary in order

| File | Responsibility |
| --- | --- |
| [main.go](main.go) | Calls an ordinary Go API and displays four demonstrations. |
| [native/bridge.go](native/bridge.go) | The sole `import "C"`, type conversions, validation, and C allocation cleanup. |
| [native/ops.h](native/ops.h) | C declarations and pointer/ownership contracts. |
| [native/ops.c](native/ops.c) | The C implementations compiled with the package. |
| [native/bridge_test.go](native/bridge_test.go) | Tests the real C-backed wrappers through Go. |
| [native/benchmark_test.go](native/benchmark_test.go) | Compares an equivalent byte sum in Go and C. |

The comment immediately before `import "C"` is the preamble. It includes the
header and supplies `#cgo CFLAGS` for C11 and compiler warnings. The Go tool builds
the package's `.c` files; no separate manual linker command is needed here.
`C` is a special pseudo-package. C types stay inside `native`; callers use Go
types and error values. See the introductory [C? Go? Cgo! article](https://go.dev/blog/cgo).

## 1. Map types and failure contracts explicitly

`Add` accepts Go `int32`, converts to C `int32_t`, and receives an `int64_t` sum.
The operands are widened before addition in C, avoiding signed int32 overflow.
The wrapper then checks the result before narrowing it to Go `int32`.
An out-of-range result returns `(0, ErrOverflow)` through wrapping.

Keep the C function's actual failure convention in view: `course_greet` returns
NULL when its result cannot be allocated; `Add` checks a numeric range. These
wrappers translate their respective outcomes into Go errors.

## 2. Borrow Go bytes for one call

`SumBytes` passes a pointer to bytes and their visible length. The backing array
contains no Go pointers. Its C function reads synchronously, returns a scalar,
and retains no address. Nil and empty inputs use `(NULL, 0)`. The byte sum is
modulo 2^64 and supplies no cryptographic guarantees.

Cgo implicitly pins Go memory passed as a pointer argument for the call's
duration. This lab's C functions never keep that pointer afterward. The caller
must also keep the input stable while C reads it; a `const` C parameter does not
prevent another Go goroutine from writing to the array.

The allocation's origin determines whether a pointer refers to Go or C memory;
casting its type does not change that origin. Longer retention requires a more
involved lifetime protocol. `runtime.Pinner` can pin eligible Go objects, and
`runtime/cgo.Handle` supports opaque tokens for Go values. Those mechanisms
serve different purposes. See the current
[pointer-passing rules](https://pkg.go.dev/cmd/cgo#hdr-Passing_pointers).

## 3–4. Copy when ownership crosses the boundary

| Operation | Storage and cleanup |
| --- | --- |
| `C.CString(name)` | Copies bytes and adds a NUL terminator in C memory; `Greet` defers `C.free`. |
| `course_greet(input)` | Allocates a separate C result with `malloc`; `Greet` checks NULL and defers `C.free`. |
| `C.GoString(output)` | Copies the terminated result into a Go string before the C allocation is freed. |
| `C.CBytes(data)` | Copies binary input to C-owned memory; `ReverseCopy` defers `C.free`. |
| `C.GoBytes(buffer, length)` | Copies the reversed binary result into independent Go bytes before C cleanup. |

`Greet` accepts empty names and preserves bytes, including UTF-8 examples. It
rejects embedded NUL bytes so a C string cannot silently truncate the input.
The operation does not validate Unicode or normalize names. `ReverseCopy`
handles NUL bytes as ordinary binary data because its API carries a length.
Empty input returns nil; nonempty output can be edited without changing input.

The lab limits reverse copies to `MaxInt32` bytes before converting the return
length to `C.int` for `GoBytes`. The limit test checks counts without allocating
gigabytes. These conversions copy rather than return views into freed C storage.

The Go GC does not free the C allocations owned here. Each successful allocation
has a cleanup owner. `ErrNativeAllocation` covers NULL from `course_greet`;
cgo's allocation helpers used by `CString` and `CBytes` instead terminate on
allocation failure, so they do not expose an ordinary recoverable allocation
error. The tests do not induce out-of-memory conditions.

## Validate the actual boundary

```sh
CGO_ENABLED=1 GOEXPERIMENT=cgocheck2 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo test -count=1 ./...
CGO_ENABLED=1 GOEXPERIMENT=cgocheck2 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo run .
CGO_ENABLED=1 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo test -race -timeout=30s ./...
```

`cgocheck2` enables the toolchain's stronger Go/C pointer checks at build time.
The tests compile and call the C implementation, cover overflow and NUL handling,
and retain results across later native allocations. They also verify that
borrowed input and independently copied results obey their mutation contracts.

The Go race detector does not turn these C files into race-instrumented C code,
and pointer checks do not establish freedom from native memory leaks or every C
memory error. Review the C implementation and its allocation/free pairs as well.
Tests live in Go files without importing `C`; wrappers keep the testable surface
in ordinary Go types. No shared mutable C state or background C work is started.

## Measure the complete operation

```sh
CGO_ENABLED=1 go -C chapters/16-reflect-unsafe-and-cgo/03-cgo test -run='^$' -bench=. -benchmem -benchtime=100ms -count=3 ./native
```

The benchmark performs the same byte sum over the same 16-byte and 4096-byte
inputs. Setup is outside `b.Loop`. The C case includes its Go wrapper and the
Go/C transition as well as the native loop. Differences also reflect compiler
optimization of the two loops, so the result does not isolate transition cost.
Changing the amount of work per call helps examine when crossing the boundary
matters. A native implementation is not automatically faster.

A local Go 1.27.0 darwin/arm64 run with 100 ms samples and three repetitions
reported these ranges for the complete operations:

| Input | Go `ns/op` | Cgo `ns/op` |
| --- | --- | --- |
| 16 bytes | 7.2–7.5 | 24.5–25.0 |
| 4096 bytes | 1228–1233 | 254.5–255.0 |

Both cases reported zero Go heap allocations per operation. These observations
illustrate how the workload changes the comparison; rerun them with your Go and
C compilers before drawing conclusions for another operation.

Allocation metrics describe Go heap allocations; they do not account for C
`malloc` storage. This benchmark's C sum allocates no C memory. Run performance
samples separately from instrumentation and compare repeated runs on the same
toolchain and machine. Native allocations also sit outside Go's GC memory budget;
see the earlier [GC lesson](../../06-pointers/03-garbage-collector/README.md).

## Further practice

1. Bind a real native library behind a similarly small Go interface. Document
   installation, linking, ownership, and the library's thread-safety contract.
2. Add a callback example using `//export` and `runtime/cgo.Handle`. Specify when
   the token is deleted and how C acknowledges that callbacks have finished.
3. Compare one call per small record with a batched operation over many records.
   Include the conversion and copying work required by each API.
