package main

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Watchdogs report a broken protocol; they do not determine the expected outcome.
// Tests choose ready cases with channel signals, rather than sleeping for workers.
func receiveWithin[T any](t *testing.T, input <-chan T) T {
	t.Helper()
	watchdog := time.NewTimer(5 * time.Second)
	defer watchdog.Stop()
	select {
	case value, ok := <-input:
		if !ok {
			t.Fatal("channel closed before the expected value")
		}
		return value
	case <-watchdog.C:
		t.Fatal("timed out waiting for a protocol event")
	}
	var zero T
	return zero
}

func waitForClose(t *testing.T, input <-chan struct{}) {
	t.Helper()
	watchdog := time.NewTimer(5 * time.Second)
	defer watchdog.Stop()
	select {
	case _, ok := <-input:
		if ok {
			t.Fatal("expected channel closure, not a value")
		}
	case <-watchdog.C:
		t.Fatal("worker failed to acknowledge completion")
	}
}

func TestRunWorkersJoinsAllCallbacks(t *testing.T) {
	for _, count := range []int{0, 1, 8} {
		values := make([]int, count)
		var calls atomic.Int64
		runWorkers(count, func(id int) {
			values[id] = id + 1
			calls.Add(1)
		})
		if got := calls.Load(); got != int64(count) {
			t.Fatalf("returned after %d callbacks, want %d", got, count)
		}
		for index, value := range values {
			if value != index+1 {
				t.Errorf("worker %d result = %d", index, value)
			}
		}
	}
}

func TestFinitePipelineClosesAfterAllValues(t *testing.T) {
	for _, count := range []int{0, 1, 4} {
		inputs := make(chan int)
		output := make(chan int)
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			var stages sync.WaitGroup
			stages.Add(2)
			go func() { defer stages.Done(); produceNumbers(inputs, count) }()
			go func() { defer stages.Done(); squareNumbers(inputs, output) }()
			stages.Wait()
		}()
		for number := 1; number <= count; number++ {
			if got := receiveWithin(t, output); got != number*number {
				t.Errorf("square %d = %d", number, got)
			}
		}
		waitForClose(t, finished)
		if value, ok := <-output; ok || value != 0 {
			t.Fatalf("drained output = (%d, %t), want (0, false)", value, ok)
		}
	}
}

func TestCollectMessagesDisablesClosedInputs(t *testing.T) {
	first := make(chan string, 2)
	first <- ""
	first <- "first"
	close(first)
	second := make(chan string, 1)
	second <- "second"
	close(second)
	finished := make(chan []string, 1)
	go func() { finished <- collectMessages(first, second) }()
	got := receiveWithin(t, finished)
	slices.Sort(got) // A ready select case is not a priority or an arrival-order contract.
	if !slices.Equal(got, []string{"", "first", "second"}) {
		t.Fatalf("messages = %q; empty messages are data, closure is not", got)
	}
	if got := collectMessages(nil, nil); len(got) != 0 {
		t.Errorf("nil inputs produced %q", got)
	}
}

func TestReceiveResult(t *testing.T) {
	ready := make(chan string, 1)
	ready <- ""
	if got, err := receiveResult(ready, nil); got != "" || err != nil {
		t.Fatalf("valid empty result = (%q, %v)", got, err)
	}
	closed := make(chan string)
	close(closed)
	if got, err := receiveResult(closed, nil); got != "" || !errors.Is(err, ErrResultsClosed) {
		t.Fatalf("closed results = (%q, %v)", got, err)
	}
	expired := make(chan time.Time, 1)
	expired <- time.Time{}
	if got, err := receiveResult(nil, expired); got != "" || !errors.Is(err, ErrTimeout) {
		t.Fatalf("expiration = (%q, %v)", got, err)
	}
}

func startTestSender(stop, release <-chan struct{}) (<-chan string, <-chan struct{}) {
	output := make(chan string)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		sendWhenReleased(stop, release, output)
	}()
	return output, finished
}

func TestSenderCompletesAndClosesOutput(t *testing.T) {
	release := make(chan struct{})
	close(release)
	output, finished := startTestSender(nil, release)
	if got := receiveWithin(t, output); got != "task completed" {
		t.Errorf("result = %q", got)
	}
	waitForClose(t, finished)
	if _, ok := <-output; ok {
		t.Fatal("sender did not close output after its value")
	}
}

func TestSenderCancellationBeforeRelease(t *testing.T) {
	stop := make(chan struct{})
	release := make(chan struct{})
	output, finished := startTestSender(stop, release)
	close(stop)
	waitForClose(t, finished)
	if _, ok := <-output; ok {
		t.Fatal("canceled sender produced a value")
	}
}

func TestSenderCancellationWhileSending(t *testing.T) {
	stop := make(chan struct{})
	release := make(chan struct{})
	output, finished := startTestSender(stop, release)
	watchdog := time.NewTimer(5 * time.Second)
	defer watchdog.Stop()
	select {
	case release <- struct{}{}:
		// The unbuffered handshake proves the first select has received release.
	case <-watchdog.C:
		close(stop)
		t.Fatal("worker never reached its release gate")
	}
	// No receiver is consuming output, so its send cannot complete without cancellation.
	close(stop)
	waitForClose(t, finished)
	if _, ok := <-output; ok {
		t.Fatal("canceled send unexpectedly completed")
	}
}

func TestContextWork(t *testing.T) {
	released := make(chan struct{})
	close(released)
	if err := workWithContext(context.Background(), released); err != nil {
		t.Fatalf("released work returned %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- workWithContext(ctx, make(chan struct{})) }()
	cancel()
	if err := receiveWithin(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled work returned %v", err)
	}
}

func TestCounterConcurrentAccess(t *testing.T) {
	var counter Counter
	const workers, times = 8, 1000
	runWorkers(workers, func(_ int) {
		for i := 0; i < times; i++ {
			counter.Increment()
			if value := counter.Value(); value < 0 || value > workers*times {
				t.Errorf("invalid concurrent snapshot: %d", value)
			}
		}
	})
	if got := counter.Value(); got != workers*times {
		t.Fatalf("counter = %d, want %d", got, workers*times)
	}
}

func TestIncrementHelpers(t *testing.T) {
	var locked Counter
	var atomicCounter atomic.Int64
	runWorkers(4, func(_ int) {
		incrementWithMutex(&locked, 500)
		incrementAtomic(&atomicCounter, 500)
	})
	if locked.Value() != 2000 || atomicCounter.Load() != 2000 {
		t.Fatalf("counters = (%d, %d), want (2000, 2000)", locked.Value(), atomicCounter.Load())
	}
}

func TestConcurrentConfigInitialization(t *testing.T) {
	var loader ConfigLoader
	var calls atomic.Int64
	values := make([]string, 12)
	runWorkers(len(values), func(id int) {
		values[id] = loader.Load(func() string {
			calls.Add(1)
			return "config loaded"
		})
	})
	if got := calls.Load(); got != 1 {
		t.Errorf("initializer ran %d times, want once", got)
	}
	for _, value := range values {
		if value != "config loaded" {
			t.Fatalf("unpublished configuration value %q", value)
		}
	}
	if got := loader.Load(func() string { return "replacement" }); got != "config loaded" {
		t.Errorf("Once unexpectedly replaced its value with %q", got)
	}
}

func TestWorkerPoolProcessesEveryJob(t *testing.T) {
	jobs := []Job{{ID: 10, Value: -2}, {ID: 20, Value: 0}, {ID: 30, Value: 5}, {ID: 40, Value: 10}}
	original := slices.Clone(jobs)
	for _, count := range []int{1, 3, 8} {
		type outcome struct {
			results []JobResult
			err     error
		}
		finished := make(chan outcome, 1)
		go func() {
			results, err := runWorkerPool(jobs, count)
			finished <- outcome{results, err}
		}()
		got := receiveWithin(t, finished)
		if got.err != nil || len(got.results) != len(jobs) {
			t.Fatalf("worker count %d returned (%v, %v)", count, got.results, got.err)
		}
		seen := make(map[int]bool)
		expected := map[int]int{10: -4, 20: 0, 30: 10, 40: 20}
		for _, result := range got.results {
			want, ok := expected[result.JobID]
			if !ok || seen[result.JobID] || result.Output != want || result.WorkerID < 1 || result.WorkerID > count {
				t.Fatalf("invalid or duplicate result: %+v", result)
			}
			seen[result.JobID] = true
		}
	}
	if !slices.Equal(jobs, original) {
		t.Fatal("worker pool changed its caller's input")
	}
}

func TestWorkerPoolEmptyAndInvalidCounts(t *testing.T) {
	results, err := runWorkerPool(nil, 3)
	if err != nil || len(results) != 0 {
		t.Fatalf("empty batch = (%v, %v)", results, err)
	}
	for _, count := range []int{0, -1} {
		results, err := runWorkerPool([]Job{{ID: 1, Value: 1}}, count)
		if results != nil || !errors.Is(err, ErrInvalidWorkerCount) {
			t.Errorf("worker count %d = (%v, %v)", count, results, err)
		}
	}
}
