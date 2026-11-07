package main

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

type reportStoreFunc func(context.Context, string) (Report, error)

func (lookup reportStoreFunc) Lookup(ctx context.Context, id string) (Report, error) {
	return lookup(ctx, id)
}

func TestGeneratePropagatesMetadataAndReleasesItsChild(t *testing.T) {
	parent := withRequestID(context.Background(), "req-1")
	var observed context.Context
	service := reportService{store: reportStoreFunc(func(ctx context.Context, id string) (Report, error) {
		observed = ctx
		if value, ok := requestIDFrom(ctx); !ok || value != "req-1" || id != "sales" {
			t.Fatalf("request metadata or explicit report ID was lost: %q %q", value, id)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("service did not attach its work budget")
		}
		return Report{ID: id, Title: "Quarterly sales"}, nil
	})}
	report, err := service.Generate(parent, "sales", time.Minute)
	if err != nil || report.ID != "sales" || report.Title != "Quarterly sales" {
		t.Fatalf("Generate = (%+v, %v)", report, err)
	}
	if !errors.Is(observed.Err(), context.Canceled) || parent.Err() != nil {
		t.Fatalf("service must release its child without canceling the caller: child=%v parent=%v", observed.Err(), parent.Err())
	}
}

func TestGenerateValidatesBeforeCallingTheDependency(t *testing.T) {
	calls := 0
	service := reportService{store: reportStoreFunc(func(context.Context, string) (Report, error) {
		calls++
		return Report{}, nil
	})}
	for _, budget := range []time.Duration{0, -time.Second} {
		report, err := service.Generate(context.Background(), "sales", budget)
		if report != (Report{}) || !errors.Is(err, ErrInvalidBudget) {
			t.Fatalf("invalid budget returned (%+v, %v)", report, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := service.Generate(ctx, "sales", time.Minute)
	if report != (Report{}) || !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("pre-canceled request called the dependency: report=%+v err=%v calls=%d", report, err, calls)
	}
}

func TestGenerateWrapsDependencyErrorsAndDiscardsPartialResults(t *testing.T) {
	service := reportService{store: reportStoreFunc(func(context.Context, string) (Report, error) {
		return Report{ID: "partial"}, ErrReportNotFound
	})}
	report, err := service.Generate(context.Background(), "missing", time.Minute)
	if report != (Report{}) || !errors.Is(err, ErrReportNotFound) {
		t.Fatalf("partial result or lost cause: %+v, %v", report, err)
	}
}

func TestGenerateCancelsAnAlreadyWaitingDependency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		type result struct {
			report Report
			err    error
		}
		started := make(chan context.Context, 1)
		service := reportService{store: reportStoreFunc(func(ctx context.Context, id string) (Report, error) {
			started <- ctx
			return (gatedReports{}).Lookup(ctx, id)
		})}
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		finished := make(chan struct{})
		results := make(chan result, 1)
		go func() {
			defer close(finished)
			report, err := service.Generate(ctx, "sales", time.Hour)
			results <- result{report, err}
		}()
		workCtx := <-started
		synctest.Wait() // The dependency is blocked before requesting cancellation.
		reason := errors.New("caller abandoned report")
		cancel(reason)
		<-finished
		got := <-results
		if got.report != (Report{}) || !errors.Is(got.err, context.Canceled) || !errors.Is(context.Cause(workCtx), reason) {
			t.Fatalf("propagation failed: report=%+v err=%v cause=%v", got.report, got.err, context.Cause(workCtx))
		}
	})
}

func TestGenerateHonorsTheEarlierParentOrLocalDeadline(t *testing.T) {
	for _, tt := range []struct {
		name         string
		parentBudget time.Duration
		localBudget  time.Duration
		wantElapsed  time.Duration
		parentEnds   bool
	}{
		{"parent first", 2 * time.Second, time.Hour, 2 * time.Second, true},
		{"local first", time.Hour, 3 * time.Second, 3 * time.Second, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				parent, cancel := context.WithTimeout(context.Background(), tt.parentBudget)
				defer cancel()
				var observed context.Context
				service := reportService{store: reportStoreFunc(func(ctx context.Context, id string) (Report, error) {
					observed = ctx
					return (gatedReports{}).Lookup(ctx, id)
				})}
				started := time.Now()
				report, err := service.Generate(parent, "sales", tt.localBudget)
				if report != (Report{}) || !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != tt.wantElapsed {
					t.Fatalf("unexpected deadline result: %+v, %v after %v", report, err, time.Since(started))
				}
				if tt.parentEnds != errors.Is(parent.Err(), context.DeadlineExceeded) {
					t.Fatalf("child deadline affected parent incorrectly: %v", parent.Err())
				}
				if deadline, ok := observed.Deadline(); !ok || !deadline.Equal(started.Add(tt.wantElapsed)) {
					t.Fatalf("effective dependency deadline = %v, present=%t", deadline, ok)
				}
			})
		})
	}
}

func TestReadyStoreAndAnAlreadyCanceledContext(t *testing.T) {
	ready := make(chan struct{})
	close(ready)
	store := gatedReports{ready: ready}
	if report, err := store.Lookup(context.Background(), "sales"); err != nil || report.ID != "sales" {
		t.Fatalf("ready report failed: %+v, %v", report, err)
	}
	if report, err := store.Lookup(context.Background(), "missing"); report != (Report{}) || !errors.Is(err, ErrReportNotFound) {
		t.Fatalf("missing report result: %+v, %v", report, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if report, err := store.Lookup(ctx, "sales"); report != (Report{}) || !errors.Is(err, context.Canceled) {
		t.Fatalf("already canceled lookup performed ready work: %+v, %v", report, err)
	}
}
