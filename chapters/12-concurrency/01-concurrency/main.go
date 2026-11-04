// Concurrency
// Topic: concurrency lets a Go program make progress on more than one task at a time.
// Args/Input: examples use goroutines, channels, select, context, mutexes, atomics, and worker pools.
// Usage: use goroutines for independent work, channels for communication, and sync tools for shared state.
// Key idea: do not share memory by accident; either communicate with channels or protect shared data.
package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

/* Helper section 1: Goroutine helpers */
func printAfterDelay(message string, delay time.Duration) {
	time.Sleep(delay)
	fmt.Println(message)
}

func waitGroupWorker(id int, wg *sync.WaitGroup) {
	defer wg.Done()

	time.Sleep(time.Duration(id) * 20 * time.Millisecond)
	fmt.Println("WaitGroup worker finished:", id)
}

/* Helper section 2: Channel helpers */
func sendGreeting(messages chan<- string) {
	// chan<- means this function can only send to the channel.
	messages <- "hello from a goroutine"
}

func produceNumbers(out chan<- int, count int) {
	defer close(out) // Closing tells receivers there are no more values.

	for number := 1; number <= count; number++ {
		out <- number
	}
}

func squareNumbers(in <-chan int, out chan<- int) {
	defer close(out)

	for number := range in {
		out <- number * number
	}
}

/* Helper section 3: Select, timeout, and cancellation helpers */
func delayedResult(label string, delay time.Duration) <-chan string {
	results := make(chan string, 1)

	go func() {
		time.Sleep(delay)
		results <- label + " completed"
	}()

	return results
}

func heartbeat(done <-chan struct{}) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			fmt.Println("heartbeat stopped")
			return
		case <-ticker.C:
			fmt.Println("heartbeat")
		}
	}
}

func workWithContext(ctx context.Context, name string) error {
	select {
	case <-time.After(100 * time.Millisecond):
		fmt.Println(name, "finished")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

/* Helper section 4: Shared state helpers */
func incrementWithMutex(counter *int, times int, mu *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()

	for i := 0; i < times; i++ {
		mu.Lock()
		*counter = *counter + 1
		mu.Unlock()
	}
}

func incrementAtomic(counter *int64, times int, wg *sync.WaitGroup) {
	defer wg.Done()

	for i := 0; i < times; i++ {
		atomic.AddInt64(counter, 1)
	}
}

/* Helper section 5: Worker pool helpers */
type Job struct {
	ID    int
	Value int
}

type JobResult struct {
	WorkerID int
	JobID    int
	Output   int
}

func poolWorker(id int, jobs <-chan Job, results chan<- JobResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for job := range jobs {
		time.Sleep(15 * time.Millisecond)
		results <- JobResult{
			WorkerID: id,
			JobID:    job.ID,
			Output:   job.Value * 2,
		}
	}
}

/* Helper section 6: sync.Once helper */
func loadConfigOnce(once *sync.Once, config *string) {
	once.Do(func() {
		fmt.Println("loading config once")
		*config = "config loaded"
	})
}

func main() {
	/* Section 1: Goroutine basics */
	fmt.Println("== 1. Goroutine basics ==")

	// go starts a function in a new goroutine.
	go printAfterDelay("goroutine message", 30*time.Millisecond)
	fmt.Println("main keeps running")

	// This small sleep is only for the lesson, so the goroutine can print.
	time.Sleep(50 * time.Millisecond)

	/* Section 2: WaitGroup waits for goroutines */
	fmt.Println("\n== 2. WaitGroup waits for goroutines ==")

	var wg sync.WaitGroup
	for id := 1; id <= 3; id++ {
		wg.Add(1)
		go waitGroupWorker(id, &wg)
	}
	wg.Wait()
	fmt.Println("all WaitGroup workers finished")

	/* Section 3: Unbuffered channel */
	fmt.Println("\n== 3. Unbuffered channel ==")

	messages := make(chan string)
	go sendGreeting(messages)

	// Receive blocks until another goroutine sends a value.
	greeting := <-messages
	fmt.Println("received:", greeting)

	/* Section 4: Buffered channel */
	fmt.Println("\n== 4. Buffered channel ==")

	bufferedMessages := make(chan string, 2)
	bufferedMessages <- "first"
	bufferedMessages <- "second"

	fmt.Println(<-bufferedMessages)
	fmt.Println(<-bufferedMessages)

	/* Section 5: Close and range over a channel */
	fmt.Println("\n== 5. Close and range over a channel ==")

	numbers := make(chan int)
	go produceNumbers(numbers, 3)

	for number := range numbers {
		fmt.Println("number:", number)
	}

	/* Section 6: Channel pipeline */
	fmt.Println("\n== 6. Channel pipeline ==")

	inputs := make(chan int)
	squares := make(chan int)

	go produceNumbers(inputs, 4)
	go squareNumbers(inputs, squares)

	for square := range squares {
		fmt.Println("square:", square)
	}

	/* Section 7: Select chooses the ready channel */
	fmt.Println("\n== 7. Select chooses the ready channel ==")

	fast := delayedResult("fast task", 20*time.Millisecond)
	slow := delayedResult("slow task", 60*time.Millisecond)

	for i := 0; i < 2; i++ {
		select {
		case result := <-fast:
			fmt.Println(result)
		case result := <-slow:
			fmt.Println(result)
		}
	}

	/* Section 8: Timeout with select */
	fmt.Println("\n== 8. Timeout with select ==")

	select {
	case result := <-delayedResult("very slow task", 80*time.Millisecond):
		fmt.Println(result)
	case <-time.After(30 * time.Millisecond):
		fmt.Println("timed out")
	}

	/* Section 9: Cancellation with a done channel */
	fmt.Println("\n== 9. Cancellation with a done channel ==")

	done := make(chan struct{})
	go heartbeat(done)

	time.Sleep(55 * time.Millisecond)
	close(done)
	time.Sleep(10 * time.Millisecond)

	/* Section 10: Cancellation with context */
	fmt.Println("\n== 10. Cancellation with context ==")

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	if err := workWithContext(ctx, "context task"); err != nil {
		fmt.Println("context task stopped:", err)
	}

	/* Section 11: Protect shared data with a Mutex */
	fmt.Println("\n== 11. Protect shared data with a Mutex ==")

	counter := 0
	var counterMu sync.Mutex
	var mutexWG sync.WaitGroup

	for i := 0; i < 3; i++ {
		mutexWG.Add(1)
		go incrementWithMutex(&counter, 1000, &counterMu, &mutexWG)
	}
	mutexWG.Wait()
	fmt.Println("counter with mutex:", counter)

	/* Section 12: Atomic counters */
	fmt.Println("\n== 12. Atomic counters ==")

	var atomicCounter int64
	var atomicWG sync.WaitGroup

	for i := 0; i < 3; i++ {
		atomicWG.Add(1)
		go incrementAtomic(&atomicCounter, 1000, &atomicWG)
	}
	atomicWG.Wait()
	fmt.Println("atomic counter:", atomicCounter)

	/* Section 13: Worker pool */
	fmt.Println("\n== 13. Worker pool ==")

	jobs := make(chan Job)
	results := make(chan JobResult)
	var poolWG sync.WaitGroup

	for workerID := 1; workerID <= 3; workerID++ {
		poolWG.Add(1)
		go poolWorker(workerID, jobs, results, &poolWG)
	}

	go func() {
		for jobID := 1; jobID <= 5; jobID++ {
			jobs <- Job{ID: jobID, Value: jobID * 10}
		}
		close(jobs)
	}()

	go func() {
		poolWG.Wait()
		close(results)
	}()

	for result := range results {
		fmt.Printf("worker %d processed job %d -> %d\n", result.WorkerID, result.JobID, result.Output)
	}

	/* Section 14: sync.Once */
	fmt.Println("\n== 14. sync.Once ==")

	var once sync.Once
	config := ""

	for i := 0; i < 3; i++ {
		loadConfigOnce(&once, &config)
	}
	fmt.Println(config)

	/* Section 15: Final concurrency rules */
	fmt.Println("\n== 15. Final concurrency rules ==")

	// Use goroutines for independent work.
	// Use WaitGroup when you only need to wait.
	// Use channels when goroutines must communicate values.
	// Use context when work needs cancellation or deadlines.
	// Use Mutex or atomic operations when goroutines share memory.
	// Run go test -race when you want Go to look for data races.
	fmt.Println("Concurrency is powerful when ownership and shutdown are clear.")
}
