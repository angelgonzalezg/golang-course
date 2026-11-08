package labelhttp_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"golangcourse/chapters/15-testing/03-http-tests"
)

func TestHandler(t *testing.T) {
	for _, tt := range []struct {
		name   string
		method string
		target string
		status int
		body   string
	}{
		{name: "valid", method: http.MethodGet, target: "/label?value=+GO+", status: 200, body: "go\n"},
		{name: "missing value", method: http.MethodGet, target: "/label", status: 400, body: "invalid label\n"},
		{name: "wrong method", method: http.MethodPost, target: "/label?value=go", status: 405},
		{name: "unknown route", method: http.MethodGet, target: "/missing", status: 404},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequestWithContext(t.Context(), tt.method, tt.target, nil)
			recorder := httptest.NewRecorder()
			labelhttp.NewHandler().ServeHTTP(recorder, request)

			// Result snapshots the response headers as the handler committed them.
			response := recorder.Result()
			t.Cleanup(func() { response.Body.Close() })
			if response.StatusCode != tt.status {
				t.Fatalf("status=%d, want %d", response.StatusCode, tt.status)
			}
			if tt.body != "" {
				body, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if string(body) != tt.body {
					t.Errorf("body=%q, want %q", body, tt.body)
				}
				if got := response.Header.Get("Content-Type"); got != "text/plain; charset=utf-8" {
					t.Errorf("Content-Type=%q", got)
				}
			}
			if tt.status == http.StatusMethodNotAllowed && response.Header.Get("Allow") != "GET, HEAD" {
				t.Errorf("Allow=%q, want GET, HEAD", response.Header.Get("Allow"))
			}
		})
	}
}

func TestHandlerOverHTTP(t *testing.T) {
	server := httptest.NewServer(labelhttp.NewHandler())
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 2 * time.Second // A failure bound, not a synchronization mechanism.
	query := url.Values{"value": {" NIÑO & GO "}}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/label?"+query.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	// Cleanup runs last-registered first: close the body before closing the server.
	t.Cleanup(func() { response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "niño & go\n" {
		t.Fatalf("response=(%d, %q), want (200, %q)", response.StatusCode, body, "niño & go\n")
	}
}

func TestHeadOverHTTP(t *testing.T) {
	// A real server suppresses the HEAD body; a recorder only records handler writes.
	server := httptest.NewServer(labelhttp.NewHandler())
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 2 * time.Second
	request, err := http.NewRequestWithContext(t.Context(), http.MethodHead, server.URL+"/label?value=go", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || len(body) != 0 {
		t.Fatalf("HEAD response=(%d, %q), want (200, empty body)", response.StatusCode, body)
	}
	if got := response.Header.Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type=%q", got)
	}
}
