package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrTimeout            = errors.New("result timed out")
	ErrResultsClosed      = errors.New("results closed without a value")
	ErrInvalidWorkerCount = errors.New("worker count must be positive")
)

// produceNumbers is the sole sender and closer of out. The caller must drain it.
func produceNumbers(out chan<- int, count int) {
	defer close(out)
	for index := 0; index < count; index++ {
		out <- index + 1
	}
}

// squareNumbers owns out, while the upstream producer owns closing in.
func squareNumbers(in <-chan int, out chan<- int) {
	defer close(out)
	for number := range in {
		out <- number * number
	}
}

// collectMessages drains two inputs until both close, in unspecified arrival order.
// Setting a local channel variable to nil disables its select case; it closes nothing.
func collectMessages(first, second <-chan string) []string {
	var messages []string
	for first != nil || second != nil {
		select {
		case message, ok := <-first:
			if !ok {
				first = nil
				continue
			}
			messages = append(messages, message)
		case message, ok := <-second:
			if !ok {
				second = nil
				continue
			}
			messages = append(messages, message)
		}
	}
	return messages
}

// receiveResult receives one value or an expiration event. The caller owns the
// timer and any producer's cancellation/joining. A nil input disables that case.
func receiveResult(results <-chan string, expired <-chan time.Time) (string, error) {
	select {
	case result, ok := <-results:
		if !ok {
			return "", ErrResultsClosed
		}
		return result, nil
	case <-expired:
		return "", ErrTimeout
	}
}

// sendWhenReleased owns closing out. Both potentially blocking stages observe
// cancellation, so stopping before work or while sending can release the worker.
// If cancellation and progress are both ready, either select case may win.
func sendWhenReleased(stop, release <-chan struct{}, out chan<- string) {
	defer close(out)
	select {
	case <-stop:
		return
	case <-release:
	}
	select {
	case <-stop:
		return
	case out <- "task completed":
	}
}

// workWithContext previews cooperative cancellation without a timing assumption.
// If both cases are ready, select may choose either; cancellation has no priority.
func workWithContext(ctx context.Context, release <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-release:
		return nil
	}
}

// Job is a value message: these integer fields are copied on send. Adding slice,
// map, or pointer fields would require a contract for ownership of referenced data.
type Job struct {
	ID    int
	Value int
}

type JobResult struct {
	WorkerID int
	JobID    int
	Output   int
}

func poolWorker(id int, jobs <-chan Job, results chan<- JobResult) {
	for job := range jobs {
		results <- JobResult{WorkerID: id, JobID: job.ID, Output: job.Value * 2}
	}
	// A worker cannot close results: other workers may still be sending.
}

// runWorkerPool completes a finite batch, drains every result, and joins its work.
// The caller keeps jobs unchanged until return. Results arrive in unspecified order.
// This example requires full consumption; see 02-concurrency-patterns for early exit.
func runWorkerPool(jobs []Job, workerCount int) ([]JobResult, error) {
	if workerCount < 1 {
		return nil, ErrInvalidWorkerCount
	}
	jobStream := make(chan Job)
	resultStream := make(chan JobResult)
	var workers sync.WaitGroup
	for index := 0; index < workerCount; index++ {
		workers.Add(1)
		go func(id int) {
			defer workers.Done()
			poolWorker(id, jobStream, resultStream)
		}(index + 1)
	}

	var coordinators sync.WaitGroup
	coordinators.Add(2)
	go func() {
		defer coordinators.Done()
		defer close(jobStream) // The feeder is the only sender and closer of jobs.
		for _, job := range jobs {
			jobStream <- job
		}
	}()
	go func() {
		defer coordinators.Done()
		workers.Wait()
		close(resultStream) // Only the coordinator can know all senders have returned.
	}()

	results := make([]JobResult, 0, len(jobs))
	for result := range resultStream {
		results = append(results, result)
	}
	coordinators.Wait()
	return results, nil
}
