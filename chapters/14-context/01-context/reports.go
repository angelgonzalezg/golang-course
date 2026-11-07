package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidBudget  = errors.New("report time budget must be positive")
	ErrReportNotFound = errors.New("report not found")
)

type Report struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type reportStore interface {
	Lookup(ctx context.Context, id string) (Report, error)
}

// Services hold dependencies, while a caller's context is passed per operation.
// Context is an interface passed as a value; a *context.Context is not needed.
type reportService struct{ store reportStore }

func (service reportService) Generate(ctx context.Context, id string, budget time.Duration) (Report, error) {
	if budget <= 0 {
		return Report{}, ErrInvalidBudget
	}
	if err := ctx.Err(); err != nil {
		return Report{}, fmt.Errorf("start report: %w", err)
	}
	// Derive from the caller, rather than replacing its context with Background.
	// A shorter parent deadline still wins. Always release this child's timer
	// and parent registration, including when Lookup succeeds before the timeout.
	workCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	report, err := service.store.Lookup(workCtx, id)
	if err != nil {
		return Report{}, fmt.Errorf("generate report %q: %w", id, err)
	}
	return report, nil
}

// gatedReports is an in-memory teaching dependency, not a database. A gate lets
// examples/tests choose readiness explicitly instead of coordinating with Sleep.
// nil keeps the work unavailable; a closed gate makes it ready immediately.
type gatedReports struct{ ready <-chan struct{} }

func (store gatedReports) Lookup(ctx context.Context, id string) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	// After this check, readiness and cancellation can race. Select does not
	// prioritize cancellation, so in-flight work can still complete.
	select {
	case <-ctx.Done():
		return Report{}, ctx.Err()
	case <-store.ready:
		if id != "sales" {
			return Report{}, ErrReportNotFound
		}
		return Report{ID: id, Title: "Quarterly sales"}, nil
	}
}
