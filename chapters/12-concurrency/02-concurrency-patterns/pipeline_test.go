package main

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestPipelineProcessesEachInputExactlyOnce(t *testing.T) {
	jobs := []Job{{ID: 7, Value: -3}, {ID: 7, Value: 0}, {ID: 20, Value: 4}, {ID: 9, Value: 1000}}
	original := slices.Clone(jobs)
	for _, workers := range []int{1, 3, 8} {
		for _, capacity := range []int{0, 2} {
			var got []Result
			err := runPipeline(context.Background(), jobs, pipelineOptions{Workers: workers, QueueCapacity: capacity}, checkedSquare,
				func(_ context.Context, result Result) error {
					got = append(got, result)
					return nil
				})
			if err != nil || len(got) != len(jobs) {
				t.Fatalf("workers=%d capacity=%d results=%v err=%v", workers, capacity, got, err)
			}
			seen := make(map[int]bool)
			for _, result := range got {
				if result.Index < 0 || result.Index >= len(jobs) || seen[result.Index] {
					t.Fatalf("invalid or duplicate input position: %+v", result)
				}
				job := jobs[result.Index]
				if result.JobID != job.ID || result.Output != job.Value*job.Value || result.WorkerID < 1 || result.WorkerID > workers {
					t.Fatalf("incorrect result: %+v for %+v", result, job)
				}
				seen[result.Index] = true
			}
		}
	}
	if !slices.Equal(jobs, original) {
		t.Fatal("pipeline mutated its caller's input")
	}
}

func TestPipelineValidatesBeforeStartingAndHandlesAnEmptyBatch(t *testing.T) {
	var calls atomic.Int64
	process := func(context.Context, Job) (int, error) { calls.Add(1); return 0, nil }
	consume := func(context.Context, Result) error { calls.Add(1); return nil }
	for _, tt := range []struct {
		options pipelineOptions
		process processor
		consume consumer
		cause   error
	}{
		{pipelineOptions{}, process, consume, ErrInvalidWorkerCount},
		{pipelineOptions{Workers: -1}, process, consume, ErrInvalidWorkerCount},
		{pipelineOptions{Workers: 1, QueueCapacity: -1}, process, consume, ErrInvalidQueueSize},
		{pipelineOptions{Workers: 1}, nil, consume, ErrMissingCallback},
		{pipelineOptions{Workers: 1}, process, nil, ErrMissingCallback},
	} {
		err := runPipeline(context.Background(), []Job{{ID: 1}}, tt.options, tt.process, tt.consume)
		if !errors.Is(err, tt.cause) {
			t.Fatalf("configuration %+v: want %v, got %v", tt.options, tt.cause, err)
		}
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	reason := errors.New("caller already stopped")
	cancel(reason)
	if err := runPipeline(ctx, []Job{{ID: 1}}, pipelineOptions{Workers: 2}, process, consume); !errors.Is(err, reason) {
		t.Fatalf("pre-canceled parent cause lost: %v", err)
	}
	if err := runPipeline(context.Background(), nil, pipelineOptions{Workers: 2}, process, consume); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("invalid, pre-canceled, or empty operations invoked callbacks")
	}
}

func TestPipelineBoundsConcurrentProcessors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const workerCount = 3
		jobs := make([]Job, 20)
		for index := range jobs {
			jobs[index] = Job{ID: index, Value: index}
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		started := make(chan struct{}, len(jobs))
		release := make(chan struct{})
		var active, peak atomic.Int64
		process := func(ctx context.Context, job Job) (int, error) {
			current := active.Add(1)
			defer active.Add(-1)
			for previous := peak.Load(); current > previous; previous = peak.Load() {
				if peak.CompareAndSwap(previous, current) {
					break
				}
			}
			started <- struct{}{}
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-release:
				return job.Value, nil
			}
		}
		finished := make(chan error, 1)
		go func() {
			finished <- runPipeline(ctx, jobs, pipelineOptions{Workers: workerCount}, process,
				func(context.Context, Result) error { return nil })
		}()
		for index := 0; index < workerCount; index++ {
			<-started
		}
		synctest.Wait()
		if active.Load() != workerCount || peak.Load() != workerCount {
			t.Fatalf("active=%d peak=%d, want three blocked processors", active.Load(), peak.Load())
		}
		close(release)
		if err := <-finished; err != nil || active.Load() != 0 || peak.Load() != workerCount {
			t.Fatalf("callback bound/join failed: err=%v active=%d peak=%d", err, active.Load(), peak.Load())
		}
	})
}

func TestWorkerFailureCancelsAndJoinsItsPeer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		failure := errors.New("processor failed")
		peerStarted := make(chan struct{})
		peerFinished := make(chan struct{})
		process := func(ctx context.Context, job Job) (int, error) {
			if job.ID == 1 {
				<-peerStarted
				return 99, failure // This partial value must never become a result.
			}
			defer close(peerFinished)
			close(peerStarted)
			<-ctx.Done()
			return 0, ctx.Err()
		}
		err := runPipeline(context.Background(), []Job{{ID: 1}, {ID: 2}, {ID: 3}}, pipelineOptions{Workers: 2}, process,
			func(context.Context, Result) error {
				t.Error("failed processors must not publish partial values")
				return nil
			})
		if !errors.Is(err, failure) {
			t.Fatalf("original worker failure lost: %v", err)
		}
		select {
		case <-peerFinished:
		default:
			t.Fatal("pipeline returned before its peer completed")
		}
	})
}

func TestConsumerEarlyExitCancelsAndWaitsForProcessorCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stop := errors.New("consumer finished early")
		peerStarted := make(chan struct{})
		cancelSeen := make(chan struct{})
		cleanupRelease := make(chan struct{})
		var releaseOnce sync.Once
		defer releaseOnce.Do(func() { close(cleanupRelease) })
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		process := func(ctx context.Context, job Job) (int, error) {
			if job.ID == 1 {
				<-peerStarted
				return 1, nil
			}
			if job.ID == 2 {
				close(peerStarted)
				<-ctx.Done()
				close(cancelSeen)
				<-cleanupRelease
			} else {
				// The first worker can receive another job before the consumer
				// asks to stop. Only job 2 owns the peer/cleanup signals.
				<-ctx.Done()
			}
			return 0, ctx.Err()
		}
		consumed := 0
		finished := make(chan error, 1)
		go func() {
			finished <- runPipeline(ctx, []Job{{ID: 1}, {ID: 2}, {ID: 3}}, pipelineOptions{Workers: 2}, process,
				func(context.Context, Result) error { consumed++; return stop })
		}()
		<-cancelSeen
		synctest.Wait()
		select {
		case err := <-finished:
			t.Fatalf("returned before processor cleanup was released: %v", err)
		default:
		}
		releaseOnce.Do(func() { close(cleanupRelease) })
		if err := <-finished; !errors.Is(err, stop) || consumed != 1 {
			t.Fatalf("early consumer stop lost: count=%d err=%v", consumed, err)
		}
	})
}

func TestCallerCanCancelWhileProcessorOrConsumerIsBlocked(t *testing.T) {
	for _, phase := range []string{"processor", "consumer"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancelCause(context.Background())
				defer cancel(nil)
				entered := make(chan struct{})
				process := func(ctx context.Context, _ Job) (int, error) {
					if phase == "processor" {
						close(entered)
						<-ctx.Done()
						return 0, ctx.Err()
					}
					return 1, nil
				}
				consume := func(ctx context.Context, _ Result) error {
					close(entered)
					<-ctx.Done()
					return ctx.Err()
				}
				finished := make(chan error, 1)
				go func() { finished <- runPipeline(ctx, []Job{{ID: 1}}, pipelineOptions{Workers: 1}, process, consume) }()
				<-entered
				synctest.Wait()
				reason := errors.New("owner stopped the operation")
				cancel(reason)
				if err := <-finished; !errors.Is(err, reason) {
					t.Fatalf("caller cause was replaced: %v", err)
				}
			})
		})
	}
}

func TestPipelineHonorsTheCallerDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		started := time.Now()
		err := runPipeline(ctx, []Job{{ID: 1}}, pipelineOptions{Workers: 2},
			func(ctx context.Context, _ Job) (int, error) { <-ctx.Done(); return 0, ctx.Err() },
			func(context.Context, Result) error { return nil })
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != time.Second {
			t.Fatalf("deadline result=%v after virtual time %v", err, time.Since(started))
		}
	})
}

func TestSlowConsumerBackpressuresThePipeline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const workerCount, count = 2, 100
		jobs := make([]Job, count)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var processed atomic.Int64
		paused := make(chan struct{})
		resume := make(chan struct{})
		consumed := 0
		finished := make(chan error, 1)
		go func() {
			finished <- runPipeline(ctx, jobs, pipelineOptions{Workers: workerCount},
				func(context.Context, Job) (int, error) { processed.Add(1); return 1, nil },
				func(ctx context.Context, _ Result) error {
					consumed++
					if consumed == 1 {
						close(paused)
						select {
						case <-ctx.Done():
							return ctx.Err()
						case <-resume:
						}
					}
					return nil
				})
		}()
		<-paused
		synctest.Wait()
		// With unbuffered queues, each worker and forwarder can hold one result,
		// plus the consumer's current result. Work cannot race through all jobs.
		if got := processed.Load(); got < 1 || got > 2*workerCount+1 {
			t.Fatalf("slow consumer did not bound progress: processed=%d", got)
		}
		close(resume)
		if err := <-finished; err != nil || consumed != count || processed.Load() != count {
			t.Fatalf("resuming lost work: err=%v consumed=%d processed=%d", err, consumed, processed.Load())
		}
	})
}

func TestSuccessReleasesOnlyThePipelineContext(t *testing.T) {
	type metadataKey struct{}
	parent := context.WithValue(context.Background(), metadataKey{}, "req-1")
	var observed context.Context
	err := runPipeline(parent, []Job{{ID: 1}}, pipelineOptions{Workers: 1},
		func(ctx context.Context, _ Job) (int, error) { observed = ctx; return 1, nil },
		func(ctx context.Context, _ Result) error {
			if ctx.Value(metadataKey{}) != "req-1" {
				t.Error("caller metadata did not reach consumer")
			}
			return nil
		})
	if err != nil || !errors.Is(observed.Err(), context.Canceled) || parent.Err() != nil {
		t.Fatalf("success/cleanup contract failed: err=%v child=%v parent=%v", err, observed.Err(), parent.Err())
	}
}
