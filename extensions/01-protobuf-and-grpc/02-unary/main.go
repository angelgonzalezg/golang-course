// Unary contacts: choose a self-contained demo, a server, or a generated client.
// This file owns startup/shutdown; client.go walks through individual RPC calls.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/course/grpc-lab/internal/rpcserver"
	"example.com/course/grpc-lab/rpcapi"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

func stopServer(server *rpcserver.Running) error {
	// The signal or client context may already be canceled. Give shutdown its
	// own budget, then fall back to Stop if active RPCs do not drain in time.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

// runDemo keeps both sides in this process, while using a real TCP connection.
func runDemo(ctx context.Context, out io.Writer) (err error) {
	// Port 0 asks the OS for an available port, avoiding a fixed-port requirement.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	// The store holds data, rpcapi translates RPCs, and rpcserver owns Serve.
	server := rpcserver.Start(listener, rpcapi.New(contacts.NewStore(), nil))
	defer func() { err = errors.Join(err, stopServer(server)) }()
	return runClient(ctx, listener.Addr().String(), out)
}

// runServer gives an interactive client a longer-lived in-memory store.
func runServer(ctx context.Context, address string, out io.Writer) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	server := rpcserver.Start(listener, rpcapi.New(contacts.NewStore(), logger))
	fmt.Fprintf(out, "Unary contacts server: %s\nStop with Ctrl+C; data lives in this process.\n", listener.Addr())
	// Stop when the caller cancels or Serve ends unexpectedly; both paths join
	// server work through stopServer before returning to main.
	select {
	case <-ctx.Done():
	case <-server.Done():
	}
	return stopServer(server)
}

func main() {
	serve := flag.Bool("serve", false, "keep a local server running for the generated client")
	address := flag.String("addr", "127.0.0.1:50051", "listen address with -serve")
	target := flag.String("target", "", "run the finite client examples against an existing server")
	flag.Parse()
	// Ctrl+C becomes cancellation that the chosen mode can observe.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	var err error
	// With no flags the lesson is finite. -serve and -target split its two sides.
	switch {
	case *serve && *target != "":
		err = errors.New("choose -serve or -target, not both")
	case *serve:
		err = runServer(ctx, *address, os.Stdout)
	case *target != "":
		err = runClient(ctx, *target, os.Stdout)
	default:
		err = runDemo(ctx, os.Stdout)
	}
	stop() // Cleanup finishes before os.Exit, which skips deferred calls.
	if err != nil {
		fmt.Fprintln(os.Stderr, "unary lesson:", err)
		os.Exit(1)
	}
	fmt.Println("Owned resources closed; any started server and shutdown goroutines joined.")
}
