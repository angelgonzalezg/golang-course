# Development environment: your first run

Prepare the tools you need for Idiomatic Go Labs, run a small program, and check
an existing test. You can follow this guide before learning Go syntax. Later
chapters explain the code and tools in more detail.

By the end, you should be able to locate the repository root, confirm the Go
version and active module, run a lesson, and recognize a successful test.

## 1. Install Go and prepare an editor

This repository requires **Go 1.27 or later**, as declared in
[the root go.mod](../../go.mod). Follow the official
[Go installation instructions](https://go.dev/doc/install) for your operating
system. Select the download matching your computer's architecture; for example,
Apple Silicon uses arm64, while an Intel Mac uses amd64.

Open a new terminal after installation and run:

```sh
go version
```

One example of a compatible result is:

```text
go version go1.27.0 darwin/arm64
```

The version, operating system, and architecture describe your own installation;
the final part will differ on Linux, Windows, or another architecture. Check
that the reported version meets the module's requirement.

Open the repository folder in your preferred editor. Go support is useful for
syntax highlighting, formatting, navigation, and diagnostics. Configure the
editor to use the same Go installation as your terminal; if their results differ,
check the editor's Go configuration and restart it after updating Go.

For example, with Visual Studio Code:

1. Install the Go extension described by the
   [official VS Code Go project](https://github.com/golang/vscode-go).
2. Open the `idiomatic-go-labs` folder, then open
   [the values lesson](../02-declarations/01-values/main.go).
3. Follow any prompt to install missing Go tools. You can also run
   `Go: Install/Update Tools` from the Command Palette and select `gopls`.

`gopls` is Go's language server: it supplies features such as code completion,
navigation, and diagnostics. The extension's
[tools guide](https://github.com/golang/vscode-go/blob/master/docs/tools.md)
explains how it finds Go and manages these tools. Other editors can provide
similar Go support; the terminal commands below work independently of the editor.

Git is useful for obtaining the repository and reviewing your practice changes.
An existing downloaded copy can also be used for the commands in this guide.
The root lessons use Go's standard library. The separate gRPC lab downloads its
pinned dependencies when first used; the optional cgo lab needs a C compiler.
The PostgreSQL lab also has pinned dependencies. Its demo and integration tests
need a running container engine; its unit tests work without Docker.

## 2. Find the repository root

Download or clone the repository, then open a terminal in its `idiomatic-go-labs`
folder. If your terminal is in the folder containing that directory, enter it:

```sh
cd idiomatic-go-labs
```

The repository root contains `README.md`, `go.mod`, `chapters`, `docs`, and
`extensions`. Run the remaining commands in this guide from that root.
The commands work in a macOS/Linux shell or Windows PowerShell.

Ask Go which module file applies to the current directory:

```sh
go env GOMOD
```

The result should be the full path to this repository's root `go.mod`, ending
in `idiomatic-go-labs/go.mod` (with backslashes on Windows). If it points to a
nested lab's file, return to the repository root first.

A **package** groups Go source files in a directory. A **module** groups packages
under a `go.mod` file, which declares their import path prefix and Go version
requirement. Read the official
[code organization guide](https://go.dev/doc/code#Code_organization) for the
general model; chapter 10 explores it with examples.

Here the folder is named `idiomatic-go-labs`, while the root module's import
path is `golangcourse`. The folder name locates your checkout on disk; the module
path identifies packages in Go imports. Renaming the checkout folder does not
change that declared import path. The repository already contains its module
files, so you can use the existing packages directly.

## 3. Run your first example

```sh
go run ./chapters/02-declarations/01-values
```

`go run` compiles the selected package and executes its `main` function. The
`./` starts the package path at your current directory. Selecting the package
also includes its companion Go files when a lesson has more than one.

The first lines should be:

```text
== 1. String values ==
Hello, Go!
Golang
line one
line two
```

The lesson prints additional numbered sections and then exits. In
[main.go](../02-declarations/01-values/main.go), find
`fmt.Println("Hello, Go!")` and connect that line to the displayed greeting.
For a small experiment, change the greeting in your editor, save, and run the
same command again. Predict which output line will change. Restore the greeting
afterward so the next reading matches the original example.

Begin with the string section; the later sections preview types you will study
in the [learning path](../../docs/learning-guide.md).

## 4. Run your first test

A test checks an expected behavior automatically. Run one existing test by name:

```sh
go test -v -count=1 -run '^TestNormalize$' ./chapters/15-testing/01-unit-tests
```

This test calls `Normalize` with `" Go "` and expects `"go"` with no error. Read
the first test in [labels_test.go](../15-testing/01-unit-tests/labels_test.go)
beside the implementation in [labels.go](../15-testing/01-unit-tests/labels.go).
You are practicing running a test here; chapter 15 teaches how to design one.

Look for these result lines, followed by an `ok` package summary:

```text
=== RUN   TestNormalize
--- PASS: TestNormalize
PASS
```

The actual output also includes elapsed times. `-v` shows test names,
`-count=1` requests a fresh run, and `-run` selects the named test. Without
`-run`, Go runs all tests in that package:

```sh
go test -count=1 ./chapters/15-testing/01-unit-tests
```

An `ok` result means that package's selected tests passed. A package reporting
`[no test files]` has no tests of its own; that message is not a test failure.
For example, the values lesson is a runnable demonstration without a test file.

## 5. Understand which commands cover which modules

From the repository root, check the main course with:

```sh
go test -count=1 -timeout=60s ./...
go vet ./...
gofmt -l chapters extensions
```

| Command | What to observe |
| --- | --- |
| `go test` | Compiles packages and runs their tests. `./...` includes packages below the current directory within the active module. |
| `go vet` | Reports selected suspicious code patterns. Successful checks normally print nothing. |
| `gofmt -l` | Lists Go files whose formatting differs from Go's standard format; a clean result prints nothing. It does not edit the files. |

The main course's `go test ./...` and `go vet ./...` stop at nested module
boundaries. Formatting is a separate file scan and includes both named trees.
The independent labs are:

| Module location | Role |
| --- | --- |
| [Chapter 10 app](../10-modules-and-packages/02-module-versions/app/go.mod) | An application using the local versioned libraries. |
| [Chapter 10 greeting v1](../10-modules-and-packages/02-module-versions/greeting-v1/go.mod) | The first library module. |
| [Chapter 10 greeting v2](../10-modules-and-packages/02-module-versions/greeting-v2/go.mod) | Its separate major-version module. |
| [Chapter 16 cgo](../16-reflect-unsafe-and-cgo/03-cgo/README.md) | Optional Go/C interoperation with a C compiler. |
| [Protobuf/gRPC](../../extensions/01-protobuf-and-grpc/README.md) | The supplementary lab with its own external dependencies. |
| [PostgreSQL/database/sql](../../extensions/02-database-sql/README.md) | SQL access with separate unit and container-based integration tests. |

Use `go -C` to select a lab directory for one command while keeping your terminal
at the repository root. For example:

```sh
go -C extensions/01-protobuf-and-grpc run ./01-protobuf
go -C extensions/01-protobuf-and-grpc test -count=1 -timeout=30s ./...
```

The first command runs a local serialization demonstration; the second checks
the entire gRPC module. Its generated Go files are checked in, so these commands
can run without regenerating them. Return to that lab after REST, context, and
testing; these commands illustrate its module boundary for now.

Use the [validation guide](../../docs/validation.md) when you need the full
procedure for every independent module and the specialized checks.

## If a command does not work

| Symptom | What to check |
| --- | --- |
| The terminal cannot find `go`. | Reopen the terminal after installation and follow the official installer's PATH instructions for your operating system. |
| Go reports that the required version is newer. | Compare `go version` with `go.mod`. Check `go env GOTOOLCHAIN` if you are investigating automatic toolchain selection. |
| Go cannot find the module, or `go env GOMOD` does not identify the expected file. | Open the terminal in the repository root, then rerun the package command. |
| `go run .` at the repository root finds no Go files. | Choose a lesson package, such as `./chapters/02-declarations/01-values`. The root is an index containing several lesson packages. |
| A package is not a main package. | Library lessons, including chapter 15, are exercised with `go test`. Use `go run` for executable lessons. |
| A dependency or toolchain download fails. | Check the connection and any configured module proxy. Retry the same command once access is restored. |

Go can select or download a suitable toolchain according to its module
requirements and `GOTOOLCHAIN` setting. See the official
[toolchain selection guide](https://go.dev/doc/toolchain) when diagnosing a
version mismatch; this course declares its requirement in `go.mod`.

## Continue with the course

Before continuing, confirm that you can explain what `go run` and `go test`
do, identify the active module, and name the extra labs that need separate
commands. Then follow the [learning guide](../../docs/learning-guide.md), starting
with the [values lesson](../02-declarations/01-values/main.go).
