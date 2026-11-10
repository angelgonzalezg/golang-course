package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/labtls"
	"example.com/course/grpc-lab/internal/rpcinfra"
	"example.com/course/grpc-lab/internal/rpcobserve"
	"example.com/course/grpc-lab/internal/rpcresilience"
	"example.com/course/grpc-lab/rpcapi"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

type countedStore struct {
	*contacts.Store
	creates atomic.Int64
}

func (s *countedStore) Create(ctx context.Context, input contacts.Input) (contacts.Contact, error) {
	s.creates.Add(1)
	return s.Store.Create(ctx, input)
}

type environment struct {
	api       contactsv1.ContactServiceClient
	client    *rpcresilience.Client
	store     *countedStore
	telemetry *rpcobserve.Telemetry
	logs      *bytes.Buffer // Read only after finish joins transport/handler work.
	finish    func()
	target    string
	transport *tls.Config
}

func setup(t *testing.T, faults *lessonFaults, capacity int) *environment {
	t.Helper()
	telemetry := rpcobserve.New()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetry.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
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
	policy, err := rpcinfra.NewPolicy("reader-fixture", "writer-fixture")
	if err != nil {
		t.Fatal(err)
	}
	base := contacts.NewStore()
	if _, err := base.Create(t.Context(), contacts.Input{Name: "Seed", Email: strings.Repeat("e", 110) + "@example.com"}); err != nil {
		t.Fatal(err)
	}
	store := &countedStore{Store: base}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	logs := new(bytes.Buffer)
	config := serverConfig{
		transport: serverTLS, policy: policy, telemetry: telemetry, cache: rpcresilience.NewCreateCache(capacity),
		logger:  slog.New(slog.NewJSONHandler(&lockedWriter{out: logs}, nil)),
		receive: rpcresilience.MaxMessageBytes, send: rpcresilience.MaxMessageBytes,
	}
	if faults != nil {
		config.faults = faults.Unary()
	}
	server := startServer(listener, rpcapi.New(store, nil), config)
	connection, err := openConnection(listener.Addr().String(), clientTLS, "writer-fixture", telemetry)
	if err != nil {
		stopServer(server)
		t.Fatal(err)
	}
	var once sync.Once
	finish := func() {
		once.Do(func() {
			if err := errors.Join(connection.Close(), stopServer(server)); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(finish) // Runs before telemetry shutdown, including on test failures.
	api := contactsv1.NewContactServiceClient(connection)
	return &environment{api: api, client: rpcresilience.NewClient(api), store: store, telemetry: telemetry, logs: logs, finish: finish, target: listener.Addr().String(), transport: clientTLS}
}

func TestFiniteResilienceDemoOverTLS(t *testing.T) {
	var out bytes.Buffer
	if err := runDemo(t.Context(), &out); err != nil {
		t.Fatalf("demo: %v\n%s", err, out.String())
	}
	for _, expected := range []string{
		"Transient read -> OK after 3 attempts", "one contact added", "Same key and payload -> original result",
		"Same key, different payload -> FailedPrecondition", "Create without key -> InvalidArgument",
		"Client send limit -> ResourceExhausted", "Server receive limit -> ResourceExhausted",
		"Server send limit: large unary page -> ResourceExhausted", "Client receive limit -> ResourceExhausted",
		"13 small messages", "Persistent outage -> Unavailable", "Expired overall deadline -> DeadlineExceeded",
		"grpc.server.call.duration", "OpenTelemetry spans=", "trace_id=",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("demo omitted %q", expected)
		}
	}
}
