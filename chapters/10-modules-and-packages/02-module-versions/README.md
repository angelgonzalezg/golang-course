# Dependency versions with local modules

This lab has three modules, each with its own `go.mod`:

| Directory | Module path | Role |
| --- | --- | --- |
| [app](app/go.mod) | `example.com/course/greeting-app` | Command importing both major versions. |
| [greeting-v1](greeting-v1/go.mod) | `example.com/course/greeting` | V1 library: `Hello(name) string`, with a fallback for blank names. |
| [greeting-v2](greeting-v2/go.mod) | `example.com/course/greeting/v2` | V2 library: `Hello(name) (string, error)`, rejecting blank names. |

These are local teaching modules, not published releases. The app's version
labels demonstrate module metadata; local replacements supply the actual source.
Everything runs without downloading a third-party dependency. There is no root
`go.work` file involved in selecting the lab's modules.

## Run and validate

Run these commands from the repository root. The `-C` option changes the working
directory before Go selects the module:

```sh
go -C chapters/10-modules-and-packages/02-module-versions/app run .
go -C chapters/10-modules-and-packages/02-module-versions/app test ./...
go -C chapters/10-modules-and-packages/02-module-versions/greeting-v1 test ./...
go -C chapters/10-modules-and-packages/02-module-versions/greeting-v2 test ./...
```

Run `vet ./...` in the same three module directories for static checks. Testing
the app compiles its library dependencies, but does not run their own test suites;
the two library test commands are therefore necessary. The root module's
`go test ./...` also does not traverse these nested modules.

The app prints `Hello, friend!` for a blank v1 name and reports
`blank name rejected: true` for the same input through v2. Both imports remain
usable in one program because their full module/import paths are different.

## Require a version and replace its source

The app's `go.mod` contains:

```go
require (
    example.com/course/greeting v1.2.0
    example.com/course/greeting/v2 v2.0.0
)

replace example.com/course/greeting => ../greeting-v1
replace example.com/course/greeting/v2 => ../greeting-v2
```

`require` adds a minimum version requirement for a module path. `replace` chooses
different source for a required module; it does not add a requirement by itself.
These replacements apply to all versions of their respective paths, since no
version appears on the left side. Relative replacement directories are resolved
from the `go.mod` containing the directives and must contain their own `go.mod`.

The app still imports `example.com/course/greeting/v2`, not `../greeting-v2`.
The module path in the replacement's `go.mod` matches the requested module path.
The `v1.2.0` label does not freeze files in `greeting-v1`: editing that local
directory immediately changes the source used by the app.

Replacement directives are controlled by the main module or active workspace;
they are not automatically inherited from a dependency's `go.mod`. The replacements
here belong in the app, where both modules are consumed.

## Major versions and aliases

For ordinary module paths, versions v0 and v1 use the base path. Starting at v2,
the major version becomes a path suffix such as `/v2`. Updating only the version
number while leaving the v1 import path does not migrate a caller to v2.

Both library source files still declare `package greeting`. The app uses aliases
`greetingv1` and `greetingv2` to resolve the file-local name collision. Import
aliases do not select versions, change module paths, or rename the libraries.

| Release change | Compatibility expectation |
| --- | --- |
| Patch, such as v1.2.0 to v1.2.1 | Compatible fixes. |
| Minor, such as v1.2.0 to v1.3.0 | Compatible additions. |
| Major, such as v1 to v2 | May introduce incompatible API changes; migrate callers explicitly. |

V0 APIs are still under development and may change incompatibly. In this lab,
adding an error result to `Hello` breaks v1 callers, so the new API uses `/v2`.
Published module versions normally correspond to release tags; the local version
labels in this lab do not simulate publishing or retrieving those tags.

## Inspect the graph and module metadata

```sh
go -C chapters/10-modules-and-packages/02-module-versions/app list -m all
go -C chapters/10-modules-and-packages/02-module-versions/app mod graph
go -C chapters/10-modules-and-packages/02-module-versions/app mod why -m example.com/course/greeting/v2
go -C chapters/10-modules-and-packages/02-module-versions/app mod tidy -diff
```

`list -m all` shows both required versions and their local replacements. `mod graph`
shows module requirements, while `mod why` explains why a dependency is needed.
`mod tidy -diff` reports any required metadata changes without applying them;
an empty diff means the metadata is already tidy.

Go uses minimal version selection: for each module path, it selects the highest
version required by the loaded module graph. For example, a direct requirement
on `A@v1.2.0` plus a dependency requiring `A@v1.4.0` selects v1.4.0. It does not
automatically choose a newer v1.5.0 just because that release exists. This lab
shows direct requirements and major-version coexistence; adding a transitive
dependency is a further exercise rather than an existing graph conflict.

The `go` directive in each file declares the required Go version and language
semantics. It is separate from the library's semantic version.

`go.sum` records checksums of dependency module metadata and content obtained
through module downloads. It is not a lock file. This lab needs no `go.sum`
because its dependencies are local directories and the libraries use only the
standard library. In a module with downloaded dependencies, commit the generated
`go.mod` and `go.sum` together.

For a real published dependency, `go get MODULE@VERSION` adds or changes the
requirement; substitute an actual module path and version for those placeholders.
`go mod tidy` updates requirements and checksums to match imports and tests.
Review those file changes as part of a dependency update. An import uses a package
path; version selection belongs in module metadata, rather than an `@version`
suffix in a Go source import.

## Further practice

1. Change v2 handling in the app and observe that its v1 caller keeps compiling.
2. Add a second package to one library module, then derive its full import path.
3. In a disposable copy, add a local dependency with a higher minimum v1
   requirement and inspect how `go mod tidy` and `go list -m all` reflect it.

References: [modules reference](https://go.dev/ref/mod),
[major-version updates](https://go.dev/doc/modules/major-version), and
[module version numbers](https://go.dev/doc/modules/version-numbers).
