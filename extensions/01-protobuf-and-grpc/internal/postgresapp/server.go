// Package postgresapp owns the finite and standalone PostgreSQL gRPC lessons.
// Transport policy stays in rpcpostgres; transactions stay in the SQL adapter.
package postgresapp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	"example.com/course/grpc-lab/internal/rpcinfra"
	"example.com/course/grpc-lab/internal/rpcobserve"
	"example.com/course/grpc-lab/internal/rpcpostgres"
	"example.com/course/sql-lab/contactpg"
	"example.com/course/sql-lab/contactsql"
)

// Serve migrates and verifies the database before publishing startup readiness.
// Shutdown joins active RPC handlers before pool and telemetry cleanup.
func Serve(ctx context.Context, dsn, addr string, transport *tls.Config, policy *rpcinfra.Policy, reflection bool, out io.Writer) (result error) {
	if _, err := contactpg.Migrate(dsn); err != nil {
		return err
	}
	startup, cancel := context.WithTimeout(ctx, 5*time.Second)
	db, err := contactsql.Open(startup, dsn)
	cancel()
	if err != nil {
		return err
	}
	defer db.Close()
	telemetry := rpcobserve.New()
	defer func() { result = errors.Join(result, shutdownTelemetry(telemetry)) }()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server, err := rpcpostgres.Start(listener, contactpg.NewStore(db), rpcpostgres.Config{
		TLS: transport, Policy: policy, Telemetry: telemetry, Reflection: reflection,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	})
	if err != nil {
		listener.Close()
		return err
	}
	fmt.Fprintf(out, "PostgreSQL gRPC server: %s\nTLS minimum: 1.3; reflection: %t. Committed data is retained.\n", listener.Addr(), reflection)
	select {
	case <-ctx.Done():
	case <-server.Done():
	}
	return rpcpostgres.Shutdown(server)
}

// Each provider cleanup gets a fresh budget after the original call/signal ends.
func shutdownTelemetry(telemetry *rpcobserve.Telemetry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return telemetry.Shutdown(ctx)
}
