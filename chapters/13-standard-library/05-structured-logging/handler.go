package main

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// failureCountingHandler observes errors that Logger methods otherwise discard.
// It delegates formatting, filtering, groups, and synchronized writes to next.
// Every derived handler shares the counter; no atomic value is copied after use.
type failureCountingHandler struct {
	next     slog.Handler
	failures *atomic.Uint64
}

var _ slog.Handler = (*failureCountingHandler)(nil)

func (handler *failureCountingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.next.Enabled(ctx, level)
}

func (handler *failureCountingHandler) Handle(ctx context.Context, record slog.Record) error {
	// The record is forwarded unchanged. A wrapper that mutates its attributes
	// would first need Record.Clone because a plain copy can share backing state.
	err := handler.next.Handle(ctx, record)
	if err != nil {
		// Do not log this failure through the same failing handler recursively.
		handler.failures.Add(1)
	}
	return err
}

func (handler *failureCountingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &failureCountingHandler{next: handler.next.WithAttrs(attrs), failures: handler.failures}
}

func (handler *failureCountingHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return handler
	}
	return &failureCountingHandler{next: handler.next.WithGroup(name), failures: handler.failures}
}
