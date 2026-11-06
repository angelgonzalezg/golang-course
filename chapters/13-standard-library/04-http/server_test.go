package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClientAndServerOverLocalHTTP(t *testing.T) {
	server := httptest.NewServer(newHandler())
	defer server.Close()
	client := newClient()
	defer client.CloseIdleConnections()

	got, err := getGreeting(context.Background(), client, server.URL, "Ana & Go? ☀ + / #", "es")
	if err != nil || got.Message != "Hola, Ana & Go? ☀ + / #!" {
		t.Fatalf("GET escaping failed: %+v, %v", got, err)
	}
	got, err = postGreeting(context.Background(), client, server.URL, greetingRequest{Name: "  Ana  "})
	if err != nil || got != (Greeting{Message: "Hello, Ana!", Language: "en"}) {
		t.Fatalf("POST failed: %+v, %v", got, err)
	}

	// A real server suppresses the HEAD body. A direct ResponseRecorder call
	// alone does not model that wire-level behavior.
	response, err := client.Head(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || len(data) != 0 || response.Header.Get("X-Course") != "learning-go" {
		t.Fatalf("HEAD status=%d body=%q headers=%v", response.StatusCode, data, response.Header)
	}

	// Hide the reader's known length so the transport sends an unknown-length
	// body. The server must enforce the same limit without Content-Length.
	request, err := http.NewRequestWithContext(context.Background(), "POST", server.URL+"/greetings", io.NopCloser(strings.NewReader(strings.Repeat("x", maxRequestBytes+1))))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	_, err = doGreeting(client, request)
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != 413 {
		t.Fatalf("unknown-length oversized body must fail with 413: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := getGreeting(ctx, client, server.URL, "Ana", "en"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request lost its cause: %v", err)
	}
}

func TestSharedClientAndHandlerSupportConcurrentRequests(t *testing.T) {
	server := httptest.NewServer(newHandler())
	defer server.Close()
	client := newClient()
	defer client.CloseIdleConnections()
	var workers sync.WaitGroup
	for index := 0; index < 12; index++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			name := fmt.Sprintf("person %d & Go", index)
			got, err := getGreeting(context.Background(), client, server.URL, name, "es")
			if err != nil || got.Message != "Hola, "+name+"!" {
				t.Errorf("request %d: %+v, %v", index, got, err)
			}
		}(index)
	}
	workers.Wait()
}

func TestClientSupportsHTTPSWithTheTestServersTrustedClient(t *testing.T) {
	server := httptest.NewTLSServer(newHandler())
	defer server.Close()
	client := server.Client() // Trusts this fixture's certificate; verification stays enabled.
	client.Timeout = 5 * time.Second
	defer client.CloseIdleConnections()
	got, err := getGreeting(context.Background(), client, server.URL, "Ana", "en")
	if err != nil || got.Message != "Hello, Ana!" {
		t.Fatalf("HTTPS request failed: %+v, %v", got, err)
	}
}

func awaitHTTP[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	watchdog := time.NewTimer(5 * time.Second)
	defer watchdog.Stop()
	select {
	case value := <-result:
		return value
	case <-watchdog.C:
		t.Fatal("HTTP test coordination did not complete")
		var zero T
		return zero
	}
}

func TestShutdownWaitsForTheActiveHandlerAndJoinsServe(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	server := newServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		writeJSON(w, 200, Greeting{Message: "Hello, Ana!", Language: "en"})
	}))
	shutdownStarted := make(chan struct{})
	server.RegisterOnShutdown(func() { close(shutdownStarted) })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	serveFinished := make(chan struct{})
	requestFinished := make(chan struct{})
	shutdownFinished := make(chan struct{})
	shutdownLaunched := false
	client := newClient()
	// Release gates and join test-owned work even after a failed assertion.
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		if err := server.Close(); err != nil {
			t.Errorf("cleanup close: %v", err)
		}
		client.CloseIdleConnections()
		awaitHTTP(t, serveFinished)
		awaitHTTP(t, requestFinished)
		if shutdownLaunched {
			awaitHTTP(t, shutdownFinished)
		}
	})
	go func() {
		defer close(serveFinished)
		served <- server.Serve(listener)
	}()
	requestResult := make(chan error, 1)
	go func() {
		defer close(requestFinished)
		_, err := getGreeting(context.Background(), client, "http://"+listener.Addr().String(), "Ana", "en")
		requestResult <- err
	}()
	awaitHTTP(t, entered)
	shutdownResult := make(chan error, 1)
	shutdownLaunched = true
	go func() {
		defer close(shutdownFinished)
		shutdownResult <- shutdownServer(server, served)
	}()
	awaitHTTP(t, shutdownStarted)
	select {
	case err := <-shutdownResult:
		t.Fatalf("shutdown returned before the active handler finished: %v", err)
	default:
	}
	releaseOnce.Do(func() { close(release) })
	if err := awaitHTTP(t, requestResult); err != nil {
		t.Fatalf("active request did not finish normally: %v", err)
	}
	if err := awaitHTTP(t, shutdownResult); err != nil {
		t.Fatalf("graceful shutdown failed: %v", err)
	}
	awaitHTTP(t, serveFinished)
}
