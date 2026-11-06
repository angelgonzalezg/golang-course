# Time, calendars, and scheduling

Practice `time.Time`, durations, layouts, locations, expiry rules, timers, and
tickers. This chapter 13 lesson builds on errors and chapter 12's cancellation
and ownership examples.

## Run and validate

Run from the repository root:

```sh
go run ./chapters/13-standard-library/02-time
go test ./chapters/13-standard-library/02-time
go test -race -timeout=30s ./chapters/13-standard-library/02-time
go vet ./chapters/13-standard-library/02-time
```

To exercise the asynchronous tests repeatedly with different scheduler settings:

```sh
go test -race -count=5 -cpu=1,2 -timeout=30s ./chapters/13-standard-library/02-time
```

The runnable example uses short real timers. Their observed delay varies with
scheduling; no exact elapsed output is required. Calendar examples use fixed
historical dates and explicit locations, independent of the machine's local zone.
The main package imports `time/tzdata` as a fallback for hosts without zone data.
This increases the binary size and is a choice for the executable to make.

## Read the example

- [main.go](main.go) runs eleven sections, explaining choices beside their use.
- [values.go](values.go) parses positive configured intervals, normalizes event
  timestamps to UTC, and clamps an expired lifetime to zero.
- [scheduling.go](scheduling.go) owns a cancellable one-shot timer and a finite
  batch of ticker notifications. Neither helper starts a goroutine itself.
- [values_test.go](values_test.go) checks units, invalid configuration, UTC
  normalization, repeated local clock readings, and the exact expiry boundary.
- [scheduling_test.go](scheduling_test.go) uses a controlled clock to check
  expiration, cancellation, partial results, and finite completion.

## Choose the representation and operation

| Need | Choice | Reason |
| --- | --- | --- |
| Represent an instant | `time.Time`, usually passed as a value | Locations affect its display; `UTC` and `In` return new values. |
| Represent an elapsed interval | `time.Duration` | Its integer unit is nanoseconds; use explicit units such as `5 * time.Second`. |
| Measure in-process elapsed work | `started := time.Now()` and `time.Since(started)` | Preserves the monotonic reading when available. |
| Find a difference or time to a deadline | `later.Sub(earlier)` or `time.Until(deadline)` | Returns a duration, possibly negative. |
| Compare instants | `Equal`, `Before`, `After` | `==` also compares location and monotonic representation. |
| Add an elapsed interval | `instant.Add(duration)` | Suitable for TTLs and elapsed delays. |
| Advance a local calendar | `instant.AddDate(years, months, days)` | Uses the instant's location and calendar rules. |
| Exchange timestamps | `time.RFC3339` with `Z` or a numeric offset | Makes the intended instant explicit. |
| Interpret a regional clock reading | `LoadLocation` and `ParseInLocation` | Uses named-zone rules rather than an assumed fixed offset. |

`time.Time{}` is a valid zero value. This lesson treats it as unset and checks
`IsZero`. It is different from Unix epoch zero, which represents 1970-01-01 UTC.

## Durations and application policy

`time.Duration(5)` is five nanoseconds, while `5 * time.Second` is five seconds.
`Milliseconds` returns an integer and discards smaller fractions; `Seconds`
returns a floating-point quantity. `Round` and `Truncate` explicitly choose how
to reduce precision. With 1.5 seconds, rounding to a second produces 2 seconds,
while truncating produces 1 second.

`parsePositiveDuration` accepts configured values such as `250ms` and `1h30m`.
Go's parser supports signed durations, including zero; this helper deliberately
requires a positive result. A value smaller than one nanosecond can become zero
and fail that policy. Syntax, unknown units, and duration overflow remain parsing
errors. Numeric configuration also needs a range policy before unit multiplication
when its magnitude is not already bounded.

`1d` is not a duration unit. Spell a fixed elapsed day as `24 * time.Hour` when
that is the requirement, or use calendar operations when the requirement is
the next local date.

## Layouts, zones, and calendar boundaries

Go layouts arrange pieces of the reference date `2006-01-02 15:04:05`.
`02 Jan 2006 at 15:04 MST` is a display layout; `YYYY-MM-DD` is not Go's pattern
syntax. Prefer predefined layouts such as `time.RFC3339` and `time.DateOnly`
when they match the required format.

`parseEventTime` requires the RFC3339 layout and returns the instant in UTC.
Parse failures retain `*time.ParseError` through wrapping. UTC is this example's
storage convention; `In` changes its display location without changing the
instant. An offset such as `-06:00` does not identify a region or retain that
region's future rules. Unix seconds and Unix milliseconds are different units;
converting to either loses finer precision, and neither encodes a display zone.

With a layout that has no zone, `Parse` assumes UTC. `ParseInLocation` interprets
that clock reading in the supplied location. Use IANA names such as
`America/New_York` for regional schedules. `FixedZone` is appropriate for a
specified constant offset; it cannot model a region's rule changes.

The historical example starts at local noon on 2024-03-09 in New York:

| Operation | Local result | Elapsed difference |
| --- | --- | --- |
| `Add(24 * time.Hour)` | 2024-03-10 13:00, offset -04:00 | 24 hours |
| `AddDate(0, 0, 1)` | 2024-03-10 12:00, offset -04:00 | 23 hours |

Some local clock readings are skipped or repeated during zone transitions.
This example uses an unambiguous noon. An appointment system must define how it
handles ambiguous or nonexistent local times; `Date` and `ParseInLocation` do not
provide that application policy. Tests demonstrate how explicit offsets select
the two distinct instants displayed as 01:30 during the historical fall transition.

`AddDate` also normalizes invalid calendar dates. January 31 plus one month in
2024 becomes March 2, not the last day of February. Billing rules that require
month-end clamping need their own policy.

## Elapsed measurements and expiry

`Now` includes a process-local monotonic reading on supported systems. Keeping
that reading lets `Since`, `Sub`, and deadline arithmetic avoid wall-clock
adjustments when both relevant values retain it. Formatting, parsing, and Unix
conversion do not preserve it. `UTC`, `In`, and calendar arithmetic also strip
the monotonic reading. Use the original `Now` value for live elapsed measurements;
use wall-clock instants for stored or exchanged timestamps.

`remainingLifetime` receives both the deadline and a chosen `now`. This keeps
the boundary reproducible: before expiry it returns the remaining duration; at
expiry and afterward it returns zero. Passing explicit time is sufficient here;
there is no need to modify the system clock or sleep until a real expiry.

## Choose a waiting mechanism and own its lifetime

| Mechanism | When to use it | Ownership and behavior |
| --- | --- | --- |
| `time.Sleep` | A simple delay in the current goroutine is sufficient. | Blocks that goroutine and has no cancellation channel; it does not establish another goroutine's completion. |
| `time.After` | A one-shot `select` only needs an expiration channel. | Provides no handle for explicitly stopping or resetting that timer. |
| `time.NewTimer` | A one-shot wait needs `Stop`, `Reset`, or explicit ownership. | Stop the owned timer when abandoning the wait; do not expect `C` to close. |
| `time.NewTicker` | Work should receive repeated opportunities at an interval. | Stop it when finished; slow receivers can miss notifications. |
| `time.AfterFunc` | A delayed callback is useful. | The callback runs in a separate goroutine; its timer's `C` is nil, and `Stop` does not wait for a running callback. |

The timer examples use the modern channel semantics introduced in Go 1.23,
under this course's Go 1.27 requirement. A single owner resets and receives.
After `Stop` or `Reset` returns, receives do not see events from an old timer
configuration; older stop-and-drain recipes should not be copied into this code.
`Reset` reports whether the timer was active, not whether resetting succeeded.
`Stop` returning false after an event was received is normal. These channel-timer
rules differ from `AfterFunc` callback scheduling and completion.

Modern Go can also collect unreferenced timers and tickers. `Stop` here expresses
the end of scheduled activity, rather than a claim that every unused timer leaks.
`time.Tick` offers a channel without a stop handle; choose `NewTicker` when explicit
lifetime control is useful.

`waitForDelay` uses a separate stop channel to interrupt a timed wait. A nil
channel disables cancellation. If both stop and expiration are ready, `select`
does not prioritize either. Stopping a timer does not cancel user work or join
a goroutine; chapter 12's completion protocols still apply. Chapter 14 will
extend cancellation through contexts and deadlines.

`collectTicks` stops after a requested number of received notifications and
returns any samples already collected if cancellation wins. Both arguments are
validated before creating a ticker: count must be nonnegative and interval must
be positive, including for an empty batch. An empty valid batch returns immediately.
Retaining the batch uses memory proportional to its count; an ongoing service
would normally process each notification without keeping an unbounded history.
Ticker timestamps describe scheduled notifications, not an exact execution time
or a guarantee that every period was processed. `Ticker.Stop` does not close `C`,
so ranging that channel is not a shutdown protocol.

## Tests with a controlled clock

The scheduling tests use `testing/synctest`, available since Go 1.25. This is a
modern addition to the course beyond the book's original tooling. Each test
bubble has a fake clock, and virtual time advances when its goroutines are
durably blocked. `time.Sleep` inside those bubbles advances virtual time rather
than waiting for real seconds or hours.

`synctest.Wait` lets the test observe a stable blocked state before checking a
deadline or sending cancellation. It does not advance time to the next tick.
All test channels and worker goroutines are created inside the bubble, and the
workers finish before the test ends. There are no real-time scheduling guesses
or assertions about the exact duration of the runnable example.

References: [`time`](https://pkg.go.dev/time),
[`time/tzdata`](https://pkg.go.dev/time/tzdata),
[Go 1.23 timer changes](https://go.dev/wiki/Go123Timer),
and [`testing/synctest`](https://pkg.go.dev/testing/synctest).
