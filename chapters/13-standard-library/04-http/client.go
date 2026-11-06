package main

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 4 * 1024

var (
	ErrResponseTooLarge = errors.New("response exceeds the 4096-byte limit")
	ErrInvalidResponse  = errors.New("invalid greeting response")
)

type HTTPStatusError struct {
	StatusCode int
	Message    string
}

func (err *HTTPStatusError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", err.StatusCode, err.Message)
}

func newClient() *http.Client {
	// Clone instead of mutating the shared default transport. Reuse this client
	// and its connection pool across requests, including concurrent requests.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 2 * time.Second
	transport.ResponseHeaderTimeout = 2 * time.Second
	transport.IdleConnTimeout = 30 * time.Second
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}
}

func getGreeting(ctx context.Context, client *http.Client, baseURL, name, language string) (Greeting, error) {
	if language == "" {
		language = "en"
	}
	query := url.Values{}
	query.Set("name", name)
	// Escape a path segment and encode query values with their separate rules.
	endpoint := strings.TrimRight(baseURL, "/") + "/greetings/" + url.PathEscape(language) + "?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Greeting{}, fmt.Errorf("create GET request: %w", err)
	}
	return doGreeting(client, request)
}

func postGreeting(ctx context.Context, client *http.Client, baseURL string, input greetingRequest) (Greeting, error) {
	data, err := json.Marshal(&input)
	if err != nil {
		return Greeting{}, fmt.Errorf("encode POST request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/greetings", bytes.NewReader(data))
	if err != nil {
		return Greeting{}, fmt.Errorf("create POST request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	return doGreeting(client, request)
}

func isJSON(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "application/json"
}

// doGreeting borrows the client, uses the request once, and owns the returned
// response body. Every failure returns a zero Greeting, including close failures.
func doGreeting(client *http.Client, request *http.Request) (result Greeting, err error) {
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		// A non-2xx status is not a Do error. This path is for request/transport
		// errors, including cancellation; wrapping preserves errors.Is/As.
		return Greeting{}, fmt.Errorf("send greeting request: %w", err)
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			result = Greeting{}
			err = errors.Join(err, fmt.Errorf("close response body: %w", closeErr))
		}
	}()

	var statusErr *HTTPStatusError
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		statusErr = &HTTPStatusError{StatusCode: response.StatusCode, Message: http.StatusText(response.StatusCode)}
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if readErr != nil {
		err = fmt.Errorf("read response body: %w", readErr)
	} else if len(data) > maxResponseBytes {
		err = ErrResponseTooLarge
	}
	if err != nil {
		if statusErr != nil {
			err = errors.Join(statusErr, err)
		}
		return Greeting{}, err
	}
	if statusErr != nil {
		// Routing and proxies may return plain text/HTML. Preserve the status
		// even when no compatible JSON problem body is available.
		var body problem
		if isJSON(response.Header.Get("Content-Type")) && json.Unmarshal(data, &body) == nil && body.Error != "" {
			statusErr.Message = body.Error
		}
		return Greeting{}, statusErr
	}
	if !isJSON(response.Header.Get("Content-Type")) {
		return Greeting{}, fmt.Errorf("%w: expected application/json", ErrInvalidResponse)
	}
	// Allow additive response fields for compatibility with a newer server.
	if err := json.Unmarshal(data, &result); err != nil {
		return Greeting{}, fmt.Errorf("%w: decode JSON: %w", ErrInvalidResponse, err)
	}
	if result.Message == "" || (result.Language != "en" && result.Language != "es") {
		return Greeting{}, fmt.Errorf("%w: missing message or unsupported language", ErrInvalidResponse)
	}
	// Reading a small body to EOF and closing it permits connection reuse.
	// Oversized responses are closed without unbounded draining.
	return result, nil
}
