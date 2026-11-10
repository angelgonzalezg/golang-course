package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/labtls"
	"example.com/course/grpc-lab/internal/rpcinfra"
	"example.com/course/grpc-lab/internal/rpcserver"
	"example.com/course/grpc-lab/rpcapi"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
)

type logBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *logBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(data)
}
func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

type environment struct {
	target    string
	transport *tls.Config
	secrets   tokens
	server    *rpcserver.Running
	health    *health.Server
	store     *contacts.Store
	logs      *logBuffer
}

func setup(t *testing.T, reflection bool, wrap func(contactsv1.ContactServiceServer) contactsv1.ContactServiceServer) *environment {
	t.Helper()
	material, err := labtls.Generate()
	if err != nil {
		t.Fatal(err)
	}
	serverTLS, err := material.ServerConfig()
	if err != nil {
		t.Fatal(err)
	}
	clientTLS, err := labtls.ClientConfig(material.CA)
	if err != nil {
		t.Fatal(err)
	}
	secrets := tokens{reader: "reader-test-secret", writer: "writer-test-secret"}
	policy, err := secrets.policy()
	if err != nil {
		t.Fatal(err)
	}
	store := contacts.NewStore()
	if _, err := store.Create(t.Context(), contacts.Input{Name: "Private contact", Email: "private@example.com"}); err != nil {
		t.Fatal(err)
	}
	var service contactsv1.ContactServiceServer = rpcapi.New(store, nil)
	if wrap != nil {
		service = wrap(service)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	logs := new(logBuffer)
	server, readiness := startServer(listener, service, serverTLS, policy, slog.New(slog.NewJSONHandler(logs, nil)), reflection)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	return &environment{target: listener.Addr().String(), transport: clientTLS, secrets: secrets, server: server, health: readiness, store: store, logs: logs}
}

func (e *environment) connection(t *testing.T, token string) *grpc.ClientConn {
	t.Helper()
	connection, err := openConnection(e.target, e.transport, token)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Error(err)
		}
	})
	return connection
}

func TestFiniteInfrastructureDemo(t *testing.T) {
	var out bytes.Buffer
	if err := runDemo(t.Context(), &out); err != nil {
		t.Fatalf("demo: %v\n%s", err, out.String())
	}
	for _, expected := range []string{
		"Health Check over verified TLS (no token) -> SERVING", "Missing token -> Unauthenticated",
		"Invalid token -> Unauthenticated", "Reader create -> PermissionDenied", "header x-request-id=infra-create",
		"Reader stream EOF / OK", "Reader import -> PermissionDenied", "Writer import -> OK",
		"Reader bidirectional lookup -> OK", "then cancellation -> Canceled", "Reflection v1 -> contacts.v1.ContactService (9 methods)",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("demo omitted %q", expected)
		}
	}
}

func TestTLSRejectsUntrustedCAAndWrongHost(t *testing.T) {
	env := setup(t, false, nil)
	for _, tc := range []struct {
		name   string
		change func(*tls.Config)
	}{
		{"untrusted CA", func(config *tls.Config) { config.RootCAs = x509.NewCertPool() }},
		{"wrong hostname", func(config *tls.Config) { config.ServerName = "elsewhere.invalid" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := env.transport.Clone()
			config.ServerName = "localhost"
			config.NextProtos = []string{"h2"}
			tc.change(config)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			connection, err := (&tls.Dialer{Config: config}).DialContext(ctx, "tcp", env.target)
			if err == nil {
				connection.Close()
				t.Fatal("TLS accepted an untrusted identity")
			}
			var certificateError *tls.CertificateVerificationError
			if !errors.As(err, &certificateError) {
				t.Fatalf("expected certificate verification failure: %v", err)
			}
		})
	}
}

func TestCallCredentialsRequireTLS(t *testing.T) {
	connection, err := grpc.NewClient("passthrough:///unused",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(rpcinfra.BearerToken("reader-test-secret")))
	if connection != nil {
		connection.Close()
	}
	if err == nil {
		t.Fatal("bearer call credentials accepted plaintext transport")
	}
}

func TestStandaloneSetupAndServerClient(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "tls")
	var out bytes.Buffer
	if err := run(t.Context(), []string{"-init-tls", directory}, &out); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRPC_LAB_READER_TOKEN", "standalone-reader")
	t.Setenv("GRPC_LAB_WRITER_TOKEN", "standalone-writer")
	transport, err := labtls.LoadServer(directory)
	if err != nil {
		t.Fatal(err)
	}
	secrets := tokens{reader: "standalone-reader", writer: "standalone-writer"}
	policy, err := secrets.policy()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan string, 1)
	done := make(chan struct{})
	var serverErr error
	go func() {
		defer close(done)
		serverErr = runServer(ctx, "127.0.0.1:0", transport, policy, true, startupWriter{started})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
			if serverErr != nil {
				t.Error(serverErr)
			}
		case <-time.After(5 * time.Second):
			t.Error("standalone server did not exit")
		}
	})
	var announcement string
	select {
	case announcement = <-started:
	case <-done:
		t.Fatalf("server exited early: %v", serverErr)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}
	line, _, _ := strings.Cut(announcement, "\n")
	target, ok := strings.CutPrefix(line, "TLS contacts server: ")
	if !ok {
		t.Fatalf("unexpected startup: %q", announcement)
	}
	for range 2 {
		if err := run(t.Context(), []string{"-target", target, "-tls-dir", directory, "-reflection"}, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
}

type startupWriter struct{ started chan string }

func (w startupWriter) Write(data []byte) (int, error) {
	w.started <- string(data)
	return len(data), nil
}
