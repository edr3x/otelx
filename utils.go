package otelx

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// IsEnabled reports whether OpenTelemetry instrumentation should be activated.
//
// The function returns true only when:
//  1. The environment variable OTEL_ENABLE is set to "true", and
//  2. The environment variable OTEL_COLLECTOR_ENDPOINT is defined.
//
// This ensures that OTEL instrumentation is enabled explicitly (via OTEL_ENABLE)
// and that a valid collector endpoint is available before attempting to export
// any telemetry data.
//
// Typical usage:
//
//	if otelx.IsEnabled() {
//	    // initialize tracing and metrics
//	}
//
// This prevents silent misconfiguration where OTEL is enabled but no collector
// endpoint is provided.
func IsEnabled() bool {
	_, ok := os.LookupEnv("OTEL_COLLECTOR_ENDPOINT")
	return os.Getenv("OTEL_ENABLE") == "true" && ok
}

// StartSpan creates a new span using the globally registered tracer.
//
// Features:
//   - Automatically generates the span name using caller function name + line
//   - Gracefully falls back to a No-Op tracer if telemetry is disabled
//   - Accepts optional trace.SpanStartOption arguments
//
// Example:
//
//	ctx, span := otelx.StartSpan(ctx)
//	defer span.End()
//
// This helper is designed for internal code paths where manually naming each
// span would be verbose. For API-level or logical spans, prefer explicit names:
//
//	ctx, span := tracer.Start(ctx, "database.query")
func StartSpan(ctx context.Context, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if tracer == nil || grpcConnection == nil {
		// Use the noop tracer provider
		noopTracer := noop.NewTracerProvider().Tracer("noop")
		return noopTracer.Start(ctx, "noop", opts...)
	}

	// Extract caller info for auto-span naming.
	pc, _, line, _ := runtime.Caller(1)
	fn := runtime.FuncForPC(pc)

	return tracer.Start(ctx, fmt.Sprintf("%s:%d", fn.Name(), line), opts...)
}

// TraceIDHeader is a middleware that adds the trace ID to response headers
// for correlation with distributed tracing systems.
//
// The header "X-Trace-ID" will contain the trace ID if tracing is active.
func TraceIDHeader(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		span := trace.SpanFromContext(r.Context())
		if span.SpanContext().HasTraceID() {
			w.Header().Set("X-Trace-ID", span.SpanContext().TraceID().String())
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

// RecordError records an error on the current span from context.
// use this where the error are ignored with returning default value
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// AddAttribute adds a custom attribute to the current span from context.
func AddAttribute(ctx context.Context, key string, value any) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	switch v := value.(type) {
	case string:
		span.SetAttributes(attribute.String(key, v))
	case int:
		span.SetAttributes(attribute.Int(key, v))
	case int64:
		span.SetAttributes(attribute.Int64(key, v))
	case float64:
		span.SetAttributes(attribute.Float64(key, v))
	case bool:
		span.SetAttributes(attribute.Bool(key, v))
	default:
		span.SetAttributes(attribute.String(key, strconv.FormatInt(int64(0), 10)))
	}
}
