package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
)

var ErrUnknownFormat = errors.New("log format must be text or json")

type loggerConfig struct {
	Format    string
	Level     slog.Leveler
	AddSource bool
}

// newLogger borrows a non-nil writer. The application owns any flush and close.
// Empty format selects text; a nil Level uses slog's default INFO threshold.
// This constructs a local dependency without changing slog's global default.
func newLogger(writer io.Writer, config loggerConfig) (*slog.Logger, error) {
	options := &slog.HandlerOptions{
		Level: config.Level, AddSource: config.AddSource, ReplaceAttr: replaceAttr,
	}
	var handler slog.Handler
	switch config.Format {
	case "", "text":
		// Choose text for convenient reading in a terminal.
		handler = slog.NewTextHandler(writer, options)
	case "json":
		// Choose JSON for systems consuming typed fields and nested groups.
		handler = slog.NewJSONHandler(writer, options)
	default:
		return nil, fmt.Errorf("configure logging: %w", ErrUnknownFormat)
	}
	return slog.New(handler), nil
}

// replaceAttr adapts the output schema and applies an exact-key field policy.
// groups is borrowed from slog; do not retain or mutate it.
func replaceAttr(groups []string, attr slog.Attr) slog.Attr {
	if attr.Key == "token" {
		return slog.String("token", "[REDACTED]")
	}
	if len(groups) == 0 && attr.Key == slog.LevelKey {
		attr.Key = "severity"
	}
	return attr
}

// Secret owns a safe representation wherever it is logged as a structured value.
// ReplaceAttr instead applies a handler-wide rule to fields named "token".
// Neither rule rewrites arbitrary strings already embedded in a message.
type Secret string

func (Secret) LogValue() slog.Value { return slog.StringValue("[REDACTED]") }

// lazyValue defers work until a handler resolves this value on an enabled record.
// Keep this on the record: With may resolve and format attributes immediately.
type lazyValue func() slog.Value

func (value lazyValue) LogValue() slog.Value { return value() }
