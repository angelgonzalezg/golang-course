// Package rpcserver owns a gRPC server's serving and shutdown goroutines.
package rpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/grpc"
)

type Running struct {
	server   *grpc.Server
	done     chan struct{}
	serveErr error // Read only after done closes.
}

func Start(listener net.Listener, service contactsv1.ContactServiceServer) *Running {
	// Stop must join handlers as well as network goroutines. Consequently,
	// application handlers must cooperate with their request context.
	server := grpc.NewServer(grpc.WaitForHandlers(true))
	contactsv1.RegisterContactServiceServer(server, service)
	running := &Running{server: server, done: make(chan struct{})}
	go func() {
		running.serveErr = server.Serve(listener)
		close(running.done)
	}()
	return running
}

// Done signals that Serve returned. It does not by itself mean active RPC
// handlers finished; the owner must also wait for Shutdown.
func (r *Running) Done() <-chan struct{} { return r.done }

func (r *Running) Shutdown(ctx context.Context) error {
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
