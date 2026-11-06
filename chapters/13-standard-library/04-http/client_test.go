package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type observedBody struct {
	io.Reader
	closes   int
	closeErr error
}

func (body *observedBody) Close() error {
	body.closes++
	return body.closeErr
}

func TestClientChecksResponsesAndClosesBodies(t *testing.T) {
	readFailure := errors.New("read failed")
	closeFailure := errors.New("close failed")
	const success = `{"message":"Hello, Ana!","language":"en"}`
	tests := []struct {
		name        string
		status      int
		contentType string
		data        string
		readErr     error
		closeErr    error
		wantErr     error
		wantStatus  int
	}{
		{"success", 200, "application/json", success, nil, nil, nil, 0},
		{"exact size limit", 200, "application/json", success + strings.Repeat(" ", maxResponseBytes-len(success)), nil, nil, nil, 0},
		{"additive field", 200, "application/json; charset=utf-8", `{"message":"Hello, Ana!","language":"en","new":true}`, nil, nil, nil, 0},
		{"wrong content type", 200, "text/html", success, nil, nil, ErrInvalidResponse, 0},
		{"invalid JSON", 200, "application/json", `{`, nil, nil, ErrInvalidResponse, 0},
		{"trailing JSON", 200, "application/json", success + `{}`, nil, nil, ErrInvalidResponse, 0},
		{"null", 200, "application/json", `null`, nil, nil, ErrInvalidResponse, 0},
		{"missing fields", 200, "application/json", `{}`, nil, nil, ErrInvalidResponse, 0},
		{"wrong field type", 200, "application/json", `{"message":42}`, nil, nil, ErrInvalidResponse, 0},
		{"wrong language", 200, "application/json", `{"message":"Hello","language":"fr"}`, nil, nil, ErrInvalidResponse, 0},
		{"oversized", 200, "application/json", strings.Repeat("x", maxResponseBytes+1), nil, nil, ErrResponseTooLarge, 0},
		{"read and close errors", 200, "application/json", success, readFailure, closeFailure, readFailure, 0},
		{"close error after success", 200, "application/json", success, nil, closeFailure, closeFailure, 0},
		{"JSON status error", 400, "application/json", `{"error":"invalid name"}`, nil, nil, nil, 400},
		{"plain status error", 404, "text/plain", "not found", nil, nil, nil, 404},
		{"status and size errors", 500, "text/html", strings.Repeat("x", maxResponseBytes+1), nil, nil, ErrResponseTooLarge, 500},
		{"status read and close errors", 503, "text/plain", "partial", readFailure, closeFailure, readFailure, 503},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reader io.Reader = strings.NewReader(tt.data)
			if tt.readErr != nil {
				reader = io.MultiReader(reader, iotest.ErrReader(tt.readErr))
			}
			body := &observedBody{Reader: reader, closeErr: tt.closeErr}
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Header.Get("Accept") != "application/json" {
					t.Error("client did not request JSON")
				}
				return &http.Response{StatusCode: tt.status, Header: http.Header{"Content-Type": {tt.contentType}}, Body: body}, nil
			})}
			request, err := http.NewRequestWithContext(context.Background(), "GET", "http://example.test/greeting", nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := doGreeting(client, request)
			if body.closes != 1 {
				t.Fatalf("response body closed %d times, want once", body.closes)
			}
			if tt.wantErr != nil || tt.wantStatus != 0 {
				if err == nil || got != (Greeting{}) {
					t.Fatalf("failed response exposed a greeting: %+v, %v", got, err)
				}
				if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
					t.Fatalf("lost cause %v in %v", tt.wantErr, err)
				}
				if tt.closeErr != nil && !errors.Is(err, tt.closeErr) {
					t.Fatalf("lost close cause in %v", err)
				}
				if tt.wantStatus != 0 {
					var statusErr *HTTPStatusError
					if !errors.As(err, &statusErr) || statusErr.StatusCode != tt.wantStatus {
						t.Fatalf("lost HTTP status %d in %v", tt.wantStatus, err)
					}
					if tt.name == "JSON status error" && statusErr.Message != "invalid name" {
						t.Fatalf("lost problem body: %v", statusErr)
					}
				}
			} else if err != nil || got != (Greeting{Message: "Hello, Ana!", Language: "en"}) {
				t.Fatalf("greeting=%+v err=%v", got, err)
			}
		})
	}
}

func TestClientStopsReadingAnOversizedResponseAndClosesIt(t *testing.T) {
	source := strings.NewReader(strings.Repeat("x", maxResponseBytes*2))
	body := &observedBody{Reader: source}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: body}, nil
	})}
	_, err := getGreeting(context.Background(), client, "http://example.test", "Ana", "en")
	if !errors.Is(err, ErrResponseTooLarge) || body.closes != 1 || source.Len() != maxResponseBytes-1 {
		t.Fatalf("expected bounded probe and closure: unread=%d closes=%d err=%v", source.Len(), body.closes, err)
	}
}

func TestClientWrapsTransportCancellation(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.Canceled
	})}
	got, err := getGreeting(context.Background(), client, "http://example.test", "Ana", "en")
	if got != (Greeting{}) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %+v, %v", got, err)
	}
}

type canceledReadBody struct{ ctx context.Context }

func (body canceledReadBody) Read([]byte) (int, error) {
	<-body.ctx.Done()
	return 0, body.ctx.Err()
}

func (body canceledReadBody) Close() error { return nil }

func TestClientTimeoutIncludesWaitingForHeadersAndReadingTheBody(t *testing.T) {
	for _, phase := range []string{"headers", "body"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// The fake transport obeys request cancellation. Network I/O is
				// excluded from this bubble, so the clock can advance deterministically.
				client := &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					if phase == "headers" {
						<-request.Context().Done()
						return nil, request.Context().Err()
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: canceledReadBody{request.Context()}}, nil
				})}
				started := time.Now()
				_, err := getGreeting(context.Background(), client, "http://example.test", "Ana", "en")
				var timeoutErr net.Error
				if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &timeoutErr) || !timeoutErr.Timeout() {
					t.Fatalf("want detectable deadline/timeout, got %v", err)
				}
				if elapsed := time.Since(started); elapsed != time.Second {
					t.Fatalf("virtual elapsed time=%v, want 1s", elapsed)
				}
			})
		})
	}
}
