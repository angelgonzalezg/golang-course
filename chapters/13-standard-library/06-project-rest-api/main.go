package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/httpapi"
)

func runServer(ctx context.Context, addr string, out io.Writer) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	server, served := startServer(listener, httpapi.NewHandler(contacts.NewStore(), logger))
	fmt.Fprintf(out, "Contacts API: http://%s\nStop with Ctrl+C. Data lives only in this process.\n", listener.Addr())
	select {
	case <-ctx.Done():
		return shutdownServer(server, served)
	case err := <-served:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		return errors.Join(err, server.Close())
	}
}

func main() {
	serve := flag.Bool("serve", false, "keep serving for interactive requests instead of running the finite demo")
	addr := flag.String("addr", "127.0.0.1:8080", "listen address used with -serve")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// Run cleanup before os.Exit, which does not run deferred functions.
	var err error
	if *serve {
		err = runServer(ctx, *addr, os.Stdout)
	} else {
		err = runDemo(ctx, os.Stdout)
	}
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "REST project:", err)
		os.Exit(1)
	}
	fmt.Println("Server shut down; Serve goroutine joined.")
}
