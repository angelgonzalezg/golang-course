package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

func withRequestMetadata(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if id := strings.TrimSpace(r.Header.Get("X-Request-ID")); id != "" {
			// Values are local to this context tree. Reading the header explicitly
			// establishes metadata at the incoming HTTP boundary.
			ctx = withRequestID(ctx, id)
		}
		// WithContext makes a shallow request copy; it does not mutate r's context.
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newReportHandler(service reportService, budget time.Duration, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /reports/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		requestLogger := logger
		if id, ok := requestIDFrom(ctx); ok {
			w.Header().Set("X-Request-ID", id)
			// slog does not extract values automatically; enrich its dependency.
			requestLogger = logger.With(slog.String("request_id", id))
		}
		report, err := service.Generate(ctx, r.PathValue("id"), budget)
		if err != nil {
			switch {
			case errors.Is(err, context.DeadlineExceeded):
				// This handler waits on local work. Use the same 503 policy as the
				// REST lesson; 504 describes a gateway waiting for an upstream server.
				http.Error(w, "report time budget exhausted", http.StatusServiceUnavailable)
			case errors.Is(err, context.Canceled):
				// This is the example's cancellation response policy. A client that
				// disconnected may no longer be able to receive any response.
				http.Error(w, "report request canceled", http.StatusServiceUnavailable)
			case errors.Is(err, ErrReportNotFound):
				http.NotFound(w, r)
			default:
				http.Error(w, "could not generate report", http.StatusInternalServerError)
			}
			requestLogger.InfoContext(ctx, "report request finished", slog.Any("error", err))
			return
		}
		data, err := json.Marshal(&report)
		if err != nil {
			http.Error(w, "could not encode report", http.StatusInternalServerError)
			requestLogger.ErrorContext(ctx, "encode report response", slog.Any("error", err))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(append(data, '\n')); err != nil {
			requestLogger.ErrorContext(ctx, "write report response", slog.Any("error", err))
		}
	})
	return withRequestMetadata(mux)
}
