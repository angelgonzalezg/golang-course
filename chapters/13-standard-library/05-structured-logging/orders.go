package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

var ErrInvalidOrder = errors.New("order needs an ID and a positive item count")

// validateOrder is a lower-level helper: it returns an error with context and
// leaves the logging decision to the operation boundary.
func validateOrder(id string, items int) error {
	if strings.TrimSpace(id) == "" || items <= 0 {
		return fmt.Errorf("validate order: %w", ErrInvalidOrder)
	}
	return nil
}

// processOrder is this example's application boundary. An injected logger keeps
// the destination/configuration outside the operation and makes records testable.
// The caller handles the returned error without logging this same failure again.
func processOrder(ctx context.Context, logger *slog.Logger, id string, items int) error {
	if err := validateOrder(id, items); err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "order rejected",
			slog.String("order_id", id), slog.Any("error", err))
		return err
	}
	// This small operation validates and records acceptance; it has no database
	// or background worker. Context is passed to the handler, not stored in a struct.
	logger.LogAttrs(ctx, slog.LevelInfo, "order accepted",
		slog.String("order_id", id), slog.Int("items", items))
	return nil
}
