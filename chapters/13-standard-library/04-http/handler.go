package main

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"
)

const maxRequestBytes = 4 * 1024

type greetingRequest struct {
	Name     string `json:"name"`
	Language string `json:"language,omitempty"`
}

type Greeting struct {
	Message  string `json:"message"`
	Language string `json:"language"`
}

type problem struct {
	Error string `json:"error"`
}

var (
	ErrInvalidName         = errors.New("name must contain 1 to 80 valid UTF-8 runes after trimming")
	ErrUnsupportedLanguage = errors.New("language must be en or es")
)

// makeGreeting is domain logic independent of HTTP. Its value-only inputs and
// output need no shared mutable state, so handlers can call it concurrently.
func makeGreeting(input greetingRequest) (Greeting, error) {
	name := strings.TrimSpace(input.Name)
	if !utf8.ValidString(name) || name == "" || utf8.RuneCountInString(name) > 80 {
		return Greeting{}, ErrInvalidName
	}
	language := strings.TrimSpace(input.Language)
	if language == "" {
		language = "en"
	}
	switch language {
	case "en":
		return Greeting{Message: "Hello, " + name + "!", Language: language}, nil
	case "es":
		return Greeting{Message: "Hola, " + name + "!", Language: language}, nil
	default:
		return Greeting{}, ErrUnsupportedLanguage
	}
}

func newHandler() http.Handler {
	// A private mux avoids registering routes in process-global DefaultServeMux.
	// Method patterns and named wildcards use the routing added in Go 1.22.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{Status: "ok"})
	})
	mux.HandleFunc("GET /greetings/{language}", getGreetingHandler)
	mux.HandleFunc("POST /greetings", postGreetingHandler)
	return withCourseHeader(mux)
}

// Middleware composes handlers; set headers before the next handler writes.
// Its local work finishes within ServeHTTP, without a background goroutine.
func withCourseHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Course", "learning-go")
		next.ServeHTTP(w, r)
	})
}

func getGreetingHandler(w http.ResponseWriter, r *http.Request) {
	// PathValue reads a named route segment; Query reads query parameters.
	// This API uses the first name value if the query repeats that member.
	input := greetingRequest{Name: r.URL.Query().Get("name"), Language: r.PathValue("language")}
	greeting, err := makeGreeting(input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, problem{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, &greeting)
}

func postGreetingHandler(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, problem{Error: "Content-Type must be application/json"})
		return
	}

	// MaxBytesReader enforces an HTTP body limit even without Content-Length.
	// The HTTP server owns closing request bodies when the handler returns.
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var limitErr *http.MaxBytesError
		if errors.As(err, &limitErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, problem{Error: "request body exceeds 4096 bytes"})
		} else {
			writeJSON(w, http.StatusBadRequest, problem{Error: "could not read request body"})
		}
		return
	}
	// Use fresh storage, require one non-null object, and reject unknown names.
	// v2 also rejects duplicate names and invalid UTF-8 by default.
	var input *greetingRequest
	if err := json.Unmarshal(data, &input, json.RejectUnknownMembers(true)); err != nil || input == nil {
		writeJSON(w, http.StatusBadRequest, problem{Error: "body must be one valid greeting request object"})
		return
	}
	greeting, err := makeGreeting(*input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, problem{Error: err.Error()})
		return
	}
	// This operation computes a greeting; it does not persist a new resource.
	writeJSON(w, http.StatusOK, &greeting)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	// Encode before committing headers so serialization failures can still be
	// reported as 500, rather than producing a partially encoded success body.
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(append(data, '\n')); err != nil {
		// The status is already committed. Record the failure instead of trying
		// to send another response. Structured logging gets its own next lesson.
		log.Print(fmt.Errorf("write JSON response: %w", err))
	}
}
