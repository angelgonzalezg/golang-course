# Log levels project

Practice `iota`, defined types, arrays, methods, `fmt.Stringer`, string handling,
and the `(value, ok)` return pattern. Read the enums, methods, interfaces, and
Stringer lessons in this chapter first.

## Run and test

Run these commands from the repository root:

```sh
go run ./chapters/07-types-methods-and-interfaces/05-project-log-levels
go test ./chapters/07-types-methods-and-interfaces/05-project-log-levels
```

## Supported levels

| Value | Constant | Display name |
| --- | --- | --- |
| 0 | `LevelTrace` | `Trace` |
| 1 | `LevelDebug` | `Debug` |
| 2 | `LevelInfo` | `Info` |
| 3 | `LevelWarn` | `Warn` |
| 4 | `LevelError` | `Error` |
| 5 | `LevelFatal` | `Fatal` |

The zero value of `LogLevel` is `LevelTrace`. `IsValid` checks membership in this
set; it does not tell whether a value was explicitly configured. Formatting a
level displays text, including `Fatal`, without taking any logging action.

## Input and output rules

- `String` formats valid levels by name and preserves an invalid value's number,
  such as `LogLevel(-1)` or `LogLevel(99)`.
- `ParseLogLevel` accepts these names with any letter case and ignores surrounding
  whitespace: `"info"`, `" WARN "`, and `"DeBuG"` are valid.
- Empty text, unknown names, aliases such as `"warning"`, numeric strings, and
  names with extra words are rejected.
- Check `ok` before using a parsed level. On failure the returned numeric value
  is zero, which is also a valid level; it must not be treated as successful parsing.
- The parser returns data to its caller. The caller handles presentation and
  chooses what to do when parsing fails.

The program prints every supported level, the zero value, invalid numeric levels,
and successful and failed parses. Its output includes:

```text
Log level: 2 Info (valid: true)
Log level: -1 LogLevel(-1) (valid: false)
Parsed " WARN ": Log level: 3 Warn (valid: true)
Cannot parse log level "2"
```

## Further practice

Add the input alias `"warning"` for `LevelWarn` while keeping the canonical display
name `Warn`. Update the parsing tests to document the new rule.

After studying chapter 9, try a parser that returns `(LogLevel, error)` with
details about invalid input. The current version uses the `(value, ok)` pattern
introduced in chapter 5.

Chapter 13's [structured logging lesson](../../13-standard-library/05-structured-logging/README.md)
builds on this exercise with `log/slog`. Its standard severity numbers differ
from this project's `iota` values, so integration needs an explicit mapping.
