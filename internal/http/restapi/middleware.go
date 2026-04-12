package restapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter

	status int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func loggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			start := time.Now()
			ctx := request.Context()

			rec := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
			next.ServeHTTP(rec, request)

			attrs := []any{
				"method", request.Method,
				"path", request.URL.Path,
				"status", rec.status,
				"duration", time.Since(start),
			}

			if rec.status >= http.StatusInternalServerError {
				logger.ErrorContext(ctx, "request failed", attrs...)

				return
			}

			logger.InfoContext(ctx, "request complete", attrs...)
		})
	}
}

func healthcheckMiddleware(healthChecker func(context.Context) error, logger *slog.Logger) func(http.Handler) http.Handler {
	checker := healthChecker
	if checker == nil {
		checker = func(context.Context) error { return nil }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/healthz" && request.Method == http.MethodGet {
				ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
				defer cancel()

				statusCode := http.StatusOK
				status := "ok"
				if err := checker(ctx); err != nil {
					statusCode = http.StatusServiceUnavailable
					status = "degraded"
					logger.ErrorContext(ctx, "health check failed", "error", err)
				}

				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(statusCode)
				_ = json.NewEncoder(writer).Encode(map[string]string{"status": status})

				return
			}

			next.ServeHTTP(writer, request)
		})
	}
}
