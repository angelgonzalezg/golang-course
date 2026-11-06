# Modules, packages, and imports

Read the types, methods, and errors chapters first. This chapter has two runnable
lessons:

| Lesson | What it demonstrates |
| --- | --- |
| [Packages and imports](01-packages-and-imports/main.go) | A command using a library, exported names, private implementation, and an `internal` helper. |
| [Module versions](02-module-versions/README.md) | An isolated app requiring two major versions through local replacements. |

## Modules and packages are different boundaries

| Concept | Meaning in this course |
| --- | --- |
| Module | A collection of packages rooted at a `go.mod` file. The main course module is named `golangcourse`. |
| Package | Go source files compiled together from one directory. Its `package` declaration defines its name. |
| Import path | The module path followed by a package's directory path within that module. |
| Command | A `package main` with a `main` function, built as an executable. Library packages expose APIs for other packages to import. |

For example, the first lesson imports:

```go
import "golangcourse/chapters/10-modules-and-packages/01-packages-and-imports/greeting"
```

This resolves using the repository's existing [go.mod](../../go.mod), without a
download. Importing another package in the same module does not require a new
`require` entry. The local module name `golangcourse` works for this course;
a published module normally uses a repository-based path that Go can locate.

Use module import paths rather than relative imports such as `./greeting`.
Directories organize source, but they do not automatically import their child
directories. Ordinary files in a package use the same package name. Tests may
instead use an external package such as `greeting_test` to exercise the public API.

## Read and run the package example

The first lesson has this structure, all within the root module:

```text
01-packages-and-imports/
  main.go                  package main: calls the public API
  greeting/
    doc.go                 package documentation
    language.go            exported constants and private helpers
    greeter.go             exported type, constructor, method, and errors
    greeter_test.go         package greeting_test: public API tests
  internal/textutil/
    name.go                helper imported by greeting
    name_test.go
```

Run from the repository root:

```sh
go run ./chapters/10-modules-and-packages/01-packages-and-imports
go test ./chapters/10-modules-and-packages/01-packages-and-imports/...
go list ./chapters/10-modules-and-packages/01-packages-and-imports/...
go doc golangcourse/chapters/10-modules-and-packages/01-packages-and-imports/greeting
```

The command prints an English greeting using a zero-value `Greeter`, a Spanish
greeting from `New(Spanish)`, and checks the library's exported errors. Names have
Unicode whitespace collapsed; this is whitespace cleanup, not Unicode normalization.

## Visibility and explicit initialization

- `Greeter`, `Language`, `English`, `Spanish`, `New`, `Greet`, and the error values
  are exported. Their uppercase names allow other packages to access them.
- `Greeter.prefix`, `defaultPrefix`, and `prefixFor` are unexported. All files in
  package `greeting` can use them; importing packages cannot. Private names belong
  to the package, rather than to an individual source file.
- `Greeter` is a small configuration value. Its usable zero value greets in
  English; `New` selects a supported language and reports invalid input.
- Library code returns errors and results. The command decides what to print.

Package dependencies initialize before their importers. Package-level variables
and `init` functions run before `main`. Prefer an explicit constructor for work
that needs configuration or can fail, so the caller controls that work. This
lesson requires no `init` hooks or hidden I/O during import.

Imports normally introduce a file-local package name. Unused imports are compile
errors. An alias changes the identifier used in that file, as the versioning lab
demonstrates. A blank import (`_`) requests initialization side effects without
using an API; reserve it for APIs that deliberately require registration. A dot
import brings exported names into the current file's scope and obscures their
origin; keep qualified package names in these examples.

## The internal boundary

`CleanName` is exported so another package can call it. The `internal` directory
adds a separate restriction: an importing package must be within the tree rooted
at the parent of that `internal` directory. Here that parent is
`01-packages-and-imports`, not the repository root.

The command and `greeting` package are inside that tree. Another lesson may
import the public `greeting` package, but cannot directly import this lesson's
`internal/textutil`, even though both lessons share the root module. Its helper
is an implementation detail that can change without becoming part of the public
greeting API.

The import direction is `main -> greeting -> internal/textutil`. Import cycles
are rejected by Go. Keep dependencies directed toward the packages doing the
underlying work instead of making the library import its command.

## Continue with dependencies and versions

The [module-version lab](02-module-versions/README.md) adds three nested modules.
Root `go test ./...` covers the package lesson above but stops at those nested
module boundaries. The lab documents how to run, test, and inspect them separately.

Further practice:

1. Add another language by changing the library and its public API tests.
2. Add a second command that imports `greeting` without copying its source.
3. Explain which callers would break if you exported the internal helper as part
   of the public library and later changed its signature.
4. Explore a `go.work` file in a separate exercise to coordinate local modules;
   keep its effect on module selection explicit.

References: [module layout](https://go.dev/doc/modules/layout),
[developing modules](https://go.dev/doc/modules/developing), and the
[modules reference](https://go.dev/ref/mod).
