// Package persistence demonstrates a small interface owned by its consumer.
package persistence

import (
	"context"
	"fmt"

	"golangcourse/chapters/15-testing/01-unit-tests"
)

type LabelStore interface {
	Put(context.Context, string) error
}

// SaveLabel normalizes before calling the required store dependency once.
// Invalid input and requests already canceled at entry never reach the store.
// The store is responsible for observing cancellation during its operation.
func SaveLabel(ctx context.Context, store LabelStore, input string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("save label: %w", err)
	}
	label, err := labels.Normalize(input)
	if err != nil {
		return fmt.Errorf("save label: %w", err)
	}
	if err := store.Put(ctx, label); err != nil {
		return fmt.Errorf("store label: %w", err)
	}
	return nil
}
