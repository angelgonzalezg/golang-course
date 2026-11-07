package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrInvalidWorkerCount = errors.New("worker count must be positive")
	ErrInvalidQueueSize   = errors.New("queue capacity must not be negative")
	ErrMissingCallback    = errors.New("process and consume callbacks are required")
	ErrValueOutOfRange    = errors.New("value must be between -1000 and 1000")
)

// Job and Result contain only scalar values, which are copied on channel sends.
// Reference fields would need a separate ownership or synchronization contract.
type Job struct {
	ID    int
	Value int
}

type Result struct {
	Index    int // Input position, independent of potentially repeated job IDs.
	JobID    int
	WorkerID int
	Output   int
}

type indexedJob struct {
	index int
	job   Job
}

type pipelineOptions struct {
	Workers       int
	QueueCapacity int // Capacity of each stage queue, not one global capacity.
}

type processor func(context.Context, Job) (int, error)
type consumer func(context.Context, Result) error

// runPipeline borrows jobs unchanged until return. process may run concurrently;
// consume runs serially in this caller. Callbacks must return, observing ctx if
// they block. The first cancellation cause wins, and all owned stages are joined.
// Already consumed results and side effects are not rolled back on failure.
func runPipeline(ctx context.Context, jobs []Job, options pipelineOptions, process processor, consume consumer) error {
	if options.Workers < 1 {
		return ErrInvalidWorkerCount
	}
	if options.QueueCapacity < 0 {
		return ErrInvalidQueueSize
	}
	if process == nil || consume == nil {
		return ErrMissingCallback
	}
	if err := ctx.Err(); err != nil {
		return context.Cause(ctx)
	}
	if len(jobs) == 0 {
		return nil
	}

	workCtx, cancel := context.WithCancelCause(ctx)
	var stages sync.WaitGroup
	defer func() {
		// Return values are evaluated before this cleanup, so successful cleanup
		// cancellation does not turn a successful result into context.Canceled.
		cancel(nil)
		stages.Wait()
	}()
	input := produceJobs(workCtx, &stages, jobs, options.QueueCapacity)
	outputs := make([]<-chan Result, options.Workers)
	for index := range outputs {
		// Fan-out: workers compete for the SAME input. This distributes jobs;
		// it does not broadcast every job to every worker.
		outputs[index] = startWorker(workCtx, &stages, index+1, input, options.QueueCapacity, process, cancel)
	}
	merged := fanIn(workCtx, &stages, options.QueueCapacity, outputs...)
	// All Add calls happen during construction, before the deferred Wait begins.
	for {
		select {
		case <-workCtx.Done():
			return context.Cause(workCtx)
		case result, ok := <-merged:
			if !ok {
				return context.Cause(workCtx) // nil on ordinary full completion.
			}
			if err := consume(workCtx, result); err != nil {
				// Early consumer exit also stops blocked upstream sends. The defer
				// joins them without requiring this consumer to drain the stream.
				cancel(fmt.Errorf("consume input %d: %w", result.Index, err))
				return context.Cause(workCtx)
			}
		}
	}
}

func produceJobs(ctx context.Context, stages *sync.WaitGroup, jobs []Job, capacity int) <-chan indexedJob {
	out := make(chan indexedJob, capacity)
	stages.Add(1)
	go func() {
		defer stages.Done()
		defer close(out) // The producer is the only sender and closer of this queue.
		for index, job := range jobs {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case out <- indexedJob{index: index, job: job}:
			}
		}
	}()
	return out
}

func startWorker(ctx context.Context, stages *sync.WaitGroup, id int, in <-chan indexedJob, capacity int, process processor, fail context.CancelCauseFunc) <-chan Result {
	out := make(chan Result, capacity)
	stages.Add(1)
	go func() {
		defer stages.Done()
		defer close(out) // Each worker owns its own result stream.
		for {
			if ctx.Err() != nil {
				return
			}
			var item indexedJob
			select {
			case <-ctx.Done():
				return
			case value, ok := <-in:
				if !ok {
					return
				}
				item = value
			}
			if ctx.Err() != nil {
				return
			}
			value, err := process(ctx, item.job)
			if err != nil {
				fail(fmt.Errorf("worker %d input %d job %d: %w", id, item.index, item.job.ID, err))
				return
			}
			result := Result{Index: item.index, JobID: item.job.ID, WorkerID: id, Output: value}
			select {
			case <-ctx.Done():
				return
			case out <- result:
			}
		}
	}()
	return out
}

// checkedSquare uses a small explicit input domain so overflow cannot obscure
// the coordination exercise, including on 32-bit platforms.
func checkedSquare(ctx context.Context, job Job) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if job.Value < -1000 || job.Value > 1000 {
		return 0, ErrValueOutOfRange
	}
	return job.Value * job.Value, nil
}
