package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestHandlerPropagatesRequestMetadataWithoutMutatingTheOriginal(t *testing.T) {
	var receivedID string
	service := reportService{store: reportStoreFunc(func(ctx context.Context, id string) (Report, error) {
		receivedID, _ = requestIDFrom(ctx)
		return Report{ID: id, Title: "Quarterly sales"}, nil
	})}
	request := httptest.NewRequest("GET", "/reports/sales", nil)
	request.Header.Set("X-Request-ID", "  req-http  ")
	recorder := httptest.NewRecorder()
	newReportHandler(service, time.Second, quietLogger()).ServeHTTP(recorder, request)
	var report Report
	if err := json.Unmarshal(recorder.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || report.ID != "sales" || receivedID != "req-http" || recorder.Header().Get("X-Request-ID") != "req-http" {
		t.Fatalf("request propagation failed: report=%+v storeID=%q headers=%v", report, receivedID, recorder.Header())
	}
	if _, ok := requestIDFrom(request.Context()); ok {
		t.Fatal("middleware changed the original request's context")
	}
}

func TestHandlerMapsDeadlinesCancellationAndMissingReports(t *testing.T) {
	for _, tt := range []struct {
		name   string
		ready  bool
		cancel bool
		id     string
		status int
	}{
		{"deadline", false, false, "sales", 504},
		{"canceled", false, true, "sales", 503},
		{"missing", true, false, "missing", 404},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				gate := make(chan struct{})
				if tt.ready {
					close(gate)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if tt.cancel {
					cancel()
				}
				request := httptest.NewRequest("GET", "/reports/"+tt.id, nil).WithContext(ctx)
				recorder := httptest.NewRecorder()
				service := reportService{store: gatedReports{ready: gate}}
				newReportHandler(service, 2*time.Second, quietLogger()).ServeHTTP(recorder, request)
				if recorder.Code != tt.status {
					t.Fatalf("status=%d want=%d body=%s", recorder.Code, tt.status, recorder.Body.String())
				}
			})
		})
	}
}

func receiveContextTest[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	watchdog := time.NewTimer(5 * time.Second)
	defer watchdog.Stop()
	select {
	case value := <-result:
		return value
	case <-watchdog.C:
		t.Fatal("context test coordination did not complete")
		var zero T
		return zero
	}
}

func TestClientCancellationReachesTheServerDependency(t *testing.T) {
	entered := make(chan context.Context, 1)
	storeFinished := make(chan struct{})
	forceCleanup := make(chan struct{})
	service := reportService{store: reportStoreFunc(func(ctx context.Context, id string) (Report, error) {
		defer close(storeFinished)
		entered <- ctx
		select {
		case <-ctx.Done():
			return Report{}, ctx.Err()
		case <-forceCleanup:
			// Test cleanup can release a broken implementation without waiting
			// for the intentionally long service budget.
			return Report{}, ErrReportNotFound
		}
	})}
	handler := newReportHandler(service, time.Hour, quietLogger())
	handlerFinished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerFinished)
		handler.ServeHTTP(w, r)
	}))
	client := server.Client()
	client.Timeout = 5 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/reports/sales", nil)
	if err != nil {
		cancel()
		server.Close()
		t.Fatal(err)
	}
	request.Header.Set("X-Request-ID", "req-wire")
	requestFinished := make(chan struct{})
	result := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		close(forceCleanup)
		server.Close()
		client.CloseIdleConnections()
		receiveContextTest(t, requestFinished)
	})
	go func() {
		defer close(requestFinished)
		response, err := client.Do(request)
		if response != nil {
			err = errors.Join(err, response.Body.Close())
		}
		result <- err
	}()
	workCtx := receiveContextTest(t, entered)
	if id, ok := requestIDFrom(workCtx); !ok || id != "req-wire" {
		t.Fatalf("header metadata did not reach the dependency: %q", id)
	}
	cancel()
	if err := receiveContextTest(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("client cancellation was lost: %v", err)
	}
	receiveContextTest(t, storeFinished)
	receiveContextTest(t, handlerFinished)
	if !errors.Is(workCtx.Err(), context.Canceled) {
		t.Fatalf("dependency did not observe cancellation: %v", workCtx.Err())
	}
}
