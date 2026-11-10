// Package rpcserver owns a gRPC server's serving and shutdown goroutines.
package rpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// Config adds transport options and optional standard services. Earlier lessons
// use Start; the infrastructure lesson opts into these features explicitly.
type Config struct {
	ServerOptions []grpc.ServerOption
	Health        *health.Server
	Reflection    bool
}

// Running owns the serving/shutdown lifecycle. Call Shutdown even when Done
// has closed so active handlers and shutdown work are joined.
type Running struct {
	server   *grpc.Server
	health   *health.Server
	done     chan struct{}
	serveErr error // Read only after done closes.
}

// Start registers contacts and serves on listener using the earlier lessons'
// plaintext transport. The returned owner is responsible for Shutdown.
func Start(listener net.Listener, service contactsv1.ContactServiceServer) *Running {
	return StartWithConfig(listener, service, Config{})
}

// StartWithConfig registers all services before starting Serve. ServerOptions
// select TLS/interceptors; Health's initial statuses belong to the caller.
// Reflection must be opted into explicitly. WaitForHandlers remains enabled.
func StartWithConfig(listener net.Listener, service contactsv1.ContactServiceServer, config Config) *Running {
	// Stop must join handlers as well as network goroutines. Consequently,
	// application handlers must cooperate with their request context.
	options := append([]grpc.ServerOption{}, config.ServerOptions...)
	options = append(options, grpc.WaitForHandlers(true))
	server := grpc.NewServer(options...)
	contactsv1.RegisterContactServiceServer(server, service)
	if config.Health != nil {
		healthv1.RegisterHealthServer(server, config.Health)
	}
	if config.Reflection {
		reflection.Register(server) // Registers both v1 and v1alpha for tooling.
	}
	running := &Running{server: server, health: config.Health, done: make(chan struct{})}
	go func() {
		running.serveErr = server.Serve(listener)
		close(running.done)
	}()
	return running
}

// Done signals that Serve returned. It does not by itself mean active RPC
// handlers finished; the owner must also wait for Shutdown.
func (r *Running) Done() <-chan struct{} { return r.done }

// Shutdown publishes health NOT_SERVING and drains active RPCs. If ctx is canceled
// or expires, Stop cancels their contexts and joins handlers before returning the
// context error, possibly joined with a Serve error.
// Handlers must cooperate with cancellation; the budget cannot kill goroutines.
func (r *Running) Shutdown(ctx context.Context) error {
	if r.health != nil {
		// Notify existing watchers before draining. Watch streams themselves
		// remain open until their clients cancel or forced shutdown closes them.
		r.health.Shutdown()
	}
	drained := make(chan struct{})
	go func() {
		r.server.GracefulStop()
		close(drained)
	}()
	var err error
	select {
	case <-drained:
	case <-ctx.Done():
		err = ctx.Err()
		r.server.Stop() // Closes connections and cancels active RPC contexts.
		<-drained       // Join the graceful-stop goroutine after the fallback.
	}
	<-r.done
	if r.serveErr != nil && !errors.Is(r.serveErr, grpc.ErrServerStopped) {
		err = errors.Join(err, fmt.Errorf("serve gRPC: %w", r.serveErr))
	}
	return err
}
