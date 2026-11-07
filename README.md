# Go course

Go examples and practice projects organized around Jon Bodner's
[Learning Go, Second Edition](https://www.oreilly.com/library/view/learning-go-2nd/9781098139285/).

The directories under `chapters/` follow the book's chapter numbers. Lessons
within a chapter introduce the prerequisites used by its practice project.
Most lessons and projects are separate `main` packages, with tests alongside them.
Chapter 10 also introduces library packages and an isolated lab with nested modules.

## Requirements

Use Go 1.27 or later, as declared in `go.mod`. Run the commands below from the
repository root.

## Run an example

```sh
go run ./chapters/02-declarations/01-values
go run ./chapters/05-functions/06-project-sales-order
```

Change the package path to run another lesson or project from the tables below.

## Current lessons

| Book chapter | Topics, in reading order |
| --- | --- |
| 2. Predeclared types and declarations | [Values](chapters/02-declarations/01-values/main.go), [variables](chapters/02-declarations/02-variables/main.go), [constants](chapters/02-declarations/03-constants/main.go) |
| 3. Composite types | [Arrays](chapters/03-composite-types/01-arrays/main.go), [slices](chapters/03-composite-types/02-slices/main.go), [slice internals](chapters/03-composite-types/03-slice-internals/main.go), [strings, bytes, and runes](chapters/03-composite-types/04-strings-bytes-and-runes/README.md), [maps](chapters/03-composite-types/05-maps/main.go), [structs](chapters/03-composite-types/06-structs/main.go) |
| 4. Blocks, shadows, and control structures | [Shadowing](chapters/04-blocks-and-control-flow/01-shadowing/main.go), [conditionals](chapters/04-blocks-and-control-flow/02-conditionals/main.go), [switch](chapters/04-blocks-and-control-flow/03-switch/main.go), [for loops](chapters/04-blocks-and-control-flow/04-for-loops/main.go), [goto](chapters/04-blocks-and-control-flow/05-goto/main.go) |
| 5. Functions | [Functions](chapters/05-functions/01-functions/main.go), [multiple return values](chapters/05-functions/02-multiple-return-values/main.go), [variadic functions](chapters/05-functions/03-variadic-functions/main.go), [function values, closures, and recursion](chapters/05-functions/04-function-values-closures-and-recursion/main.go), [defer](chapters/05-functions/05-defer/main.go) |
| 6. Pointers | [Pointers](chapters/06-pointers/01-pointers/main.go), [pointer performance](chapters/06-pointers/02-pointer-performance/README.md), [garbage collector](chapters/06-pointers/03-garbage-collector/README.md) |
| 7. Types, methods, and interfaces | [Enums](chapters/07-types-methods-and-interfaces/01-enums/main.go), [methods](chapters/07-types-methods-and-interfaces/02-methods/main.go), [interfaces](chapters/07-types-methods-and-interfaces/03-interfaces/main.go), [Stringer](chapters/07-types-methods-and-interfaces/04-stringer/main.go) |
| 8. Generics | [Generics](chapters/08-generics/01-generics/main.go) |
| 9. Errors | [Error handling](chapters/09-errors/01-error-handling/main.go), [panic and recover](chapters/09-errors/02-panic-and-recover/main.go) |
| 10. Modules, packages, and imports | [Package boundaries, exports, and internal helpers](chapters/10-modules-and-packages/README.md), [dependency versions and local modules](chapters/10-modules-and-packages/02-module-versions/README.md) |
| 11. Go tooling | [Builds and analysis, build tags, embedded resources, and code generation](chapters/11-go-tooling/README.md) |
| 12. Concurrency in Go | [Concurrency fundamentals, ownership, and shutdown](chapters/12-concurrency/01-concurrency/README.md) |
| 13. The standard library | [I/O, buffering, files, and filesystem interfaces](chapters/13-standard-library/01-io-and-files/README.md), [time, calendars, timers, and tickers](chapters/13-standard-library/02-time/README.md), [JSON, field semantics, and validation](chapters/13-standard-library/03-json/README.md), [HTTP handlers, clients, and shutdown](chapters/13-standard-library/04-http/README.md), [structured logging with slog](chapters/13-standard-library/05-structured-logging/README.md) |
| 14. The context | [Propagation, cancellation, deadlines, request values, and lifecycle](chapters/14-context/01-context/README.md) |

The performance and GC lessons provide additional practice after pointer basics.
The concurrency lesson introduces `context`; chapter 14 extends it with deadlines,
propagation across API boundaries, request values, and explicit lifetime ownership.

## Practice projects

| Project | Book chapter | Example |
| --- | --- | --- |
| Sales order processor | 5. Functions | [Run after the function lessons](chapters/05-functions/06-project-sales-order/README.md) |
| Contact book | 6. Pointers | [Run after pointer basics](chapters/06-pointers/04-project-contact-list/README.md) |
| Log levels | 7. Types, methods, and interfaces | [Run after Stringer](chapters/07-types-methods-and-interfaces/05-project-log-levels/README.md) |
| Math helpers with errors | 9. Errors | [Run after error handling](chapters/09-errors/03-project-math-helpers/README.md) |

## Validate the course

```sh
go test ./...
go vet ./...
```

Check chapter 11's alternate build configuration:

```sh
go test -tags=diagnostic ./chapters/11-go-tooling/...
go vet -tags=diagnostic ./chapters/11-go-tooling/...
```

The chapter 10 versioning lab has three nested modules. Validate them separately;
the root commands above stop at nested module boundaries:

```sh
go -C chapters/10-modules-and-packages/02-module-versions/app test ./...
go -C chapters/10-modules-and-packages/02-module-versions/greeting-v1 test ./...
go -C chapters/10-modules-and-packages/02-module-versions/greeting-v2 test ./...
go -C chapters/10-modules-and-packages/02-module-versions/app vet ./...
go -C chapters/10-modules-and-packages/02-module-versions/greeting-v1 vet ./...
go -C chapters/10-modules-and-packages/02-module-versions/greeting-v2 vet ./...
```

Validate the concurrency lesson with the race detector:

```sh
go test -race -timeout=30s ./chapters/12-concurrency/01-concurrency
go run -race ./chapters/12-concurrency/01-concurrency
```

Validate the dedicated context lesson, including its local HTTP cancellation test:

```sh
go test -race -timeout=30s ./chapters/14-context/01-context
```

Run the pointer and GC benchmarks explicitly:

```sh
go test -run='^$' -bench=. -benchmem ./chapters/06-pointers/02-pointer-performance
go test -run='^$' -bench=. -benchmem ./chapters/06-pointers/03-garbage-collector
```

## Planned chapters and extensions

The remaining units cover:

- Chapter 15: writing tests.
- Chapter 16: reflection, unsafe, and cgo.

Further exercises will extend the finite pipeline and worker pool with fan-out,
fan-in, cancellation, error propagation, and backpressure.
