// Streaming contacts adds three message exchange shapes to the unary service.
// Startup stays here; client.go explains sending, receiving, and completion.
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

// stopServer uses a fresh budget because the run's context may already be canceled.
// Shutdown drains active calls, then forces cancellation if its budget expires.
func stopServer(server *rpcserver.Running) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

// runDemo owns a temporary TCP server and closes it even if a client example fails.
func runDemo(ctx context.Context, out io.Writer) (err error) {
	// An available loopback port keeps this finite mode independent of port 50051.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	// The same service implements unary and streaming methods over one store.
	server := rpcserver.Start(listener, rpcapi.New(contacts.NewStore(), nil))
	defer func() { err = errors.Join(err, stopServer(server)) }()
	return runClient(ctx, listener.Addr().String(), out)
}

// runServer keeps its store alive across client runs, until this process exits.
func runServer(ctx context.Context, address string, out io.Writer) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	server := rpcserver.Start(listener, rpcapi.New(contacts.NewStore(), logger))
	fmt.Fprintf(out, "Streaming contacts server: %s\nStop with Ctrl+C; data lives in this process.\n", listener.Addr())
	// A signal or an unexpected Serve exit leads through the same cleanup path.
	select {
	case <-ctx.Done():
	case <-server.Done():
	}
	return stopServer(server)
}

func main() {
	serve := flag.Bool("serve", false, "keep a local server running for the generated client")
	address := flag.String("addr", "127.0.0.1:50051", "listen address with -serve")
	target := flag.String("target", "", "run the finite streaming examples against an existing server")
	flag.Parse()
	// The signal context reaches client calls or the long-running server owner.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	var err error
	// Default: run both sides and exit. Flags can run either side separately.
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
	// Finish owned cleanup before os.Exit, which would skip deferred calls.
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "streaming lesson:", err)
		os.Exit(1)
	}
	fmt.Println("Streams completed, sender joined, and owned resources closed.")
}
