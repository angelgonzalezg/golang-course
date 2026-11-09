# Unsafe: layout, pointers, and ownership

Read [pointer basics](../../06-pointers/01-pointers/main.go),
[slice internals](../../03-composite-types/03-slice-internals/main.go),
[strings and bytes](../../03-composite-types/04-strings-bytes-and-runes/README.md),
[GC](../../06-pointers/03-garbage-collector/README.md), and
[reflection](../01-reflection/README.md) first.
The examples use the course's Go 1.27 toolchain and Go-owned memory.

## Run and validate

From the repository root:

```sh
go run ./chapters/16-reflect-unsafe-and-cgo/02-unsafe
go test -v ./chapters/16-reflect-unsafe-and-cgo/02-unsafe
go vet ./chapters/16-reflect-unsafe-and-cgo/02-unsafe
```

Run pointer instrumentation on both the tests and the complete walkthrough:

```sh
go test -gcflags=all=-d=checkptr=2 ./chapters/16-reflect-unsafe-and-cgo/02-unsafe
go run -gcflags=all=-d=checkptr=2 ./chapters/16-reflect-unsafe-and-cgo/02-unsafe
```

Read the numbered sections of [main.go](main.go), then [memory.go](memory.go)
and [strings.go](strings.go). The tests check aliases, rejected indexes, and
independent retained strings. The walkthrough prints sizes for its compilation
target; the tests avoid fixed size assumptions.

`checkptr` instruments unsafe operations; level 2 also forces heap allocation
for conversions to `unsafe.Pointer`. It changes allocation behavior, so keep it
out of performance comparisons. Instrumentation and vet help find mistakes;
passing checks does not establish every bounds or ownership precondition.
See the [pointer-checking background](https://go.dev/doc/go1.14#compiler).

## Choose the ordinary operation first

| Need | Ordinary Go approach | What this lesson explores |
| --- | --- | --- |
| Inspect size, alignment, or offsets | `unsafe.Sizeof`, `Alignof`, and `Offsetof` observe layout without dereferencing arbitrary memory. | Field order and padding on the current target. |
| Obtain floating-point bits | `math.Float64bits` | A documented compatible-representation conversion. |
| Address a field or slice element | `&record.Count` or checked `&values[index]` | Byte offsets with `unsafe.Add`. |
| View an existing array as a slice | `array[:]` | `unsafe.Slice` with a known allocation and length. |
| Produce an independent string or mutable bytes | `string(buffer)` or `[]byte(text)` | Private-buffer string ownership and a read-only borrowed view. |

Typical reasons for representation-level code include a low-level system
boundary or a measured copying cost that ordinary APIs cannot address suitably.
Keep that implementation small and give its callers an explicit contract.
For building strings, consider the standard
[`strings.Builder`](https://pkg.go.dev/strings#Builder) before writing custom unsafe code.

## 1. Size and alignment describe the value itself

`Sizeof` includes a struct's padding and excludes separately referenced storage.
For a slice or string it measures the descriptor, rather than the referenced
contents. Allocator overhead and total retained memory require other observations.

`Alignof` gives the required address alignment; `Offsetof` gives a field's byte
position inside its struct. The two record types show how changing field order
can change padding. Exact results depend on the target and implementation.
These three layout operations produce constants for the concrete fixed-size
types used here.
These layouts describe Go values; wire formats need an explicit encoding.
See the [size and alignment rules](https://go.dev/ref/spec#Size_and_alignment_guarantees).

## 2–4. Preserve real pointers and stay inside their allocation

The float example reads the same bits through compatible `float64` and `uint64`
storage. A numeric conversion instead computes an integer value. Prefer
[`math.Float64bits`](https://pkg.go.dev/math#Float64bits) for that application task.
Arbitrary structs, differently aligned data, and values containing pointers
need their own layout analysis; equal sizes alone do not establish compatibility.

| Representation | Meaning for Go-managed storage |
| --- | --- |
| `*T` | A typed pointer that can keep its referenced allocation reachable. |
| `unsafe.Pointer` | A pointer subject to documented conversion rules, retaining pointer semantics. |
| `uintptr` | An integer address with no GC reference or relocation tracking. |

`uint32At` checks the index against the visible length before computing an
offset. Its base remains an `unsafe.Pointer`; the `uintptr` holds only a byte
offset, never a saved object address. An element stride preserves alignment.
`countPointer` uses the offset of an actual declared field in the same struct.
Both return aliases to existing data, with ordinary pointer lifetimes.

Pointer arithmetic must remain inside the original allocation. Spare capacity
does not expand this helper's accepted slice length. A pointer saved before an
append may still refer to an old backing array if growth reallocates; it does
not start following the replacement element. Use stable ownership while editing.

Storing an address in `uintptr` and converting it back later is generally
invalid. The documented arithmetic pattern keeps the conversions in one
expression; `unsafe.Add` keeps a pointer as the base. `runtime.KeepAlive` does
not relax those rules. See the full
[`unsafe.Pointer` contract](https://pkg.go.dev/unsafe#Pointer) and
[`KeepAlive` documentation](https://pkg.go.dev/runtime#KeepAlive).

## 5. A slice view borrows storage

`unsafe.Slice` needs an aligned pointer and enough valid elements for the
requested length. The walkthrough supplies an actual array's start and length.
The resulting slice has that length and capacity, and writes reach the array.
The function cannot establish the real size of an arbitrary allocation for you.

`SliceData(nil)` returns nil; `Slice(nil, 0)` returns a nil slice. A non-nil slice
with zero capacity has an unspecified data address, and an empty string's
`StringData` may be nil. A pointer returned for an empty value does not establish
an element that can be dereferenced. Modern helpers express these operations
without manufacturing `reflect.SliceHeader` or `reflect.StringHeader` values.

## 6–7. Strings require immutable backing bytes

`buildLabel` constructs private bytes and uses an ordinary string conversion.
`buildLabelUnsafe` constructs the same bytes and returns a string view. The
buffer is private, no mutable alias escapes, and later calls create separate
storage. Its string reference retains the backing allocation, including any
unused capacity. The view removes the final explicit copy; allocation and timing
results still depend on compiler decisions and the complete workload.

A buffer supplied by a caller or returned to a pool would allow later mutation,
breaking that string's contract. Sharing bytes through `unsafe.String` requires
that they remain immutable for the string's lifetime. `StringData` exposes the
same restriction in the other direction. The walkthrough's borrowed `[]byte`
is read-only by contract; use `[]byte(text)` for independent mutable bytes.
See [`String`](https://pkg.go.dev/unsafe#String) and
[`StringData`](https://pkg.go.dev/unsafe#StringData).

## Measure an equivalent retained result

```sh
go test -run='^$' -bench=. -benchmem -benchtime=100ms -count=3 ./chapters/16-reflect-unsafe-and-cgo/02-unsafe
```

Both [benchmarks](benchmark_test.go) build `task:42` and retain the result in
the same global sink. Each operation includes buffer construction, decimal
formatting, and final conversion; `b.Loop` excludes setup and the final check.
Report `ns/op`, `B/op`, and `allocs/op` for that operation. A removed copy can
trade transient work for retained spare capacity, so measurements do not supply
a universal reason to use unsafe. Compare repeated runs on the same machine
without instrumentation. See [benchmark guidance](../../15-testing/README.md).

A local Go 1.27.0 darwin/arm64 run with 100 ms samples and three repetitions
reported **8 B/op and 1 alloc/op for both variants**. Removing the final explicit
copy did not reduce the allocation count in this workload. These counters do
not measure how many bytes were copied; rerun the benchmark for your compiler
and use case.

## Further practice

1. Change field order and inspect the offsets on another target. Explain the
   padding you observe without making it a fixed serialization format.
2. Replace the two pointer helpers with ordinary typed access and keep their
   behavioral tests. Compare the clarity of each implementation.
3. Compare short and long label prefixes, then implement the operation with
   `strings.Builder`. Include ownership and retained memory in the comparison.
4. Explain why returning a string view of pooled bytes would conflict with
   reusing that buffer. Keep invalid lifetime examples out of executable code.
