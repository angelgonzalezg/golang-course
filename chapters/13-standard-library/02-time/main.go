// Time
// Topic: distinguish instants, elapsed durations, calendars, and scheduled events.
// Usage: go run ./chapters/13-standard-library/02-time
package main

import (
	"errors"
	"fmt"
	"os"
	"time"
	// Bundle fallback timezone data so this standalone lesson also works on
	// hosts without a timezone database. This is a program-level choice.
	_ "time/tzdata"
)

func showDurations() error {
	fmt.Println("== 1. Durations carry units ==")
	// A Duration's integer unit is nanoseconds. Converting a number does not
	// attach seconds; multiply by an explicit unit for numeric configuration.
	fmt.Println("time.Duration(5):", time.Duration(5))
	fmt.Println("5 * time.Second:", 5*time.Second)
	interval, err := parsePositiveDuration("1.5s")
	if err != nil {
		return err
	}
	fmt.Println("parsed interval:", interval)
	fmt.Println("integer milliseconds:", interval.Milliseconds())
	fmt.Println("fractional seconds:", interval.Seconds())
	fmt.Println("rounded to a second:", interval.Round(time.Second))
	fmt.Println("truncated to a second:", interval.Truncate(time.Second))

	_, err = parsePositiveDuration("0s")
	if !errors.Is(err, ErrNonPositiveDuration) {
		return fmt.Errorf("expected an invalid interval, got %v", err)
	}
	fmt.Println("zero interval rejected by application:", errors.Is(err, ErrNonPositiveDuration))
	return nil
}

func showInstants() error {
	fmt.Println("\n== 2. Layouts use Go's reference date ==")
	instant, err := parseEventTime("2024-07-01T09:30:00-06:00")
	if err != nil {
		return err
	}
	// Use standard layouts for interchange. A custom layout uses the components
	// of 2006-01-02 15:04:05, rather than YYYY/MM/DD-style format directives.
	fmt.Println("event in UTC:", instant.Format(time.RFC3339))
	fmt.Println("custom display:", instant.Format("02 Jan 2006 at 15:04 MST"))
	// Choose the unit required by the external protocol. This example has no
	// fractional seconds; Unix seconds would discard a finer subsecond part.
	fmt.Println("Unix seconds:", instant.Unix())
	fmt.Println("Unix milliseconds:", instant.UnixMilli())
	fmt.Println("Unix round trip:", time.Unix(instant.Unix(), 0).Equal(instant))

	_, err = parseEventTime("2024-07-01 09:30")
	var parseErr *time.ParseError
	if !errors.As(err, &parseErr) {
		return fmt.Errorf("expected a timestamp parse error, got %v", err)
	}
	fmt.Println("timestamp without an offset rejected:", true)

	fmt.Println("\n== 3. Compare instants with Equal ==")
	// Time is normally stored and passed as a value. In returns a new value
	// without mutating instant or changing the event it represents.
	// A fixed offset is useful when the offset itself is the specification.
	// It does not model a region's daylight-saving or historical rule changes.
	display := instant.In(time.FixedZone("display +02", 2*60*60))
	fmt.Println("same event at a fixed offset:", display.Format(time.RFC3339))
	fmt.Println("Equal compares the instant:", instant.Equal(display))
	fmt.Println("== also compares representation:", instant == display)
	fmt.Println("zero Time is unset:", (time.Time{}).IsZero())
	return nil
}

func showCalendar() error {
	fmt.Println("\n== 4. Named locations interpret local clock readings ==")
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		return fmt.Errorf("load example location: %w", err)
	}
	const layout = "2006-01-02 15:04"
	const text = "2024-03-09 12:00"
	local, err := time.ParseInLocation(layout, text, location)
	if err != nil {
		return fmt.Errorf("parse local appointment: %w", err)
	}
	// Parse without a zone in its layout assumes UTC. ParseInLocation applies
	// the supplied region. This example chooses an unambiguous local time.
	assumedUTC, err := time.Parse(layout, text)
	if err != nil {
		return err
	}
	fmt.Println("Parse assumes UTC:", assumedUTC.Format(time.RFC3339))
	fmt.Println("ParseInLocation uses the region:", local.Format(time.RFC3339))

	fmt.Println("\n== 5. A calendar day need not be 24 elapsed hours ==")
	// Use Add for elapsed intervals. Use AddDate for local calendar arithmetic;
	// this historical example crosses the region's spring DST transition.
	elapsedDay := local.Add(24 * time.Hour)
	calendarDay := local.AddDate(0, 0, 1)
	fmt.Println("start:", local.Format(time.RFC3339))
	fmt.Println("Add(24h):", elapsedDay.Format(time.RFC3339))
	fmt.Println("AddDate(0, 0, 1):", calendarDay.Format(time.RFC3339))
	fmt.Println("elapsed time to next local noon:", calendarDay.Sub(local))
	// AddDate normalizes invalid dates; it does not clamp to month-end.
	monthEnd := time.Date(2024, time.January, 31, 12, 0, 0, 0, time.UTC)
	fmt.Println("January 31 plus one month:", monthEnd.AddDate(0, 1, 0).Format(time.DateOnly))
	return nil
}

func showExpiry() {
	fmt.Println("\n== 6. Make the expiry boundary explicit ==")
	issued := time.Date(2024, time.July, 1, 12, 0, 0, 0, time.UTC)
	expires := issued.Add(30 * time.Minute)
	// Passing a chosen now value makes this rule reproducible. The application
	// considers the lifetime exhausted exactly at expiry, as well as afterward.
	fmt.Println("remaining after 20 minutes:", remainingLifetime(expires, issued.Add(20*time.Minute)))
	fmt.Println("remaining at expiry:", remainingLifetime(expires, expires))
	fmt.Println("remaining after expiry:", remainingLifetime(expires, expires.Add(time.Minute)))
}

func showScheduling() error {
	fmt.Println("\n== 7. Measure elapsed time with Since ==")
	// Keep Now's monotonic clock reading for in-process elapsed measurements.
	// Formatting, parsing, and Unix timestamps cannot preserve that reading.
	started := time.Now()
	if err := waitForDelay(5*time.Millisecond, nil); err != nil {
		return err
	}
	fmt.Println("observed wait (varies with scheduling):", time.Since(started))
	fmt.Println("past deadline has no time left:", time.Until(started.Add(-time.Second)) <= 0)

	fmt.Println("\n== 8. Own a timer when stopping or resetting it ==")
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	// This course uses modern timer-channel semantics (Go 1.23+, default here).
	// A single owner resets and receives; no legacy stop-and-drain is needed.
	fmt.Println("Reset replaced an active wait:", timer.Reset(5*time.Millisecond))
	<-timer.C
	fmt.Println("Reset after receiving the event:", timer.Reset(5*time.Millisecond))
	<-timer.C
	fmt.Println("Stop after the event was received:", timer.Stop())
	other := time.NewTimer(time.Hour)
	fmt.Println("Stop an active timer:", other.Stop())
	fmt.Println("Stop it again:", other.Stop())
	// Do not receive from other.C after stopping it: Stop does not close it.

	fmt.Println("\n== 9. A stop request can interrupt a timed wait ==")
	stop := make(chan struct{})
	close(stop)
	// The long timer is not ready, so this demonstrates cancellation without
	// racing two short real-time events or leaving a worker in the background.
	err := waitForDelay(time.Hour, stop)
	if !errors.Is(err, ErrStopped) {
		return fmt.Errorf("expected a stopped wait, got %v", err)
	}
	fmt.Println("wait stopped:", errors.Is(err, ErrStopped))

	fmt.Println("\n== 10. A ticker signals periodic opportunities ==")
	ticks, err := collectTicks(3, 5*time.Millisecond, nil)
	if err != nil {
		return err
	}
	fmt.Println("notifications received:", len(ticks))
	fmt.Println("The owner stopped the ticker before returning.")
	return nil
}

func run() error {
	if err := showDurations(); err != nil {
		return err
	}
	if err := showInstants(); err != nil {
		return err
	}
	if err := showCalendar(); err != nil {
		return err
	}
	showExpiry()
	if err := showScheduling(); err != nil {
		return err
	}
	fmt.Println("\n== 11. Time rules ==")
	fmt.Println("Use durations for elapsed intervals; use calendar arithmetic for dates in a location.")
	fmt.Println("Exchange timestamps with explicit offsets, and compare instants with Equal.")
	fmt.Println("Own timer/ticker cleanup; stopping a timer does not stop or join a goroutine.")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "time lesson:", err)
		os.Exit(1)
	}
}
