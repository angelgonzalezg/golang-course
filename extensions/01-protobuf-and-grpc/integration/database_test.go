//go:build integration

// Package integration_test exercises the persistent gRPC endpoint against real
// disposable PostgreSQL. Selecting the tag requires a working container engine.
package integration_test

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/labtls"
	"example.com/course/grpc-lab/internal/rpcinfra"
	"example.com/course/grpc-lab/internal/rpcobserve"
	"example.com/course/grpc-lab/internal/rpcpostgres"
	"example.com/course/grpc-lab/internal/rpcserver"
	"example.com/course/grpc-lab/rpcapi"
	"example.com/course/sql-lab/contactpg"
	"example.com/course/sql-lab/contactsql"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

// Own a private database, independent of DATABASE_URL and the learner's Compose
// volume. Cleanup closes pools before removing this test's container.
func newDatabase(t *testing.T) (*sql.DB, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	container, err := postgres.Run(ctx, "postgres:18.6-alpine3.24", postgres.WithDatabase("grpc_lab_test"), postgres.WithUsername("grpc_lab"), postgres.WithPassword("grpc_lab_test_password"), postgres.BasicWaitStrategies())
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("start PostgreSQL (check docker info): %v", err)
	}
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db, err := contactsql.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := contactpg.Migrate(dsn); err != nil {
		t.Fatal(err)
	}
	return db, dsn
}

type endpoint struct {
	server         *rpcserver.Running
	connection     *grpc.ClientConn
	api            contactsv1.ContactServiceClient
	target         string
	clientTLS      *tls.Config
	reader, writer string
	telemetry      *rpcobserve.Telemetry
	once           sync.Once
	closeErr       error
}

// Build either the durable composition or the earlier memory-backed service.
// Both use real TCP, verified TLS 1.3, the same permissions, and owned telemetry.
func newEndpoint(t *testing.T, backend rpcpostgres.Backend, memory bool, faults grpc.UnaryServerInterceptor) *endpoint {
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
	reader, writer := rand.Text(), rand.Text()
	policy, err := rpcinfra.NewPolicy(reader, writer)
	if err != nil {
		t.Fatal(err)
	}
	telemetry := rpcobserve.New()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := telemetry.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var server *rpcserver.Running
	if memory {
		ready := health.NewServer()
		ready.SetServingStatus(rpcpostgres.ServiceName, healthv1.HealthCheckResponse_SERVING)
		server = rpcserver.StartWithConfig(listener, rpcapi.New(contacts.NewStore(), nil), rpcserver.Config{
			Health: ready,
			ServerOptions: []grpc.ServerOption{
				grpc.Creds(credentials.NewTLS(serverTLS)),
				grpc.ChainUnaryInterceptor(policy.Unary()),
				grpc.ChainStreamInterceptor(policy.Stream()),
			},
		})
	} else {
		server, err = rpcpostgres.Start(listener, backend, rpcpostgres.Config{TLS: serverTLS, Policy: policy, Telemetry: telemetry, Reflection: true, Faults: faults})
		if err != nil {
			listener.Close()
			t.Fatal(err)
		}
	}
	connection, err := rpcpostgres.OpenConnection(listener.Addr().String(), clientTLS, writer, telemetry)
	if err != nil {
		rpcpostgres.Shutdown(server)
		t.Fatal(err)
	}
	h := &endpoint{server: server, connection: connection, api: contactsv1.NewContactServiceClient(connection), target: listener.Addr().String(), clientTLS: clientTLS, reader: reader, writer: writer, telemetry: telemetry}
	t.Cleanup(func() {
		if err := h.close(); err != nil {
			t.Error(err)
		}
	})
	// Establish TLS and the channel before any cancellation/deadline scenario.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, err := healthv1.NewHealthClient(connection).Check(ctx, &healthv1.HealthCheckRequest{Service: rpcpostgres.ServiceName}); err != nil {
		t.Fatal(err)
	}
	return h
}

// close is safe for an explicit telemetry check followed by ordinary cleanup.
func (e *endpoint) close() error {
	e.once.Do(func() { e.closeErr = errors.Join(e.connection.Close(), rpcpostgres.Shutdown(e.server)) })
	return e.closeErr
}

// Add a reader/anonymous/invalid-token channel without changing the original
// writer's credentials. Its lifetime ends before this test's providers close.
func (e *endpoint) client(t *testing.T, token string) (*grpc.ClientConn, contactsv1.ContactServiceClient) {
	t.Helper()
	connection, err := rpcpostgres.OpenConnection(e.target, e.clientTLS, token, e.telemetry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	return connection, contactsv1.NewContactServiceClient(connection)
}

// Clear only service rows between serial cases in this disposable database.
func resetService(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), "TRUNCATE sql_service_create_requests, sql_service_contacts RESTART IDENTITY"); err != nil {
		t.Fatal(err)
	}
}

// Count committed effects separately from RPC replies, detecting duplicate or
// partially committed work even if a status alone looks correct.
func assertCounts(t *testing.T, db *sql.DB, wantContacts, wantResults int) {
	t.Helper()
	var rows, results int
	if err := db.QueryRowContext(t.Context(), `SELECT (SELECT count(*) FROM sql_service_contacts),(SELECT count(*) FROM sql_service_create_requests)`).Scan(&rows, &results); err != nil {
		t.Fatal(err)
	}
	if rows != wantContacts || results != wantResults {
		t.Fatalf("committed contacts/results=%d/%d, want %d/%d", rows, results, wantContacts, wantResults)
	}
}
