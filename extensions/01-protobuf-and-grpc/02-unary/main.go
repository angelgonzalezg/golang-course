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

func runDemo(ctx context.Context, out io.Writer) (err error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	server := rpcserver.Start(listener, rpcapi.New(contacts.NewStore(), nil))
	defer func() { err = errors.Join(err, stopServer(server)) }()
	return runClient(ctx, listener.Addr().String(), out)
}

func runServer(ctx context.Context, address string, out io.Writer) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	server := rpcserver.Start(listener, rpcapi.New(contacts.NewStore(), logger))
	fmt.Fprintf(out, "Unary contacts server: %s\nStop with Ctrl+C; data lives in this process.\n", listener.Addr())
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	var err error
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
