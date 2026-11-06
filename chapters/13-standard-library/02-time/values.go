package main

import (
	"errors"
	"fmt"
	"time"
)

var ErrNonPositiveDuration = errors.New("duration must be positive")

// parsePositiveDuration reads a configured delay or polling interval.
// ParseDuration itself accepts zero and negative durations; rejecting them is
// this application's policy, which also avoids NewTicker's nonpositive panic.
func parsePositiveDuration(text string) (time.Duration, error) {
	duration, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("parse interval %q: %w", text, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("parse interval %q: %w", text, ErrNonPositiveDuration)
	}
	return duration, nil
}

// parseEventTime expects a timestamp with Z or a numeric UTC offset, using the
// RFC3339 layout. Store this example's event instants in UTC; convert to a user's
// location when displaying them. A timestamp's offset does not identify a region.
func parseEventTime(text string) (time.Time, error) {
	instant, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse event time %q: %w", text, err)
	}
	return instant.UTC(), nil
}

// remainingLifetime reports zero at and after expiry instead of a negative TTL.
// Passing now explicitly makes the expiry boundary testable without changing the
// process clock. Callers measuring a live deadline can also use time.Until.
func remainingLifetime(expiresAt, now time.Time) time.Duration {
	remaining := expiresAt.Sub(now)
	if remaining < 0 {
		return 0
	}
	return remaining
}
