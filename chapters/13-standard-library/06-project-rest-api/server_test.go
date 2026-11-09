package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type readyWriter struct{ started chan string }

func (w readyWriter) Write(p []byte) (int, error) {
	w.started <- string(p)
	return len(p), nil
}

func TestInteractiveServerStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	writer := readyWriter{started: make(chan string, 1)}
	done := make(chan error, 1)
	go func() { done <- runServer(ctx, "127.0.0.1:0", writer) }()
	select {
	case text := <-writer.started:
		if !strings.Contains(text, "Contacts API: http://127.0.0.1:") {
			t.Fatalf("unexpected startup message: %q", text)
		}
	case err := <-done:
		t.Fatalf("server exited before startup: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not finish shutdown")
	}
}

func TestShutdownWaitsForActiveRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	releaseHandler := func() { once.Do(func() { close(release) }) }
	defer releaseHandler()
	server, served := startServer(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	requested := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(t.Context(), "GET", "http://"+listener.Addr().String(), nil)
		if err != nil {
			requested <- err
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			requested <- err
			return
		}
		_, readErr := io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != http.StatusNoContent {
			readErr = errors.Join(readErr, errors.New("active request did not finish normally"))
		}
		requested <- errors.Join(readErr, resp.Body.Close())
	}()
	select {
	case <-entered:
	case err := <-requested:
		t.Fatalf("request ended before handler entered: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not start")
	}
	shutting := make(chan struct{})
	server.RegisterOnShutdown(func() { close(shutting) })
	stopped := make(chan error, 1)
	go func() { stopped <- shutdownServer(server, served) }()
	<-shutting
	select {
	case err := <-stopped:
		t.Fatalf("shutdown returned before active request finished: %v", err)
	default:
	}
	releaseHandler()
	if err := <-requested; err != nil {
		t.Fatal(err)
	}
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
}
