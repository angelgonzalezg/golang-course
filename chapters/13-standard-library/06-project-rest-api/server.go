package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

func startServer(listener net.Listener, handler http.Handler) (*http.Server, <-chan error) {
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    8 * 1024,
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	return server, served
}

func shutdownServer(server *http.Server, served <-chan error) error {
	// The request or signal context may already be canceled. Shutdown gets its
	// own budget so active requests have time to finish before we exit.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := server.Shutdown(ctx)
	if err != nil {
		err = errors.Join(err, server.Close())
	}
	if serveErr := <-served; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		err = errors.Join(err, fmt.Errorf("serve contacts: %w", serveErr))
	}
	return err
}
