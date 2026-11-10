package main

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/httpapi"
)

func runDemo(ctx context.Context, out io.Writer) (err error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen locally: %w", err)
	}
	server, served := startServer(listener, httpapi.NewHandler(contacts.NewStore(), nil))
	defer func() { err = errors.Join(err, shutdownServer(server, served)) }()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // Every request in this demonstration stays on loopback.
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	baseURL := "http://" + listener.Addr().String()

	// The server assigns the ID. Follow the returned Location instead of
	// guessing which ID the first POST will receive.
	created, _, err := demoRequest(ctx, client, out, baseURL, "POST", "/contacts",
		map[string]string{"name": " Ana ", "email": "ana@example.com", "phone": "555-0100"}, http.StatusCreated)
	if err != nil {
		return err
	}
	path := created.Get("Location")
	if path == "" {
		return errors.New("creation response has no Location")
	}
	for _, step := range []struct {
		method string
		path   string
		input  any
		status int
	}{
		{"GET", "/contacts", nil, http.StatusOK},
		{"GET", path, nil, http.StatusOK},
		{"POST", "/contacts", map[string]string{"name": "Ana"}, http.StatusConflict},
		// PATCH selects one field; the name and phone stay as they were.
		{"PATCH", path, map[string]string{"email": "ana.torres@example.com"}, http.StatusOK},
		{"PATCH", path, map[string]string{"email": "ana.torres@example.com"}, http.StatusOK},
		{"PATCH", path, map[string]any{"phone": nil}, http.StatusOK},
		// PUT supplies the whole editable state; omitted email and phone clear.
		{"PUT", path, map[string]string{"name": "Ana Torres"}, http.StatusOK},
		{"PUT", path, map[string]string{"name": "Ana Torres"}, http.StatusOK},
		{"DELETE", path, nil, http.StatusNoContent},
		{"DELETE", path, nil, http.StatusNotFound},
		{"GET", "/contacts", nil, http.StatusOK},
	} {
		if _, _, err := demoRequest(ctx, client, out, baseURL, step.method, step.path, step.input, step.status); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "\n== Collection queries ==")
	for _, name := range []string{"Ana López", "Bob", "Ana Ruiz"} {
		if _, _, err := demoRequest(ctx, client, out, baseURL, "POST", "/contacts", map[string]string{"name": name}, http.StatusCreated); err != nil {
			return err
		}
	}
	query := url.Values{"limit": {"1"}, "name_contains": {"ana"}, "sort": {"name"}}
	for next := "/contacts?" + query.Encode(); next != ""; {
		_, data, err := demoRequest(ctx, client, out, baseURL, "GET", next, nil, http.StatusOK)
		if err != nil {
			return err
		}
		var page httpapi.Collection
		if err := json.Unmarshal(data, &page); err != nil {
			return fmt.Errorf("decode collection page: %w", err)
		}
		// The server's link retains the filter, limit, and order for us.
		next = page.Links.Next
	}
	fmt.Fprintln(out, "\n== Conditional requests ==")
	created, _, err = demoRequest(ctx, client, out, baseURL, "POST", "/contacts", map[string]string{"name": "Conditional example"}, http.StatusCreated)
	if err != nil {
		return err
	}
	path = created.Get("Location")
	read, _, err := demoRequest(ctx, client, out, baseURL, "GET", path, nil, http.StatusOK)
	if err != nil {
		return err
	}
	tag := read.Get("ETag")
	if tag == "" {
		return errors.New("read response has no ETag")
	}
	// A stored copy is still current: 304 returns headers without a JSON body.
	if _, _, err := demoRequestWithHeaders(ctx, client, out, baseURL, "GET", path, nil, http.StatusNotModified, http.Header{"If-None-Match": {tag}}); err != nil {
		return err
	}
	updated, _, err := demoRequestWithHeaders(ctx, client, out, baseURL, "PATCH", path, map[string]string{"email": "current@example.com"}, http.StatusOK, http.Header{"If-Match": {tag}})
	if err != nil {
		return err
	}
	// Another client still holding the old validator must not overwrite this edit.
	if _, _, err := demoRequestWithHeaders(ctx, client, out, baseURL, "PATCH", path, map[string]string{"email": "stale@example.com"}, http.StatusPreconditionFailed, http.Header{"If-Match": {tag}}); err != nil {
		return err
	}
	if _, _, err := demoRequestWithHeaders(ctx, client, out, baseURL, "DELETE", path, nil, http.StatusNoContent, http.Header{"If-Match": {updated.Get("ETag")}}); err != nil {
		return err
	}
	fmt.Fprintln(out, "304 reuses a stored representation; 412 rejects an outdated write without changing data.")
	fmt.Fprintln(out, "OpenAPI contract: GET /openapi.json (also embedded in the binary).")
	fmt.Fprintln(out, "Filter first, order all matches, then select a bounded page; follow links.next.")
	fmt.Fprintln(out, "PATCH kept omitted fields; null removed an optional field.")
	fmt.Fprintln(out, "PUT replaced every editable field; omitted email and phone became empty.")
	fmt.Fprintln(out, "Empty optional fields are absent from the JSON representation.")
	fmt.Fprintln(out, "These assignment/removal patches are idempotent; PATCH in general need not be.")
	fmt.Fprintln(out, "Repeated DELETE keeps the contact absent; its status need not be identical.")
	return nil
}

func demoRequest(ctx context.Context, client *http.Client, out io.Writer, baseURL, method, path string, input any, expected int) (http.Header, []byte, error) {
	return demoRequestWithHeaders(ctx, client, out, baseURL, method, path, input, expected, nil)
}

func demoRequestWithHeaders(ctx context.Context, client *http.Client, out io.Writer, baseURL, method, path string, input any, expected int, headers http.Header) (http.Header, []byte, error) {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return nil, nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return nil, nil, err
	}
	if headers != nil {
		req.Header = headers.Clone()
	}
	if body != nil {
		mediaType := "application/json"
		if method == http.MethodPatch {
			mediaType = httpapi.MergePatchMediaType
		}
		req.Header.Set("Content-Type", mediaType)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	// Read a bounded response and close it on success and failure alike.
	const maxResponse = 64 * 1024
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err := errors.Join(readErr, resp.Body.Close()); err != nil {
		return nil, nil, err
	}
	if len(data) > maxResponse {
		return nil, nil, errors.New("demonstration response too large")
	}
	fmt.Fprintf(out, "%s %s -> %d\n", method, path, resp.StatusCode)
	if len(data) > 0 {
		fmt.Fprintln(out, string(bytes.TrimSpace(data)))
	}
	if resp.StatusCode != expected {
		return nil, nil, fmt.Errorf("%s %s: got status %d, want %d", method, path, resp.StatusCode, expected)
	}
	return resp.Header, data, nil
}
