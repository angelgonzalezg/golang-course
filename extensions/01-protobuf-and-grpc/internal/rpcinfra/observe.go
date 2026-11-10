package rpcinfra

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type requestIDKey struct{}

// RequestID returns the observer's validated/generated ID, or an empty string
// when the metadata observer has not assigned one to this context.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func requestContext(ctx context.Context) (context.Context, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("x-request-id")
	id := ""
	if len(values) == 0 {
		id = rand.Text()
	} else {
		if len(values) != 1 || !validRequestID(values[0]) {
			return ctx, status.Error(codes.InvalidArgument, "x-request-id must be one 1-64 character ASCII identifier")
		}
		id = values[0]
	}
	return context.WithValue(ctx, requestIDKey{}, id), nil
}

func validRequestID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func usableLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return logger
}

func logCompletion(ctx context.Context, logger *slog.Logger, method, kind string, start time.Time, err error) {
	// Log a bounded set of fields. Do not log tokens, raw metadata, contact
	// payloads, or error text that could contain private backend information.
	attributes := []slog.Attr{
		slog.String("method", method), slog.String("kind", kind),
		slog.String("request_id", RequestID(ctx)), slog.String("code", status.Code(err).String()),
		slog.Duration("duration", time.Since(start)),
	}
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		attributes = append(attributes, slog.String("trace_id", span.TraceID().String()), slog.String("span_id", span.SpanID().String()))
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "rpc completed", attributes...)
}

// ObserveUnary runs shared steps around one unary handler: label the call, add
// response metadata, then log its result. Placing it before the permission check
// also records rejected calls. A nil logger discards logs.
func ObserveUnary(logger *slog.Logger) grpc.UnaryServerInterceptor {
	logger = usableLogger(logger)
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (response any, err error) {
		start := time.Now()
		// defer runs when this interceptor returns. The named err is then the
		// result to log, including a rejection before the handler runs.
		defer func() { logCompletion(ctx, logger, info.FullMethod, "unary", start, err) }()
		ctx, err = requestContext(ctx)
		if err != nil {
			return nil, err
		}
		if err := grpc.SetHeader(ctx, metadata.Pairs("x-request-id", RequestID(ctx))); err != nil {
			return nil, err
		}
		grpc.SetTrailer(ctx, metadata.Pairs("x-rpc-kind", "unary"))
		// next is the following interceptor, or the contact handler at the end
		// of the chain. Its result also becomes this interceptor's result.
		return next(ctx, request)
	}
}

// ObserveStream adds metadata and logs once when the entire stream handler ends.
// The handler sees the same stream with a request ID added to its context.
func ObserveStream(logger *slog.Logger) grpc.StreamServerInterceptor {
	logger = usableLogger(logger)
	return func(service any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) (err error) {
		ctx := stream.Context()
		start := time.Now()
		defer func() { logCompletion(ctx, logger, info.FullMethod, "stream", start, err) }()
		ctx, err = requestContext(ctx)
		if err != nil {
			return err
		}
		if err := stream.SetHeader(metadata.Pairs("x-request-id", RequestID(ctx))); err != nil {
			return err
		}
		stream.SetTrailer(metadata.Pairs("x-rpc-kind", "stream"))
		return next(service, contextStream{ServerStream: stream, ctx: ctx})
	}
}
