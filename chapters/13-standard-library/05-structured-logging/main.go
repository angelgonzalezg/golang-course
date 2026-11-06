// Structured logging
// Topic: choose levels and handlers, enrich records, and own logging policy.
// Usage: go run ./chapters/13-standard-library/05-structured-logging
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

func showFormats(out io.Writer) error {
	fmt.Fprintln(out, "== 1. Text for reading and JSON for structured consumers ==")
	for _, format := range []string{"text", "json"} {
		logger, err := newLogger(out, loggerConfig{Format: format})
		if err != nil {
			return err
		}
		// Ordinary key/value pairs are convenient; keep keys stable for queries.
		logger.Info("service ready", "service", "course", "workers", 3)
		// LogAttrs accepts only typed Attr values. A duration is rendered readably
		// in text and as integer nanoseconds in the built-in JSON handler.
		logger.LogAttrs(context.Background(), slog.LevelInfo, "batch completed",
			slog.Int("count", 2), slog.Duration("elapsed", 12*time.Millisecond))
	}
	return nil
}

func showLevels(out io.Writer) error {
	fmt.Fprintln(out, "\n== 2. Severity thresholds and dynamic levels ==")
	var minimum slog.LevelVar // Zero value is INFO; safe for concurrent updates.
	logger, err := newLogger(out, loggerConfig{Format: "text", Level: &minimum})
	if err != nil {
		return err
	}
	logger.Debug("hidden while minimum is INFO")
	logger.Info("normal operation")
	logger.Warn("fallback selected", "fallback", true)
	logger.Error("operation failed", "retryable", false)
	// Error records an event; slog has no built-in Fatal method that exits.
	fmt.Fprintln(out, "Execution continues after logger.Error.")
	minimum.Set(slog.LevelDebug)
	logger.Debug("visible after lowering the threshold")
	var configured slog.Level
	if err := configured.UnmarshalText([]byte("WARN")); err != nil {
		return err
	}
	fmt.Fprintln(out, "parsed standard level:", configured)
	fmt.Fprintln(out, "standard numeric levels:", int(slog.LevelDebug), int(slog.LevelInfo), int(slog.LevelWarn), int(slog.LevelError))
	return nil
}

func showGroupsAndPolicy(out io.Writer) error {
	fmt.Fprintln(out, "\n== 3. Bind shared metadata and group related attributes ==")
	logger, err := newLogger(out, loggerConfig{Format: "json"})
	if err != nil {
		return err
	}
	service := logger.With(slog.String("service", "checkout"))
	request := service.WithGroup("request").With(slog.String("id", "req-42"))
	request.Info("request completed", slog.Int("status", 200))
	// With and WithGroup derive a logger; they do not mutate the parent.
	service.Info("background ready", slog.Group("worker", slog.Int("count", 3)))

	fmt.Fprintln(out, "\n== 4. Value-owned representation and handler-wide field policy ==")
	logger.Info("credentials configured",
		slog.Any("credential", Secret("example credential")),
		slog.Group("auth", slog.String("token", "example token")))
	// ReplaceAttr renames only the root level key to severity. An application's
	// nested attribute named level remains application data.
	logger.Info("domain data", slog.Group("game", slog.Int("level", 7)))
	return nil
}

func showDeferredWork(out io.Writer) error {
	fmt.Fprintln(out, "\n== 5. Disabled records still evaluate ordinary arguments ==")
	var minimum slog.LevelVar
	logger, err := newLogger(out, loggerConfig{Format: "text", Level: &minimum})
	if err != nil {
		return err
	}
	builds := 0
	build := func() string {
		builds++
		return "computed snapshot"
	}
	// This deliberately demonstrates eager argument evaluation, not a benchmark.
	logger.Debug("filtered eager record", slog.String("snapshot", build()))
	if logger.Enabled(context.Background(), slog.LevelDebug) {
		logger.Debug("gated record", slog.String("snapshot", build()))
	}
	fmt.Fprintln(out, "builds after eager and guarded disabled calls:", builds)
	value := lazyValue(func() slog.Value { return slog.StringValue(build()) })
	logger.Debug("filtered deferred record", slog.Any("snapshot", value))
	fmt.Fprintln(out, "builds after deferred disabled call:", builds)
	minimum.Set(slog.LevelDebug)
	logger.Debug("enabled deferred record", slog.Any("snapshot", value))
	fmt.Fprintln(out, "builds after deferred enabled call:", builds)
	return nil
}

func showOperationBoundary(out io.Writer) error {
	fmt.Fprintln(out, "\n== 6. Inject a logger and record an operation's outcome once ==")
	logger, err := newLogger(out, loggerConfig{Format: "json"})
	if err != nil {
		return err
	}
	// Built-in handlers do not automatically extract request_id from context.
	// Attach it explicitly; pass an available context through LogAttrs/InfoContext.
	requestLogger := logger.With(slog.String("request_id", "req-101"))
	if err := processOrder(context.Background(), requestLogger, "order-1", 2); err != nil {
		return err
	}
	if err := processOrder(context.Background(), requestLogger, "order-2", 0); !errors.Is(err, ErrInvalidOrder) {
		return fmt.Errorf("expected invalid order, got %v", err)
	}
	fmt.Fprintln(out, "The caller handled the returned error without logging it again.")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Cancellation does not suppress a log record; it may explain the failure.
	requestLogger.InfoContext(ctx, "request cancellation observed")
	return nil
}

type brokenWriter struct{ err error }

func (writer brokenWriter) Write([]byte) (int, error) { return 0, writer.err }

func showHandlerFailures(out io.Writer) {
	fmt.Fprintln(out, "\n== 7. Observe output errors in a handler wrapper ==")
	var failures atomic.Uint64
	handler := &failureCountingHandler{
		next:     slog.NewJSONHandler(brokenWriter{errors.New("destination unavailable")}, nil),
		failures: &failures,
	}
	logger := slog.New(handler).With(slog.String("service", "mail")).WithGroup("request")
	// Logger methods have no error return. The wrapper observes Handle's error
	// and shares its counter across With/WithGroup derivatives.
	logger.Error("delivery failed", slog.String("id", "req-3"))
	fmt.Fprintln(out, "observed handler errors:", failures.Load())
}

func showSource(out io.Writer) error {
	fmt.Fprintln(out, "\n== 8. Add source locations when the diagnostic value is useful ==")
	logger, err := newLogger(out, loggerConfig{Format: "text", AddSource: true})
	if err != nil {
		return err
	}
	// This reports this call site. A wrapper around logger.Info would report the
	// wrapper's location unless it deliberately preserved the caller's PC.
	logger.Info("source location included")
	return nil
}

func run(out io.Writer) error {
	for _, section := range []func(io.Writer) error{showFormats, showLevels, showGroupsAndPolicy, showDeferredWork, showOperationBoundary} {
		if err := section(out); err != nil {
			return err
		}
	}
	showHandlerFailures(out)
	if err := showSource(out); err != nil {
		return err
	}
	fmt.Fprintln(out, "\n== 9. Logging rules ==")
	fmt.Fprintln(out, "Choose severity and stable attributes; use text or JSON to suit the consumer.")
	fmt.Fprintln(out, "Inject loggers, derive shared metadata, and log failures once where they are handled.")
	fmt.Fprintln(out, "Defer expensive values when useful; own writer cleanup and observe output failures deliberately.")
	return nil
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "structured logging lesson:", err)
		os.Exit(1)
	}
}
