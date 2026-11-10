package main

import (
	"context"
	"strings"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/rpcresilience"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestTracePropagationAndTransportFailuresOutsideInterceptors(t *testing.T) {
	env := setup(t, nil, 16)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ctx, root := env.telemetry.Tracer.Tracer("course/test").Start(ctx, "end-to-end")
	rootID := root.SpanContext().TraceID()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", "correlated-read")
	if _, err := env.client.GetContact(ctx, &contactsv1.GetContactRequest{Id: 1}); err != nil {
		t.Fatal(err)
	}
	oversized := metadata.AppendToOutgoingContext(ctx, rpcresilience.KeyHeader, "private-key")
	_, err := env.api.CreateContact(oversized, request(strings.Repeat("x", 2*rpcresilience.MaxMessageBytes)), grpc.MaxCallSendMsgSize(4*rpcresilience.MaxMessageBytes))
	if err == nil {
		t.Fatal("oversized transport request succeeded")
	}
	root.End()
	env.finish() // Complete server stats.End callbacks before reading telemetry/logs.
	spans, dropped := env.telemetry.Spans.Snapshot()
	if dropped != 0 {
		t.Fatal("small fixture unexpectedly overflowed its span buffer")
	}
	byID := make(map[trace.SpanID]bool)
	for _, span := range spans {
		byID[span.SpanID] = true
	}
	linkedServer := false
	for _, span := range spans {
		if span.Kind == trace.SpanKindServer && span.TraceID == rootID && byID[span.ParentID] {
			linkedServer = true
		}
	}
	if !linkedServer {
		t.Fatalf("no server span linked to the propagated client trace: %v", spans)
	}
	logs := env.logs.String()
	if !strings.Contains(logs, `"trace_id":"`+rootID.String()+`"`) || !strings.Contains(logs, `"request_id":"correlated-read"`) {
		t.Fatalf("completion logs lost trace/request correlation: %s", logs)
	}
	if strings.Contains(logs, "private-key") || strings.Contains(logs, "writer-fixture") || strings.Contains(logs, "@example.com") {
		t.Fatal("completion logs leaked a key, credential, or payload")
	}
	metrics, err := env.telemetry.Collect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	transportFailure := false
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "grpc.server.call.duration" {
				continue
			}
			data, ok := metric.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("duration metric has unexpected aggregation: %T", metric.Data)
			}
			for _, point := range data.DataPoints {
				code, _ := point.Attributes.Value("grpc.status")
				if code.AsString() == "RESOURCE_EXHAUSTED" && point.Count > 0 {
					transportFailure = true
				}
			}
		}
	}
	if !transportFailure || env.store.creates.Load() != 0 {
		t.Fatal("transport rejection must be observable without dispatching a create handler")
	}
}
