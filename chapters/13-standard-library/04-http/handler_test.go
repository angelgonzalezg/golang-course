package main

import (
	json "encoding/json/v2"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
)

func TestMakeGreetingAppliesDomainRules(t *testing.T) {
	tests := []struct {
		input greetingRequest
		want  Greeting
		err   error
	}{
		{greetingRequest{Name: "  Ana  "}, Greeting{Message: "Hello, Ana!", Language: "en"}, nil},
		{greetingRequest{Name: "☀", Language: " es "}, Greeting{Message: "Hola, ☀!", Language: "es"}, nil},
		{greetingRequest{Name: " "}, Greeting{}, ErrInvalidName},
		{greetingRequest{Name: "\xff"}, Greeting{}, ErrInvalidName},
		{greetingRequest{Name: strings.Repeat("☀", 81)}, Greeting{}, ErrInvalidName},
		{greetingRequest{Name: "Ana", Language: "fr"}, Greeting{}, ErrUnsupportedLanguage},
	}
	for _, tt := range tests {
		got, err := makeGreeting(tt.input)
		if got != tt.want || !errors.Is(err, tt.err) {
			t.Fatalf("input %+v: got (%+v, %v), want (%+v, %v)", tt.input, got, err, tt.want, tt.err)
		}
	}
	if _, err := makeGreeting(greetingRequest{Name: strings.Repeat("☀", 80)}); err != nil {
		t.Fatalf("limit counts runes rather than UTF-8 bytes: %v", err)
	}
}

func TestHandlerRoutesAndInputFailures(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		body        string
		status      int
		message     string
	}{
		{"health", "GET", "/health", "", "", 200, ""},
		{"GET query escaping", "GET", "/greetings/es?name=Ana+%26+Go", "", "", 200, "Hola, Ana & Go!"},
		{"missing name", "GET", "/greetings/en", "", "", 400, ""},
		{"unsupported language", "GET", "/greetings/fr?name=Ana", "", "", 400, ""},
		{"POST default", "POST", "/greetings", "application/json", `{"name":" Ana "}`, 200, "Hello, Ana!"},
		{"POST parameters", "POST", "/greetings", "application/json; charset=utf-8", `{"name":"☀","language":"es"}`, 200, "Hola, ☀!"},
		{"no media type", "POST", "/greetings", "", `{"name":"Ana"}`, 415, ""},
		{"unsupported media type", "POST", "/greetings", "text/plain", `{"name":"Ana"}`, 415, ""},
		{"malformed media type", "POST", "/greetings", "application/json; bad", `{"name":"Ana"}`, 415, ""},
		{"null", "POST", "/greetings", "application/json", `null`, 400, ""},
		{"empty", "POST", "/greetings", "application/json", "", 400, ""},
		{"array", "POST", "/greetings", "application/json", `[]`, 400, ""},
		{"blank name", "POST", "/greetings", "application/json", `{"name":" "}`, 400, ""},
		{"wrong type", "POST", "/greetings", "application/json", `{"name":42}`, 400, ""},
		{"unknown member", "POST", "/greetings", "application/json", `{"name":"Ana","extra":true}`, 400, ""},
		{"wrong case", "POST", "/greetings", "application/json", `{"Name":"Ana"}`, 400, ""},
		{"duplicate", "POST", "/greetings", "application/json", `{"name":"Ana","name":"Ben"}`, 400, ""},
		{"trailing value", "POST", "/greetings", "application/json", `{"name":"Ana"} {}`, 400, ""},
		{"unknown route", "GET", "/missing", "", "", 404, ""},
		{"method mismatch", "PUT", "/health", "", "", 405, ""},
	}
	handler := newHandler()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", tt.contentType)
			defer request.Body.Close() // Direct handler tests own their constructed bodies.
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != tt.status || recorder.Header().Get("X-Course") != "learning-go" {
				t.Fatalf("status=%d headers=%v body=%s", recorder.Code, recorder.Header(), recorder.Body.String())
			}
			if tt.status == 404 || tt.status == 405 {
				if tt.status == 405 && recorder.Header().Get("Allow") != "GET, HEAD" {
					t.Fatalf("missing method information: %v", recorder.Header())
				}
				return // Native mux errors use plain text, rather than the application's JSON.
			}
			if recorder.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected representation: %v", recorder.Header())
			}
			if tt.message != "" {
				var greeting Greeting
				if err := json.Unmarshal(recorder.Body.Bytes(), &greeting); err != nil || greeting.Message != tt.message {
					t.Fatalf("greeting=%+v err=%v", greeting, err)
				}
			} else if tt.status >= 400 {
				var body problem
				if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Error == "" {
					t.Fatalf("problem=%+v err=%v", body, err)
				}
			}
		})
	}
}

func TestHandlerLimitsBodyBytesWithoutContentLength(t *testing.T) {
	const document = `{"name":"☀"}`
	exactFit := document + strings.Repeat(" ", maxRequestBytes-len(document))
	for _, tt := range []struct {
		body   string
		status int
	}{{exactFit, 200}, {exactFit + " ", 413}} {
		request := httptest.NewRequest("POST", "/greetings", strings.NewReader(tt.body))
		request.ContentLength = -1
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		newHandler().ServeHTTP(recorder, request)
		if err := request.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != tt.status {
			t.Fatalf("%d bytes: status=%d, want %d", len(tt.body), recorder.Code, tt.status)
		}
	}
}

func TestWriteJSONEncodesBeforeCommittingSuccess(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeJSON(recorder, http.StatusCreated, math.NaN())
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "NaN") {
		t.Fatalf("serialization failure committed an invalid response: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerMapsRequestReadFailuresToBadRequest(t *testing.T) {
	request := httptest.NewRequest("POST", "/greetings", iotest.ErrReader(errors.New("input failed")))
	request.Header.Set("Content-Type", "application/json")
	defer request.Body.Close()
	recorder := httptest.NewRecorder()
	newHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("read failure status=%d, want 400", recorder.Code)
	}
}
