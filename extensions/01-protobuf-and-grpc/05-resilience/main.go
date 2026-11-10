package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/labtls"
	"example.com/course/grpc-lab/internal/rpcinfra"
	"example.com/course/grpc-lab/internal/rpcobserve"
	"example.com/course/grpc-lab/internal/rpcresilience"
	"example.com/course/grpc-lab/internal/rpcserver"
	"example.com/course/grpc-lab/rpcapi"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/stats/opentelemetry"
)

type lockedWriter struct {
	mu  sync.Mutex
	out io.Writer
}

func (w *lockedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out.Write(data)
}

type serverConfig struct {
	transport *tls.Config
	policy    *rpcinfra.Policy
	telemetry *rpcobserve.Telemetry
	cache     *rpcresilience.CreateCache
	faults    grpc.UnaryServerInterceptor // Supplied only by this controlled lesson/test.
	logger    *slog.Logger
	receive   int
	send      int
}

func startServer(listener net.Listener, service contactsv1.ContactServiceServer, config serverConfig) *rpcserver.Running {
	unary := []grpc.UnaryServerInterceptor{rpcinfra.ObserveUnary(config.logger), config.policy.Unary()}
	if config.faults != nil {
		// Outside the cache: a simulated lost response must not replace the
		// successfully saved result with the injected transport failure.
		unary = append(unary, config.faults)
	}
	unary = append(unary, config.cache.Unary())
	ready := health.NewServer()
	ready.SetServingStatus("contacts.v1.ContactService", healthv1.HealthCheckResponse_SERVING)
	return rpcserver.StartWithConfig(listener, service, rpcserver.Config{
		Health: ready,
		ServerOptions: []grpc.ServerOption{
			grpc.Creds(credentials.NewTLS(config.transport)),
			grpc.MaxRecvMsgSize(config.receive), grpc.MaxSendMsgSize(config.send),
			opentelemetry.ServerOption(config.telemetry.Options()),
			grpc.ChainUnaryInterceptor(unary...),
			grpc.ChainStreamInterceptor(rpcinfra.ObserveStream(config.logger), config.policy.Stream(), rpcresilience.RejectStreamingKeys()),
		},
	})
}

func openConnection(target string, transport *tls.Config, token string, telemetry *rpcobserve.Telemetry) (*grpc.ClientConn, error) {
	return grpc.NewClient(target,
		grpc.WithTransportCredentials(credentials.NewTLS(transport)),
		grpc.WithPerRPCCredentials(rpcinfra.BearerToken(token)),
		grpc.WithDisableRetry(), // The teaching client owns the configured retry loop.
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(rpcresilience.MaxMessageBytes), grpc.MaxCallSendMsgSize(rpcresilience.MaxMessageBytes)),
		opentelemetry.DialOption(telemetry.Options()),
	)
}

func stopServer(server *rpcserver.Running) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func runDemo(parent context.Context, out io.Writer) (err error) {
	out = &lockedWriter{out: out}
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	telemetry := rpcobserve.New()
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		err = errors.Join(err, telemetry.Shutdown(cleanup))
	}()
	material, err := labtls.Generate()
	if err != nil {
		return err
	}
	serverTLS, err := material.ServerConfig()
	if err != nil {
		return err
	}
	clientTLS, err := labtls.ClientConfig(material.CA)
	if err != nil {
		return err
	}
	readerToken, writerToken := rand.Text(), rand.Text()
	policy, err := rpcinfra.NewPolicy(readerToken, writerToken)
	if err != nil {
		return err
	}
	store := contacts.NewStore()
	for i := range 12 {
		_, err := store.Create(ctx, contacts.Input{Name: fmt.Sprintf("Seed %02d", i), Email: strings.Repeat("e", 110) + "@example.com"})
		if err != nil {
			return err
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	faults := new(lessonFaults)
	faults.readFailures.Store(2)
	faults.dropCreate.Store(true)
	server := startServer(listener, rpcapi.New(store, nil), serverConfig{
		transport: serverTLS, policy: policy, telemetry: telemetry, cache: rpcresilience.NewCreateCache(128),
		faults: faults.Unary(), logger: slog.New(slog.NewTextHandler(out, nil)),
		receive: rpcresilience.MaxMessageBytes, send: rpcresilience.MaxMessageBytes,
	})
	err = func() (err error) {
		defer func() { err = errors.Join(err, stopServer(server)) }()
		return runClient(ctx, listener.Addr().String(), clientTLS, writerToken, telemetry, store, faults, out)
	}()
	if err != nil {
		return err
	}
	// Closing channels and joining the server above completes transport spans
	// before collection. The SDK providers are shut down after this report.
	fmt.Fprintln(out, "\n== OpenTelemetry metrics and bounded trace export ==")
	return telemetry.Report(ctx, out)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := runDemo(ctx, os.Stdout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "resilience lesson:", err)
		os.Exit(1)
	}
}
