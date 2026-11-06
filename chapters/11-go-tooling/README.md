# Go tooling

Read [modules, packages, and imports](../10-modules-and-packages/README.md) first.
All examples in this chapter use the root module and the standard library.

| Example | Purpose |
| --- | --- |
| [Build and analysis](01-build-and-analysis/main.go) | Compile a complete package, set version metadata, and inspect the artifact. |
| [Build tags](02-build-tags/main.go) | Select one implementation at compile time with complementary constraints. |
| [Embedded resources](03-embed/main.go) | Bundle static files into a binary through a string and `embed.FS`. |
| [Code generation](04-go-generate/main.go) | Generate formatted enum declarations from a maintained input file. |

Run the commands below from the repository root. Shell examples use a temporary
directory for binaries, keeping build artifacts out of the source tree.

## Build and inspect a package

```sh
go version
go env GOVERSION GOMOD GOOS GOARCH
go run ./chapters/11-go-tooling/01-build-and-analysis

tooling_dir=$(mktemp -d)
go build -o "$tooling_dir/tooling-demo" ./chapters/11-go-tooling/01-build-and-analysis
"$tooling_dir/tooling-demo"

go build -ldflags='-X main.version=v1.0.0' -o "$tooling_dir/tooling-release" ./chapters/11-go-tooling/01-build-and-analysis
"$tooling_dir/tooling-release"
go version -m "$tooling_dir/tooling-release"
```

Keep `tooling_dir` in the same shell for later commands. The default command
prints `version: development`; the linker-configured binary prints
`version: v1.0.0`. `version` is a string variable, which allows the linker's `-X`
option to set it. Build metadata identifies an artifact; it is separate from the
module's semantic version.

`main.go` calls `targetDescription` from `target.go`. Passing the package path
lets Go select its files, including build constraints and generated declarations.
Running only `main.go` omits its companion file. Prefer package paths for these
lessons.

| Command | What it does |
| --- | --- |
| `go run PACKAGE` | Builds and runs a command using a temporary executable. |
| `go build -o PATH PACKAGE` | Builds a named executable that can be run later. |
| `go build ./...` | Checks the root module's packages; with multiple packages, no named executables are copied into the repository. |
| `go install PACKAGE` | Builds and places a command in `GOBIN`, or the default Go binary directory. |
| `go version -m BINARY` | Inspects available Go build and module metadata in an artifact. |

To try installation without changing your regular Go binary directory:

```sh
mkdir -p "$tooling_dir/bin"
GOBIN="$tooling_dir/bin" go install ./chapters/11-go-tooling/01-build-and-analysis
"$tooling_dir/bin/01-build-and-analysis"
```

`go install example.com/some/command@VERSION` is the syntax for installing a
published command at a selected version; replace that illustrative path and
version with real values. `go get` manages dependencies rather than installing
commands. The local install above needs no downloads.

For a pure-Go cross-build:

```sh
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$tooling_dir/tooling-linux-amd64" ./chapters/11-go-tooling/01-build-and-analysis
go version -m "$tooling_dir/tooling-linux-amd64"
```

This produces a Linux/amd64 binary. Run it on a compatible target; inspecting
its metadata does not require running it. OS-specific dependencies and cgo can
introduce additional requirements in other programs.

## Format, analyze, and test

These tools answer different questions:

| Tool | Purpose |
| --- | --- |
| `gofmt -l DIRECTORY` | Lists Go source needing standard formatting, without changing it. |
| `gofmt -w FILE` | Applies formatting to a source file. |
| `go fmt PACKAGES` | Formats source files selected for the specified packages. |
| `go build PACKAGES` | Compiles and links, checking language and type rules. |
| `go vet PACKAGES` | Reports suspicious constructs through static analyzers, such as mismatched `Printf` arguments. |
| `go test PACKAGES` | Builds and runs tests and executable examples. |

```sh
gofmt -l chapters/11-go-tooling
go vet ./chapters/11-go-tooling/...
go test ./chapters/11-go-tooling/...
go tool vet help
```

Formatting is independent of which source files happen to be selected for one
build. `gofmt -l` over the directory also inspects the alternate tagged file.
Static analyzers find classes of mistakes without executing the program; review
their reports alongside compilation and behavioral checks. The existing
[pointer benchmarks](../06-pointers/02-pointer-performance/README.md) demonstrate
measurement, and chapter 15 will develop testing techniques further.

## Select an implementation with build tags

```sh
go run ./chapters/11-go-tooling/02-build-tags
go run -tags=diagnostic ./chapters/11-go-tooling/02-build-tags
go list -f 'selected={{.GoFiles}} ignored={{.IgnoredGoFiles}}' ./chapters/11-go-tooling/02-build-tags
go list -tags=diagnostic -f 'selected={{.GoFiles}} ignored={{.IgnoredGoFiles}}' ./chapters/11-go-tooling/02-build-tags
```

The default run prints `build mode: standard`; the tagged run prints
`build mode: diagnostic`. The two source files provide the same `buildMode`
function with complementary constraints:

```go
//go:build !diagnostic
```

```go
//go:build diagnostic
```

Constraints belong near the top of a file, before `package`, followed by a blank
line. `!`, `&&`, and `||` compose conditions. Exactly one implementation is selected
in this example, so the package has neither duplicate declarations nor a missing
implementation. `diagnostic` is a custom tag, with no built-in debugging behavior.

Use build constraints when a build must select different source implementations,
such as platform support or an optional compiled feature. Use ordinary runtime
configuration for choices that must change after the program starts. Place Go's
`-tags` flag before the package path in `go run`; arguments after the package
belong to the program.

Names such as `storage_linux.go` or `storage_windows_amd64.go` also impose implicit
OS/architecture constraints. The `_test.go` suffix marks test source. Ordinary
Go package builds skip source names starting with `.` or `_`; our generator
instead lives in its own `internal/cmd/statusgen` package so it can be tested.

Validate both configurations:

```sh
go test ./chapters/11-go-tooling/...
go test -tags=diagnostic ./chapters/11-go-tooling/...
go vet -tags=diagnostic ./chapters/11-go-tooling/...
```

## Bundle static resources with embed

```sh
go run ./chapters/11-go-tooling/03-embed
go build -o "$tooling_dir/embedded-demo" ./chapters/11-go-tooling/03-embed
(cd "$tooling_dir" && ./embedded-demo)
```

The binary still prints its welcome text, help, and file listing when run from
the temporary directory. It reads the embedded filesystem rather than looking
for an `assets` folder in the process's working directory.

| Storage | Use |
| --- | --- |
| `string` | One fixed text file; strings are immutable. The example uses it for `welcome.txt`. |
| `[]byte` | One resource whose in-memory bytes the program may modify. |
| `embed.FS` | Multiple read-only resources accessed by name. The example selects `assets/*.txt`. |

`//go:embed` initializes a package-level variable at compile time. Patterns are
relative to the source package directory and use forward slashes. Each pattern
must match a file or nonempty directory; invalid patterns or missing matches fail
the build. A string or byte-slice variable must select exactly one file. Patterns
cannot use `..` to reach outside the package or include another module's files or
symbolic links.
Even the string/byte forms require importing `embed`; a blank import is used if
the file does not otherwise refer to `embed.FS`.

Choose embedding for static assets shipped with an artifact. Rebuild after
changing an asset; editing its source file does not update an already compiled
binary. Use runtime file/configuration access when contents must change without
rebuilding. The I/O and HTTP chapters will extend the use of filesystem interfaces.

## Generate maintained source explicitly

Read [statuses.txt](04-go-generate/statuses.txt), the directive in
[status.go](04-go-generate/status.go), the
[local generator](04-go-generate/internal/cmd/statusgen/main.go), and its
[generated output](04-go-generate/status_names_gen.go) in that order.

```sh
go generate -n ./chapters/11-go-tooling/04-go-generate
go generate ./chapters/11-go-tooling/04-go-generate
go run ./chapters/11-go-tooling/04-go-generate
go test ./chapters/11-go-tooling/04-go-generate/...
```

`-n` shows the directive's command without executing it. The actual command runs
from the containing package's directory, so its input, output, and generator
paths resolve there. It uses only the standard library, including `go/token` for
identifier validation and `go/format` for generated source formatting.

The input contains one identifier suffix per line, with blank lines and `#`
comments allowed. The tool rejects duplicate names, keywords, `_`, invalid
identifiers, and an empty list. It validates the complete input before writing,
so invalid input preserves an existing output file. Input order determines the
`iota` values. Use explicit stable IDs when values must be stored or exchanged;
this exercise derives numbers from line order.

Use generation when repetitive declarations can be derived from a maintained
specification. Here it produces enum constants and their name table; the handwritten
`String` method preserves numeric details for unknown values. Generated output has
the standard `Code generated ... DO NOT EDIT.` header and contains no timestamps
or machine-dependent paths. Repeated generation with unchanged input produces
the same bytes.

`go build` and `go test` do not run `go generate`. The generated file is included
in the repository so a fresh checkout builds immediately. After editing the input
or generator, regenerate, review the output diff, and run the tests before
committing the input, generator, and generated source together. Regenerating an
unchanged committed output should leave that file's Git diff empty.

Further practice:

1. Add a status, regenerate, and inspect which declarations and numeric values change.
2. Build both modes as separate binaries and compare their runtime output.
3. Add an embedded resource and explain whether the current glob selects it.
4. Automate these checks in CI, including the nested modules introduced in chapter 10.

References: [Go command](https://pkg.go.dev/cmd/go),
[build constraints](https://pkg.go.dev/cmd/go#hdr-Build_constraints),
[vet](https://pkg.go.dev/cmd/vet), [embed](https://pkg.go.dev/embed), and
[code generation](https://go.dev/blog/generate).
