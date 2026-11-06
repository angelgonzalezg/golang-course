package main

import (
	"errors"
	"time"
)

var (
	ErrStopped           = errors.New("scheduled wait stopped")
	ErrNegativeTickCount = errors.New("tick count must not be negative")
)

// waitForDelay waits once, observing a separate stop request. The caller owns
// stop; nil disables cancellation. A positive delay is this helper's policy,
// although NewTimer itself also accepts zero and negative durations.
func waitForDelay(delay time.Duration, stop <-chan struct{}) error {
	if delay <= 0 {
		return ErrNonPositiveDuration
	}
	// Use NewTimer when the owner needs Stop or Reset, rather than only the
	// receive channel returned by time.After. Stop does not close timer.C.
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-stop:
		return ErrStopped
	case <-timer.C:
		return nil
	}
}

// collectTicks receives a finite number of periodic notifications. It owns the
// ticker and stops it on every return path; it does not start any goroutines.
// A slow receiver can miss ticks, so count means received notifications, not
// every elapsed period. Results retain O(count) samples; keep the batch bounded.
func collectTicks(count int, interval time.Duration, stop <-chan struct{}) ([]time.Time, error) {
	if count < 0 {
		return nil, ErrNegativeTickCount
	}
	if interval <= 0 {
		return nil, ErrNonPositiveDuration
	}
	if count == 0 {
		return nil, nil
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var ticks []time.Time
	for len(ticks) < count {
		// Stop does not close ticker.C, so do not range it expecting shutdown.
		// If stop and a tick are both ready, select gives neither priority.
		select {
		case <-stop:
			// Already received samples remain useful on cancellation.
			return ticks, ErrStopped
		case tick := <-ticker.C:
			ticks = append(ticks, tick)
		}
	}
	return ticks, nil
}
