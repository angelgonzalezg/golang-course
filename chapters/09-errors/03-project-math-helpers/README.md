# Math helpers with errors

Practice variadic functions, integer arithmetic, custom error types, and explicit
error handling. Read the [error-handling lesson](../01-error-handling/main.go)
first; the [panic and recover lesson](../02-panic-and-recover/main.go) explains
why expected input failures should be returned as errors.

## Run and test

Run these commands from the repository root:

```sh
go run ./chapters/09-errors/03-project-math-helpers
go test ./chapters/09-errors/...
```

## Arithmetic contracts

| Function | Success | Failure |
| --- | --- | --- |
| `Sum(numbers ...int) (int, error)` | Adds values from left to right; no arguments return `(0, nil)`. | Returns `(0, error)` when an intermediate addition would overflow. |
| `SafeDivision(a, b int) (int, error)` | Returns an integer quotient, truncated toward zero. | Returns `(0, error)` for a zero divisor or an unrepresentable quotient. |

Check `err` before using a result. A zero result can be valid: `Sum(5, -5)` and
`SafeDivision(0, 5)` both return `(0, nil)`. These helpers return no partial result
on failure. Other APIs may have different contracts, such as returning bytes
read alongside an error.

`int` has a platform-dependent size. The helpers use `math.MinInt` and
`math.MaxInt` rather than assuming 64 bits. Addition is checked before it takes
place: a positive next value requires `total <= math.MaxInt - number`, while a
negative next value requires `total >= math.MinInt - number`.
`Sum(math.MaxInt, 1, -1)` fails at the second operand even though the final
mathematical total would fit: later cancellation cannot undo an earlier error.

Division discards the fractional part toward zero: `7 / 2` is `3`, and `-7 / 2`
is `-3`. The mathematical quotient for `SafeDivision(math.MinInt, -1)` is one
greater than `math.MaxInt`. For `int` variables, Go's `/` operator returns
`math.MinInt` in this special overflow case; this helper deliberately reports
`ErrOverflow` instead.

The arithmetic functions do not print or panic for these input failures.
`main` owns output and uses `defer` to print its completion message, including
when the last error example returns early.

## Inspecting errors

Each failure is a `*MathError` containing the operation, the two operands, and
the underlying cause. For a failed sum, the operands are the running total and
the next number, rather than the entire input slice.

- `errors.Is(err, ErrDivisionByZero)` identifies a zero divisor.
- `errors.Is(err, ErrOverflow)` identifies an integer result that cannot fit.
- `errors.As(err, &mathErr)` extracts a `*MathError` and its structured fields.
- `fmt.Errorf("calculate ratio: %w", err)` adds context while preserving both
  forms of inspection. `MathError.Unwrap` exposes its cause to the same tools.

Use the error values and fields for decisions; the formatted message is for
people. The final example prints:

```text
calculate ratio: division(10, 0): division by zero
division by zero: true
operation=division, a=10, b=0
```

The neighboring lessons also demonstrate wrapped `strconv` errors, configured
ports in the range 1–65535, joined validation failures, returning a literal
`nil` on success, and preserving both processing and cleanup failures with
`errors.Join`. A typed nil pointer stored in an `error` interface is non-nil.

Recovery examples intentionally panic for teaching. `recover` must be called
directly by a deferred function in the goroutine that panics; it does not resume
execution at the panic site or repair shared state. Use ordinary returned errors
for arithmetic input validation, as this project does.

## Further practice

1. Add checked multiplication with the same sentinel and custom error contracts.
2. Add an operand index to sum errors so a caller can locate the failing input.
3. Create a command-line calculator that parses arguments, adds error context,
   and chooses an exit status at the application boundary.

For the language rules and error inspection APIs, see the official
[integer operators specification](https://go.dev/ref/spec#Integer_operators),
[panic and recovery specification](https://go.dev/ref/spec#Handling_panics),
and [errors package documentation](https://pkg.go.dev/errors).
