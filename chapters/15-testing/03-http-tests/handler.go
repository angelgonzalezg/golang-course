// Package labelhttp exposes label normalization through a small HTTP handler.
package labelhttp

import (
	"fmt"
	"net/http"

	"golangcourse/chapters/15-testing/01-unit-tests"
)

// NewHandler builds an independent mux, avoiding shared default-mux state.
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /label", func(w http.ResponseWriter, r *http.Request) {
		label, err := labels.Normalize(r.URL.Query().Get("value"))
		if err != nil {
			http.Error(w, "invalid label", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, label)
	})
	return mux
}
