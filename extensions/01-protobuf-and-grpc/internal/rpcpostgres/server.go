package rpcpostgres

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net"
	"time"

	"example.com/course/grpc-lab/internal/rpcinfra"
	"example.com/course/grpc-lab/internal/rpcobserve"
	"example.com/course/grpc-lab/internal/rpcresilience"
	"example.com/course/grpc-lab/internal/rpcserver"
	"example.com/course/grpc-lab/rpcapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/stats/opentelemetry"
)

const (
	// UnaryBudget also bounds pool/row/key waits when a caller supplies no deadline.
	UnaryBudget = 3 * time.Second
	// StreamBudget bounds this finite teaching protocol, not each individual message.
	StreamBudget = 15 * time.Second
	ServiceName  = "contacts.v1.ContactService"
)

// Config supplies resources owned by the application. Faults is a local test/demo
// hook only; no incoming metadata can enable failures or bypass authorization.
type Config struct {
	TLS        *tls.Config
	Policy     *rpcinfra.Policy
	Telemetry  *rpcobserve.Telemetry
	Logger     *slog.Logger
	Reflection bool
	Faults     grpc.UnaryServerInterceptor
}

// Start composes the existing RPC service with verified TLS, permissions,
// deadlines, durable creation, message bounds, health, and optional reflection.
// The caller must migrate/ping its pool before startup readiness is published.
func Start(listener net.Listener, backend Backend, config Config) (*rpcserver.Running, error) {
	if config.TLS == nil || config.Policy == nil || config.Telemetry == nil || backend == nil {
		return nil, errors.New("TLS, policy, telemetry, and a usable backend are required")
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	transport := config.TLS.Clone()
	transport.MinVersion = max(transport.MinVersion, tls.VersionTLS13)
	unary := []grpc.UnaryServerInterceptor{rpcinfra.ObserveUnary(logger), config.Policy.Unary(), unaryBudget}
	if config.Faults != nil {
		unary = append(unary, config.Faults)
	}
	// Authorization runs on EVERY invocation, including a replay. The fault hook
	// wraps the committed operation, so a hidden reply cannot replace its result.
	unary = append(unary, Unary())
	ready := health.NewServer()
	ready.SetServingStatus(ServiceName, healthv1.HealthCheckResponse_SERVING)
	return rpcserver.StartWithConfig(listener, rpcapi.New(NewStore(backend), logger), rpcserver.Config{
		Health: ready, Reflection: config.Reflection,
		ServerOptions: []grpc.ServerOption{
			grpc.Creds(credentials.NewTLS(transport)),
			grpc.MaxRecvMsgSize(rpcresilience.MaxMessageBytes), grpc.MaxSendMsgSize(rpcresilience.MaxMessageBytes),
			opentelemetry.ServerOption(config.Telemetry.Options()),
			grpc.ChainUnaryInterceptor(unary...),
			grpc.ChainStreamInterceptor(rpcinfra.ObserveStream(logger), config.Policy.Stream(), streamBudget, rpcresilience.RejectStreamingKeys()),
		},
	}), nil
}

// OpenConnection configures verified TLS, call credentials, size bounds, and
// owned telemetry. The existing resilience client supplies application retries.
// The caller owns Close; NewClient defers connecting until an RPC needs it.
func OpenConnection(target string, transport *tls.Config, token string, telemetry *rpcobserve.Telemetry) (*grpc.ClientConn, error) {
	if transport == nil || transport.InsecureSkipVerify || telemetry == nil {
		return nil, errors.New("verified TLS and owned telemetry are required")
	}
	transport = transport.Clone()
	transport.MinVersion = max(transport.MinVersion, tls.VersionTLS13)
	options := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(transport)), grpc.WithDisableRetry(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(rpcresilience.MaxMessageBytes), grpc.MaxCallSendMsgSize(rpcresilience.MaxMessageBytes)),
		opentelemetry.DialOption(telemetry.Options()),
	}
	if token != "" {
		options = append(options, grpc.WithPerRPCCredentials(rpcinfra.BearerToken(token)))
	}
	return grpc.NewClient(target, options...)
}

// Shutdown remains usable after a request or signal deadline has expired.
// rpcserver joins serving and handler work before the application closes SQL.
func Shutdown(server *rpcserver.Running) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func unaryBudget(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, UnaryBudget)
	defer cancel()
	return next(ctx, request)
}

// Preserve the stream's transport methods while adding a bounded context.
type boundedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s boundedStream) Context() context.Context { return s.ctx }

func streamBudget(service any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
	ctx, cancel := context.WithTimeout(stream.Context(), StreamBudget)
	defer cancel()
	return next(service, boundedStream{ServerStream: stream, ctx: ctx})
}
