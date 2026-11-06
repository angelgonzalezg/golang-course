package main

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestWaitForDelayFiresAtItsDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const delay = 5 * time.Second
		started := time.Now()
		finished := make(chan error, 1)
		go func() { finished <- waitForDelay(delay, nil) }()
		// Wait until the worker has created its timer and blocked on select.
		synctest.Wait()

		// Inside this bubble Sleep advances virtual time, not wall-clock time.
		time.Sleep(delay - time.Nanosecond)
		synctest.Wait()
		select {
		case err := <-finished:
			t.Fatalf("wait returned before its deadline: %v", err)
		default:
		}
		time.Sleep(time.Nanosecond)
		if err := <-finished; err != nil {
			t.Fatalf("wait failed: %v", err)
		}
		if elapsed := time.Since(started); elapsed != delay {
			t.Fatalf("virtual elapsed time = %v, want %v", elapsed, delay)
		}
	})
}

func TestWaitForDelayCanBeStoppedBeforeTheDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		stop := make(chan struct{})
		finished := make(chan error, 1)
		go func() { finished <- waitForDelay(time.Hour, stop) }()
		synctest.Wait()
		time.Sleep(3 * time.Second)
		close(stop)
		if err := <-finished; !errors.Is(err, ErrStopped) {
			t.Fatalf("want ErrStopped, got %v", err)
		}
		if elapsed := time.Since(started); elapsed != 3*time.Second {
			t.Fatalf("cancellation waited for the timer: virtual elapsed time %v", elapsed)
		}
	})
}

func TestWaitForDelayWithAnAlreadyClosedStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stop := make(chan struct{})
		close(stop)
		if err := waitForDelay(time.Hour, stop); !errors.Is(err, ErrStopped) {
			t.Fatalf("want ErrStopped, got %v", err)
		}
	})
}

func TestWaitForDelayRejectsNonPositiveDelays(t *testing.T) {
	for _, delay := range []time.Duration{0, -time.Second} {
		if err := waitForDelay(delay, nil); !errors.Is(err, ErrNonPositiveDuration) {
			t.Fatalf("delay %v: want ErrNonPositiveDuration, got %v", delay, err)
		}
	}
}

func TestCollectTicksCompletesItsFiniteBatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		ticks, err := collectTicks(3, time.Second, nil)
		if err != nil || len(ticks) != 3 {
			t.Fatalf("collectTicks = (%v, %v), want three notifications", ticks, err)
		}
		for index, tick := range ticks {
			// The virtual receiver is ready for each scheduled notification.
			want := started.Add(time.Duration(index+1) * time.Second)
			if !tick.Equal(want) {
				t.Fatalf("tick %d = %v, want %v", index, tick, want)
			}
		}
		if elapsed := time.Since(started); elapsed != 3*time.Second {
			t.Fatalf("virtual elapsed time = %v, want 3s", elapsed)
		}
	})
}

func TestCollectTicksCancellationKeepsReceivedSamples(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		type result struct {
			ticks []time.Time
			err   error
		}
		stop := make(chan struct{})
		finished := make(chan result, 1)
		go func() {
			ticks, err := collectTicks(10, time.Hour, stop)
			finished <- result{ticks, err}
		}()
		synctest.Wait()
		time.Sleep(2 * time.Hour)
		// Let the worker consume the second tick before stopping it. Wait is a
		// synchronization point; it does not advance time to the third tick.
		synctest.Wait()
		close(stop)
		got := <-finished
		if len(got.ticks) != 2 || !errors.Is(got.err, ErrStopped) {
			t.Fatalf("collectTicks = (%v, %v), want two samples and ErrStopped", got.ticks, got.err)
		}
	})
}

func TestCollectTicksCanStopBeforeAnyTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stop := make(chan struct{})
		close(stop)
		ticks, err := collectTicks(3, time.Hour, stop)
		if len(ticks) != 0 || !errors.Is(err, ErrStopped) {
			t.Fatalf("collectTicks = (%v, %v), want no samples and ErrStopped", ticks, err)
		}
	})
}

func TestCollectTicksValidatesBeforeStarting(t *testing.T) {
	tests := []struct {
		count    int
		interval time.Duration
		err      error
	}{
		{-1, time.Second, ErrNegativeTickCount},
		{1, 0, ErrNonPositiveDuration},
		{1, -time.Second, ErrNonPositiveDuration},
		{0, 0, ErrNonPositiveDuration},
		{0, time.Hour, nil},
	}
	for _, tt := range tests {
		ticks, err := collectTicks(tt.count, tt.interval, nil)
		if ticks != nil || !errors.Is(err, tt.err) {
			t.Fatalf("count %d interval %v: got (%v, %v), want nil and %v", tt.count, tt.interval, ticks, err, tt.err)
		}
	}
}
