package otelx

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// responseWriter is a thin wrapper around http.ResponseWriter that captures
// the final HTTP status code written by the handler.
//
// The standard ResponseWriter does not expose the status code after the
// handler executes, so middleware often wraps it to record metrics such as
// request count and duration by status class (2xx, 4xx, 5xx).
//
// By default, the status code is initialized to http.StatusOK until WriteHeader
// is explicitly called by the handler.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// NewResponseWriter creates a new responseWriter that wraps an existing
// http.ResponseWriter while defaulting the status code to http.StatusOK.
//
// This helper is typically used in HTTP middleware to capture both the
// handler response and any modifications to the HTTP status code.
func NewResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

// WriteHeader updates the tracked status code and forwards the call
// to the underlying ResponseWriter.
//
// Handlers may call WriteHeader multiple times, but only the first call
// sets the actual HTTP response status. This wrapper preserves the semantics
// by only storing the first status used.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Status returns the final HTTP status code written for the request.
//
// It is used after the wrapped handler finishes so middleware can record
// metrics per status code (e.g., 200, 404, 500).
func (rw *responseWriter) Status() int {
	return rw.statusCode
}

// MetricsMiddleware instruments every HTTP request with OpenTelemetry
// metrics using the global otelx.Metrics instance.
//
// It records two metrics per request:
//
//  1. http_requests_total (counter)
//     - method        (e.g., GET, POST)
//     - path          (full URL path)
//     - status_code   (integer)
//
//  2. http_request_duration_seconds (histogram)
//     - same attributes as above
//
// This middleware must be registered *after* calling
// NewMeterProvider(), otherwise the metrics instruments
// will not be initialized.
//
// Example:
//
//	mux := http.NewServeMux()
//	mux.Handle("/api", handler)
//
//	http.ListenAndServe(":8080",
//	    otelx.MetricsMiddleware(mux),
//	)
//
// Each request is timed precisely and attributes are attached via
// metric.WithAttributes, matching OTEL best practices for HTTP server metrics.
func MetricsMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		rw := NewResponseWriter(w)
		start := time.Now()

		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()

		// Record metrics correctly using metric.WithAttributes
		metrics.RequestCounter.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("method", r.Method),
				attribute.String("path", r.URL.Path),
				attribute.Int("status_code", rw.Status()),
			),
		)

		metrics.RequestHistogram.Record(ctx, duration,
			metric.WithAttributes(
				attribute.String("method", r.Method),
				attribute.String("path", r.URL.Path),
				attribute.Int("status_code", rw.Status()),
			),
		)
	}

	return http.HandlerFunc(fn)
}
