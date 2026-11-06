package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"testing/slogtest"
	"time"
)

func TestFailureCountingHandlerPreservesTheHandlerContract(t *testing.T) {
	var output bytes.Buffer
	var failures atomic.Uint64
	handler := &failureCountingHandler{next: slog.NewJSONHandler(&output, nil), failures: &failures}
	// Use the standard conformance suite with the standard schema. The custom
	// factory's field policies are tested separately from handler delegation.
	err := slogtest.TestHandler(handler, func() []map[string]any {
		return decodeRecords(t, output.String())
	})
	if err != nil {
		t.Fatal(err)
	}
	if failures.Load() != 0 {
		t.Fatal("successful writes were counted as failures")
	}
}

func TestFailureCountingHandlerPreservesTheOutputError(t *testing.T) {
	failure := errors.New("writer unavailable")
	var failures atomic.Uint64
	handler := &failureCountingHandler{next: slog.NewJSONHandler(brokenWriter{failure}, nil), failures: &failures}
	// Direct Handle is used here to test the wrapper's error contract. Application
	// code uses Logger methods, which deliberately discard this returned error.
	err := handler.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelInfo, "record", 0))
	if !errors.Is(err, failure) || failures.Load() != 1 {
		t.Fatalf("lost output error or observation: err=%v failures=%d", err, failures.Load())
	}
}

func TestFailureCounterIsSharedAcrossConcurrentDerivedLoggers(t *testing.T) {
	var failures atomic.Uint64
	handler := &failureCountingHandler{
		next:     slog.NewJSONHandler(brokenWriter{errors.New("output failed")}, nil),
		failures: &failures,
	}
	logger := slog.New(handler)
	logger.Debug("filtered record must not count as a write failure")
	const workerCount, perWorker = 8, 40
	var workers sync.WaitGroup
	for id := 0; id < workerCount; id++ {
		workers.Add(1)
		go func(id int) {
			defer workers.Done()
			child := logger.With("worker_id", id).WithGroup("request")
			for index := 0; index < perWorker; index++ {
				child.Error("operation failed", slog.Int("sequence", index))
			}
		}(id)
	}
	workers.Wait()
	if got, want := failures.Load(), uint64(workerCount*perWorker); got != want {
		t.Fatalf("handler failures=%d, want %d", got, want)
	}
}
