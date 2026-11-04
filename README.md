# Go course

Go examples and practice projects organized around Jon Bodner's
[Learning Go, Second Edition](https://www.oreilly.com/library/view/learning-go-2nd/9781098139285/).

The directories under `chapters/` follow the book's chapter numbers. Lessons
within a chapter introduce the prerequisites used by its practice project.
Each lesson and project is a separate `main` package, with its tests alongside it.

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
| 3. Composite types | [Arrays](chapters/03-composite-types/01-arrays/main.go), [slices](chapters/03-composite-types/02-slices/main.go), [slice internals](chapters/03-composite-types/03-slice-internals/main.go), [maps](chapters/03-composite-types/04-maps/main.go), [structs](chapters/03-composite-types/05-structs/main.go) |
| 4. Blocks, shadows, and control structures | [Shadowing](chapters/04-blocks-and-control-flow/01-shadowing/main.go), [conditionals](chapters/04-blocks-and-control-flow/02-conditionals/main.go), [switch](chapters/04-blocks-and-control-flow/03-switch/main.go), [for loops](chapters/04-blocks-and-control-flow/04-for-loops/main.go), [goto](chapters/04-blocks-and-control-flow/05-goto/main.go) |
| 5. Functions | [Functions](chapters/05-functions/01-functions/main.go), [multiple return values](chapters/05-functions/02-multiple-return-values/main.go), [variadic functions](chapters/05-functions/03-variadic-functions/main.go), [function values, closures, and recursion](chapters/05-functions/04-function-values-closures-and-recursion/main.go), [defer](chapters/05-functions/05-defer/main.go) |
| 6. Pointers | [Pointers](chapters/06-pointers/01-pointers/main.go), [pointer performance](chapters/06-pointers/02-pointer-performance/main.go), [garbage collector](chapters/06-pointers/03-garbage-collector/main.go) |
| 7. Types, methods, and interfaces | [Enums](chapters/07-types-methods-and-interfaces/01-enums/main.go), [methods](chapters/07-types-methods-and-interfaces/02-methods/main.go), [interfaces](chapters/07-types-methods-and-interfaces/03-interfaces/main.go), [Stringer](chapters/07-types-methods-and-interfaces/04-stringer/main.go) |
| 8. Generics | [Generics](chapters/08-generics/01-generics/main.go) |
| 9. Errors | [Error handling](chapters/09-errors/01-error-handling/main.go), [panic and recover](chapters/09-errors/02-panic-and-recover/main.go) |
| 12. Concurrency in Go | [Concurrency](chapters/12-concurrency/01-concurrency/main.go) |

The performance and GC lessons provide additional practice after pointer basics.
The concurrency lesson includes a short introduction to `context`; a dedicated
context chapter is planned below.

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

Run the concurrency example with the race detector:

```sh
go run -race ./chapters/12-concurrency/01-concurrency
```

Run the pointer benchmarks explicitly:

```sh
go test -run='^$' -bench=. -benchmem ./chapters/06-pointers/02-pointer-performance
```

## Planned chapters and extensions

The gaps in chapter numbering leave room for the remaining units:

- Chapter 10: modules, packages, and imports.
- Chapter 11: Go tooling.
- Chapter 13: I/O, time, JSON, HTTP, and structured logging.
- Chapter 14: context propagation, values, cancellation, and deadlines.
- Chapter 15: writing tests.
- Chapter 16: reflection, unsafe, and cgo.

Further exercises will extend concurrency with pipelines, fan-out, fan-in,
bounded worker pools, and cancellation.
