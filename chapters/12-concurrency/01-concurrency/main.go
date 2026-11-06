// Concurrency Fundamentals
// Topic: coordinate goroutines with explicit communication, ownership, and completion.
// Usage: go run -race ./chapters/12-concurrency/01-concurrency
// Key idea: cancellation requests a stop; joining confirms that work has returned.
package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

func showGoroutine() {
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		fmt.Println("goroutine message")
	}()
	fmt.Println("main keeps running")
	// A completion signal coordinates the work; sleeping would only guess at timing.
	<-finished
}

func showWaitGroup() {
	results := make([]int, 3)
	runWorkers(len(results), func(id int) {
		// Each worker owns one distinct element; readers wait for all writes.
		results[id] = (id + 1) * 10
	})
	fmt.Println("joined worker results:", results)
}

func showUnbufferedChannel() {
	messages := make(chan string)
	var sender sync.WaitGroup
	sender.Add(1)
	go func() {
		defer sender.Done()
		messages <- "hello from a goroutine"
	}()
	fmt.Println("received:", <-messages)
	sender.Wait()
	// Exactly one value is expected, so the channel needs no close for this protocol.
}

func showBufferedChannel() {
	messages := make(chan string, 2)
	messages <- "first"
	messages <- "second"
	select {
	case messages <- "third":
		fmt.Println("queued third")
	default:
		// Nonblocking rejection is intentional here. A blocking send would wait.
		fmt.Println("buffer full: third was not queued")
	}
	fmt.Println(<-messages)
	fmt.Println(<-messages)
}

func showChannelClose() {
	numbers := make(chan int)
	var producer sync.WaitGroup
	producer.Add(1)
	go func() {
		defer producer.Done()
		produceNumbers(numbers, 3)
	}()
	for number := range numbers {
		fmt.Println("number:", number)
	}
	producer.Wait()
	value, ok := <-numbers
	fmt.Printf("after drain: value=%d, ok=%t\n", value, ok)

	// A queued zero is still a real value. Close preserves already queued values.
	queued := make(chan int, 1)
	queued <- 0
	close(queued)
	value, ok = <-queued
	fmt.Printf("queued zero: value=%d, ok=%t\n", value, ok)
	value, ok = <-queued
	fmt.Printf("after queued zero: value=%d, ok=%t\n", value, ok)
}

func showPipeline() {
	inputs := make(chan int)
	squares := make(chan int)
	var stages sync.WaitGroup
	stages.Add(2)
	go func() {
		defer stages.Done()
		produceNumbers(inputs, 4)
	}()
	go func() {
		defer stages.Done()
		squareNumbers(inputs, squares)
	}()
	// This finite example consumes every result. Early exit needs a cancellation
	// protocol through every stage, developed in the later patterns lessons.
	for square := range squares {
		fmt.Println("square:", square)
	}
	stages.Wait()
}

func showSelect() {
	first := make(chan string, 1)
	second := make(chan string, 1)
	first <- "first source"
	second <- "second source"
	close(first)
	close(second)
	// Both sources are ready. select does not prioritize the first written case.
	fmt.Println("selected messages (arrival order may vary):", collectMessages(first, second))
}

func showTimeout() {
	stop := make(chan struct{})
	release := make(chan struct{})
	results := make(chan string)
	var worker sync.WaitGroup
	worker.Add(1)
	go func() {
		defer worker.Done()
		sendWhenReleased(stop, release, results)
	}()

	// The work gate stays unreleased, so only expiration can produce a result here.
	timer := time.NewTimer(10 * time.Millisecond)
	_, err := receiveResult(results, timer.C)
	timer.Stop()
	fmt.Println("timed out:", errors.Is(err, ErrTimeout))

	// A timeout stopped this receive, not the worker. Request cancellation and join.
	close(stop)
	worker.Wait()
	_, ok := <-results
	fmt.Println("worker returned and closed results:", !ok)
}

func showStopBroadcast() {
	stop := make(chan struct{})
	var workers sync.WaitGroup
	for id := 0; id < 2; id++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-stop
		}()
	}
	// The caller is the sole closer. All current and future receivers observe it.
	close(stop)
	workers.Wait()
	fmt.Println("both workers acknowledged stop")
}

func showContext() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := make(chan struct{})
	result := make(chan error, 1)
	var worker sync.WaitGroup
	worker.Add(1)
	go func() {
		defer worker.Done()
		result <- workWithContext(ctx, release)
	}()
	cancel()
	fmt.Println("context task canceled:", errors.Is(<-result, context.Canceled))
	worker.Wait()
}

func showMutex() {
	var counter Counter
	runWorkers(3, func(_ int) {
		incrementWithMutex(&counter, 1000)
	})
	fmt.Println("counter with mutex:", counter.Value())
}

func showAtomic() {
	var counter atomic.Int64
	runWorkers(3, func(_ int) {
		incrementAtomic(&counter, 1000)
	})
	fmt.Println("atomic counter:", counter.Load())
}

func showWorkerPool() {
	jobs := []Job{{ID: 1, Value: 10}, {ID: 2, Value: 20}, {ID: 3, Value: 30}, {ID: 4, Value: 40}, {ID: 5, Value: 50}}
	results, err := runWorkerPool(jobs, 3)
	if err != nil {
		fmt.Println("worker pool:", err)
		return
	}
	// Sort only for presentation. Completion order and worker assignment may vary.
	slices.SortFunc(results, func(a, b JobResult) int { return cmp.Compare(a.JobID, b.JobID) })
	for _, result := range results {
		fmt.Printf("worker %d processed job %d -> %d\n", result.WorkerID, result.JobID, result.Output)
	}
}

func showOnce() {
	var loader ConfigLoader
	var calls atomic.Int64
	values := make([]string, 3)
	runWorkers(len(values), func(id int) {
		values[id] = loader.Load(func() string {
			calls.Add(1)
			return "config loaded"
		})
	})
	fmt.Println("config values:", values)
	fmt.Println("initializer calls:", calls.Load())
}

func main() {
	fmt.Println("== 1. Goroutine and completion signal ==")
	showGoroutine()
	fmt.Println("\n== 2. WaitGroup joins finite work ==")
	showWaitGroup()
	fmt.Println("\n== 3. Unbuffered rendezvous ==")
	showUnbufferedChannel()
	fmt.Println("\n== 4. Buffered queue and nonblocking rejection ==")
	showBufferedChannel()
	fmt.Println("\n== 5. Close, range, and comma-ok ==")
	showChannelClose()
	fmt.Println("\n== 6. Finite pipeline ==")
	showPipeline()
	fmt.Println("\n== 7. Select, closed inputs, and nil channels ==")
	showSelect()
	fmt.Println("\n== 8. Timeout, cancellation, and joining ==")
	showTimeout()
	fmt.Println("\n== 9. Broadcast cancellation ==")
	showStopBroadcast()
	fmt.Println("\n== 10. Context cancellation preview ==")
	showContext()
	fmt.Println("\n== 11. Mutex-protected state ==")
	showMutex()
	fmt.Println("\n== 12. Atomic counter ==")
	showAtomic()
	fmt.Println("\n== 13. Finite worker pool ==")
	showWorkerPool()
	fmt.Println("\n== 14. Concurrent initialization with Once ==")
	showOnce()
	fmt.Println("\n== 15. Choose the coordination tool ==")
	fmt.Println("WaitGroup joins; channels communicate; Mutex protects shared invariants.")
	fmt.Println("Atomics update independent values; Once initializes once; cancellation needs acknowledgment.")
}
