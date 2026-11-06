// HTTP
// Topic: route requests, validate JSON, reuse clients, and own server shutdown.
// Usage: go run ./chapters/13-standard-library/04-http
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
)

func showRequests(client *http.Client, baseURL string) error {
	fmt.Println("== 1. GET with a path value and an encoded query ==")
	greeting, err := getGreeting(context.Background(), client, baseURL, "Ana & Go", "es")
	if err != nil {
		return err
	}
	fmt.Println(greeting.Message)

	fmt.Println("\n== 2. POST a JSON object ==")
	greeting, err = postGreeting(context.Background(), client, baseURL, greetingRequest{Name: "  ☀  "})
	if err != nil {
		return err
	}
	fmt.Println(greeting.Message)

	fmt.Println("\n== 3. HTTP errors are different from transport errors ==")
	_, err = postGreeting(context.Background(), client, baseURL, greetingRequest{Name: " "})
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("expected status 400, got %v", err)
	}
	fmt.Println("invalid input:", statusErr)
	_, err = postGreeting(context.Background(), client, baseURL, greetingRequest{Name: strings.Repeat("x", maxRequestBytes)})
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusRequestEntityTooLarge {
		return fmt.Errorf("expected status 413, got %v", err)
	}
	fmt.Println("oversized body status:", statusErr.StatusCode)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPut, baseURL+"/health", nil)
	if err != nil {
		return err
	}
	_, err = doGreeting(client, request)
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusMethodNotAllowed {
		return fmt.Errorf("expected status 405, got %v", err)
	}
	fmt.Println("mux-generated plain-text error status:", statusErr.StatusCode)

	fmt.Println("\n== 4. GET routes also accept HEAD ==")
	request, err = http.NewRequestWithContext(context.Background(), http.MethodHead, baseURL+"/health", nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	// This direct Do call owns its response body too, even for HEAD. Close it
	// immediately after use instead of accumulating defers across a loop.
	data, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return err
	}
	fmt.Println("HEAD status:", response.StatusCode)
	fmt.Println("HEAD body bytes:", len(data))
	fmt.Println("middleware header:", response.Header.Get("X-Course"))

	fmt.Println("\n== 5. A request carries the caller's cancellation ==")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = getGreeting(ctx, client, baseURL, "Ana", "en")
	if !errors.Is(err, context.Canceled) {
		return fmt.Errorf("expected context cancellation, got %v", err)
	}
	fmt.Println("canceled request recognized:", errors.Is(err, context.Canceled))
	fmt.Println("client total timeout:", client.Timeout)
	return nil
}

func run() (err error) {
	// Bind only loopback and ask the OS for an available port. The example
	// exercises a real server without needing a fixed port or manual shutdown.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen locally: %w", err)
	}
	server := newServer(newHandler())
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	defer func() { err = errors.Join(err, shutdownServer(server, served)) }()

	client := newClient()
	// This owner created the transport. Reuse it for all requests, then release
	// idle connections after response bodies have been consumed and closed.
	defer client.CloseIdleConnections()
	if err := showRequests(client, "http://"+listener.Addr().String()); err != nil {
		return err
	}
	fmt.Println("\n== 6. HTTP rules ==")
	fmt.Println("Set headers before writing, limit incoming bodies, and validate the decoded request.")
	fmt.Println("Reuse clients, check response status and representation, and close every response body.")
	fmt.Println("Choose timeouts for each I/O phase, and wait for graceful shutdown before exiting.")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "HTTP lesson:", err)
		os.Exit(1)
	}
	fmt.Println("Local server shut down; Serve goroutine joined.")
}
