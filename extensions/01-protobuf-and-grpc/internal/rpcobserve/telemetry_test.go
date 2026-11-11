package rpcobserve

import (
	"context"
	"testing"
	"time"
)

// Export more completed spans than the buffer permits. Keep only the cap,
// count the excess, and prevent edits to a snapshot from changing retained state.
func TestTraceExportIsBoundedAndSnapshotsAreIndependent(t *testing.T) {
	telemetry := New()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetry.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	tracer := telemetry.Tracer.Tracer("course/test")
	for range MaxExportedSpans + 5 {
		_, span := tracer.Start(t.Context(), "bounded-export")
		span.End()
	}
	spans, dropped := telemetry.Spans.Snapshot()
	if len(spans) != MaxExportedSpans || dropped != 5 {
		t.Fatalf("span export grew past its budget: retained=%d, dropped=%d", len(spans), dropped)
	}
	spans[0].Name = "caller edit"
	again, _ := telemetry.Spans.Snapshot()
	if again[0].Name != "bounded-export" {
		t.Fatal("caller edited exporter state through a snapshot")
	}
}
