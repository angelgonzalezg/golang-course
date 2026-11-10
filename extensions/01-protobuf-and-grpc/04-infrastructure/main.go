package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/labtls"
	"example.com/course/grpc-lab/internal/rpcinfra"
	"example.com/course/grpc-lab/internal/rpcserver"
	"example.com/course/grpc-lab/rpcapi"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

const contactService = "contacts.v1.ContactService"

type tokens struct{ reader, writer string }

func (t tokens) policy() (*rpcinfra.Policy, error) { return rpcinfra.NewPolicy(t.reader, t.writer) }

type lockedWriter struct {
	mu  sync.Mutex
	out io.Writer
}

func (w *lockedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out.Write(data)
}

func startServer(listener net.Listener, service contactsv1.ContactServiceServer, transport *tls.Config, policy *rpcinfra.Policy, logger *slog.Logger, reflection bool) (*rpcserver.Running, *health.Server) {
	readiness := health.NewServer()
	// This store has no external startup dependencies. Mark readiness only after
	// constructing the usable service; real dependency probes belong here.
	readiness.SetServingStatus(contactService, healthv1.HealthCheckResponse_SERVING)
	server := rpcserver.StartWithConfig(listener, service, rpcserver.Config{
		Health: readiness, Reflection: reflection,
		ServerOptions: []grpc.ServerOption{
			grpc.Creds(credentials.NewTLS(transport)),
			// The first interceptor is outermost: it observes auth failures too.
			grpc.ChainUnaryInterceptor(rpcinfra.ObserveUnary(logger), policy.Unary()),
			grpc.ChainStreamInterceptor(rpcinfra.ObserveStream(logger), policy.Stream()),
		},
	})
	return server, readiness
}

func stopServer(server *rpcserver.Running) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func runDemo(ctx context.Context, out io.Writer) (err error) {
	// Server logs and client output can arrive concurrently, even when the
	// caller supplies a plain bytes.Buffer during a test.
	out = &lockedWriter{out: out}
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
	secrets := tokens{reader: rand.Text(), writer: rand.Text()}
	policy, err := secrets.policy()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(out, nil))
	server, _ := startServer(listener, rpcapi.New(contacts.NewStore(), nil), serverTLS, policy, logger, true)
	defer func() { err = errors.Join(err, stopServer(server)) }()
	return runClient(ctx, listener.Addr().String(), clientTLS, secrets, true, out)
}

func runServer(ctx context.Context, address string, transport *tls.Config, policy *rpcinfra.Policy, reflection bool, out io.Writer) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	server, _ := startServer(listener, rpcapi.New(contacts.NewStore(), logger), transport, policy, logger, reflection)
	fmt.Fprintf(out, "TLS contacts server: %s\nReflection enabled: %t\nStop with Ctrl+C.\n", listener.Addr(), reflection)
	select {
	case <-ctx.Done():
	case <-server.Done():
	}
	return stopServer(server)
}

func run(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("infrastructure", flag.ContinueOnError)
	serve := flags.Bool("serve", false, "run a TLS server")
	address := flags.String("addr", "127.0.0.1:50052", "listen address with -serve")
	target := flags.String("target", "", "run the finite client against an existing TLS server")
	initTLS := flags.String("init-tls", "", "create certificates in a new directory and exit")
	tlsDirectory := flags.String("tls-dir", "", "directory containing the local TLS files")
	reflection := flags.Bool("reflection", false, "enable reflection on the server or exercise it in the client")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || *serve && *target != "" || *initTLS != "" && (*serve || *target != "") {
		return errors.New("choose the demo, -init-tls, -serve, or -target")
	}
	if *initTLS != "" {
		material, err := labtls.Generate()
		if err != nil {
			return err
		}
		if err := material.WriteDirectory(*initTLS); err != nil {
			return err
		}
		fmt.Fprintf(out, "Local TLS files created in %s; certificates expire in 24 hours.\n", *initTLS)
		return nil
	}
	if !*serve && *target == "" {
		return runDemo(ctx, out)
	}
	if *tlsDirectory == "" {
		return errors.New("-tls-dir is required for standalone server/client modes")
	}
	secrets := tokens{reader: os.Getenv("GRPC_LAB_READER_TOKEN"), writer: os.Getenv("GRPC_LAB_WRITER_TOKEN")}
	policy, err := secrets.policy()
	if err != nil {
		return fmt.Errorf("GRPC_LAB_READER_TOKEN and GRPC_LAB_WRITER_TOKEN: %w", err)
	}
	if *serve {
		transport, err := labtls.LoadServer(*tlsDirectory)
		if err != nil {
			return err
		}
		return runServer(ctx, *address, transport, policy, *reflection, out)
	}
	transport, err := labtls.LoadClient(*tlsDirectory)
	if err != nil {
		return err
	}
	return runClient(ctx, *target, transport, secrets, *reflection, out)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "infrastructure lesson:", err)
		os.Exit(1)
	}
}
