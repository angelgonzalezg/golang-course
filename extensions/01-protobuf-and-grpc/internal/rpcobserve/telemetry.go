// Package rpcobserve configures local OpenTelemetry metrics and traces for the
// finite resilience lab. It needs no collector and installs no global provider.
package rpcobserve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	traceoptions "google.golang.org/grpc/experimental/opentelemetry"
	"google.golang.org/grpc/stats/opentelemetry"
)

// MaxExportedSpans bounds retained summaries, not all SDK or application memory.
const MaxExportedSpans = 256

// SpanSummary keeps only identifiers needed to inspect propagation. It excludes
// metadata, message payloads, status descriptions, and unbounded event lists.
type SpanSummary struct {
	Name     string
	TraceID  trace.TraceID
	SpanID   trace.SpanID
	ParentID trace.SpanID
	Kind     trace.SpanKind
}

// SpanBuffer is a concurrency-safe local exporter. Its zero value is usable;
// after the retained-summary cap, it counts dropped summaries instead of growing.
type SpanBuffer struct {
	mu      sync.Mutex
	spans   []SpanSummary
	dropped int
}

func (b *SpanBuffer) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, span := range spans {
		if len(b.spans) == MaxExportedSpans {
			b.dropped++
			continue
		}
		ctx := span.SpanContext()
		b.spans = append(b.spans, SpanSummary{
			Name: span.Name(), TraceID: ctx.TraceID(), SpanID: ctx.SpanID(),
			ParentID: span.Parent().SpanID(), Kind: span.SpanKind(),
		})
	}
	return nil
}

// Shutdown satisfies the SDK exporter interface; this local buffer has no
// transport or background worker to close. Existing snapshots remain readable.
func (*SpanBuffer) Shutdown(context.Context) error { return nil }

// Snapshot returns independent summaries and the total number dropped so far.
func (b *SpanBuffer) Snapshot() ([]SpanSummary, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.spans), b.dropped
}

// Telemetry owns SDK providers for one lab run. Finish RPCs and transport
// shutdown before final collection, then shut down the providers themselves.
type Telemetry struct {
	Reader *sdkmetric.ManualReader
	Meter  *sdkmetric.MeterProvider
	Tracer *sdktrace.TracerProvider
	Spans  *SpanBuffer
}

// New creates a manual metric reader and a synchronous bounded span exporter.
// No collector, background export loop, or process-global provider is installed.
func New() *Telemetry {
	reader := sdkmetric.NewManualReader()
	spans := new(SpanBuffer)
	return &Telemetry{
		Reader: reader, Meter: sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)), Spans: spans,
		// AlwaysSample and synchronous export make this small local lesson
		// inspectable. Production sampling/export queues need separate sizing.
		Tracer: sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithSyncer(spans)),
	}
}

// Options instruments both peers with owned providers and trace propagation.
// The same options can be used for the lab's client and server.
func (t *Telemetry) Options() opentelemetry.Options {
	known := make(map[string]bool)
	for _, method := range contactsv1.ContactService_ServiceDesc.Methods {
		known[contactsv1.ContactService_ServiceDesc.ServiceName+"/"+method.MethodName] = true
	}
	for _, stream := range contactsv1.ContactService_ServiceDesc.Streams {
		known[contactsv1.ContactService_ServiceDesc.ServiceName+"/"+stream.StreamName] = true
	}
	for _, method := range []string{"Check", "List", "Watch"} {
		known["grpc.health.v1.Health/"+method] = true
	}
	return opentelemetry.Options{
		MetricsOptions: opentelemetry.MetricsOptions{
			MeterProvider: t.Meter,
			// Keep unknown method names in the plugin's "other" bucket. Never
			// turn request IDs, idempotency keys, or contact names into labels.
			MethodAttributeFilter: func(method string) bool { return known[strings.TrimPrefix(method, "/")] },
		},
		// In the pinned gRPC version, trace configuration still uses this
		// experimental type. Keep the version pinned when following the lesson.
		TraceOptions: traceoptions.TraceOptions{TracerProvider: t.Tracer, TextMapPropagator: propagation.TraceContext{}},
	}
}

// Collect reads the SDK's current cumulative metric snapshot without an exporter.
// RPCs still in progress can contribute to a later snapshot instead.
func (t *Telemetry) Collect(ctx context.Context) (metricdata.ResourceMetrics, error) {
	var metrics metricdata.ResourceMetrics
	err := t.Reader.Collect(ctx, &metrics)
	return metrics, err
}

// Report prints totals across method/status series and the span buffer count.
// Use Collect when inspecting individual outcomes and their attributes.
func (t *Telemetry) Report(ctx context.Context, out io.Writer) error {
	metrics, err := t.Collect(ctx)
	if err != nil {
		return err
	}
	var lines []string
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			switch data := metric.Data.(type) {
			case metricdata.Histogram[float64]:
				var count uint64
				for _, point := range data.DataPoints {
					count += point.Count
				}
				lines = append(lines, fmt.Sprintf("%s observations=%d", metric.Name, count))
			case metricdata.Sum[int64]:
				var total int64
				for _, point := range data.DataPoints {
					total += point.Value
				}
				lines = append(lines, fmt.Sprintf("%s total=%d", metric.Name, total))
			}
		}
	}
	slices.Sort(lines)
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
	spans, dropped := t.Spans.Snapshot()
	fmt.Fprintf(out, "OpenTelemetry spans=%d, dropped=%d (buffer cap=%d)\n", len(spans), dropped, MaxExportedSpans)
	return nil
}

// Shutdown closes both SDK providers. Call it after channels/server have stopped
// so transport-completion telemetry can be recorded before provider shutdown.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	return errors.Join(t.Tracer.Shutdown(ctx), t.Meter.Shutdown(ctx))
}
