// Concurrency patterns
// Topic: compose fan-out/fan-in with bounded queues, cancellation, and joining.
// Usage: go run ./chapters/12-concurrency/02-concurrency-patterns
package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"
)

func showFanOutAndFanIn() error {
	fmt.Println("== 1. Distribute work, then merge the worker streams ==")
	jobs := []Job{{ID: 10, Value: 2}, {ID: 20, Value: -3}, {ID: 10, Value: 4}, {ID: 40, Value: 0}}
	var results []Result
	err := runPipeline(context.Background(), jobs, pipelineOptions{Workers: 3, QueueCapacity: 1}, checkedSquare,
		func(_ context.Context, result Result) error {
			// One consumer owns this slice. Workers do not mutate it.
			results = append(results, result)
			return nil
		})
	if err != nil {
		return err
	}
	// Sorting is a presentation choice, not a guarantee of the parallel stage.
	// Input positions also distinguish jobs with the same application ID.
	slices.SortFunc(results, func(first, second Result) int { return cmp.Compare(first.Index, second.Index) })
	for _, result := range results {
		fmt.Printf("input=%d job=%d square=%d\n", result.Index, result.JobID, result.Output)
	}
	fmt.Println("Worker assignment and arrival order may vary.")
	return nil
}

func showBackpressure() {
	fmt.Println("\n== 2. A bounded fan-in queue propagates backpressure ==")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input := make(chan Result)
	var stages sync.WaitGroup
	merged := fanIn(ctx, &stages, 2, input)
	for value := 1; value <= 3; value++ {
		// The third unbuffered handshake proves the forwarder has received that
		// value. Its output queue already holds two values with no consumer yet.
		input <- Result{Output: value}
	}
	close(input)
	// This goroutine is the only receiver, so the queue cannot drain yet. This
	// observation is not a concurrent len-based check for whether a send is safe.
	fmt.Println("queued before the consumer starts:", len(merged))
	fmt.Println("The third delivery waits for space; a fourth input send would wait too.")
	for result := range merged {
		fmt.Println("received:", result.Output)
	}
	stages.Wait()
}

func showEarlyExitAndErrors() error {
	fmt.Println("\n== 3. A consumer can stop without draining every result ==")
	stopAfterFirst := errors.New("consumer has enough results")
	jobs := []Job{{ID: 1, Value: 1}, {ID: 2, Value: 2}, {ID: 3, Value: 3}, {ID: 4, Value: 4}}
	consumed := 0
	err := runPipeline(context.Background(), jobs, pipelineOptions{Workers: 2}, checkedSquare,
		func(context.Context, Result) error {
			consumed++
			return stopAfterFirst // An explicit control signal in this callback contract.
		})
	if !errors.Is(err, stopAfterFirst) || consumed != 1 {
		return fmt.Errorf("expected one result and consumer stop, got count=%d err=%v", consumed, err)
	}
	fmt.Println("results consumed:", consumed)
	fmt.Println("upstream work canceled and joined:", errors.Is(err, stopAfterFirst))

	fmt.Println("\n== 4. A worker failure cancels the shared operation ==")
	jobs = []Job{{ID: 1, Value: 1}, {ID: 2, Value: 1001}, {ID: 3, Value: 3}}
	err = runPipeline(context.Background(), jobs, pipelineOptions{Workers: 2, QueueCapacity: 1}, checkedSquare,
		func(context.Context, Result) error { return nil })
	if !errors.Is(err, ErrValueOutOfRange) {
		return fmt.Errorf("expected the original worker cause, got %v", err)
	}
	fmt.Println("worker cause preserved:", errors.Is(err, ErrValueOutOfRange))
	fmt.Println("Already consumed results are not rolled back; this is not a transaction.")
	return nil
}

func showDeadline() error {
	fmt.Println("\n== 5. A deadline stops cooperative processing and joins it ==")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	process := func(ctx context.Context, _ Job) (int, error) {
		<-ctx.Done() // Simulated dependency that observes the operation's context.
		return 0, ctx.Err()
	}
	err := runPipeline(ctx, []Job{{ID: 1}}, pipelineOptions{Workers: 2}, process,
		func(context.Context, Result) error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("expected caller deadline, got %v", err)
	}
	fmt.Println("deadline recognized after joining:", errors.Is(err, context.DeadlineExceeded))
	return nil
}

func run() error {
	if err := showFanOutAndFanIn(); err != nil {
		return err
	}
	showBackpressure()
	if err := showEarlyExitAndErrors(); err != nil {
		return err
	}
	if err := showDeadline(); err != nil {
		return err
	}
	fmt.Println("\n== 6. Pattern rules ==")
	fmt.Println("Bound active workers and queue capacities separately; choose ordering explicitly.")
	fmt.Println("Make blocking stages observe cancellation, assign stream closure, and join every stage.")
	fmt.Println("Callbacks must cooperate; cancellation cannot undo side effects or preempt arbitrary code.")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "concurrency patterns lesson:", err)
		os.Exit(1)
	}
}
