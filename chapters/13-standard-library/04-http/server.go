package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func newServer(handler http.Handler) *http.Server {
	// These budgets fit this small request/response exercise. They are I/O
	// limits, not a mechanism for preempting arbitrary work inside a handler.
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    8 * 1024,
	}
}

// shutdownServer owns shutdown and joins the Serve goroutine, which must send
// its result to served exactly once. Shutdown waits for normal active handlers;
// Serve returning alone is not confirmation that those handlers have finished.
func shutdownServer(server *http.Server, served <-chan error) error {
	// A fresh context provides a shutdown budget independent of a canceled
	// request. Context propagation is explored further in chapter 14.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := server.Shutdown(ctx)
	if err != nil {
		// Fall back to closing connections, preserving the shutdown error. Close
		// cannot force arbitrary handler code to return; handlers must cooperate.
		err = errors.Join(err, server.Close())
	}
	if serveErr := <-served; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		err = errors.Join(err, fmt.Errorf("serve HTTP: %w", serveErr))
	}
	return err
}
