// Package otelx provides a lightweight, opinionated wrapper around
// OpenTelemetry for Go microservices. It standardizes how services initialize
// tracing, metrics, HTTP instrumentation, and gRPC instrumentation.
//
// The package is designed for production microservice environments where
// consistency, low overhead, and automatic propagation across distributed
// systems are required.
//
// # Overview
//
// otelx provides:
//
//   - Automatic initialization of OTEL TracerProvider and MeterProvider
//   - A standard resource definition (service name, environment, version, host.id)
//   - gRPC interceptors for recording request metrics
//   - HTTP middleware that records request count & latency
//   - Outgoing HTTP client instrumentation and trace propagation
//   - Helper utilities such as StartSpan() for trace spans
//
// Tracing and metrics are fully compatible with the OpenTelemetry Collector,
// Prometheus, Tempo, Jaeger, Grafana, and any OTLP-based backend.
//
// # Environment Variables
//
// The following environment variables control runtime behavior:
//
//	OTEL_ENABLE=true|false
//	    Enables or disables tracing/metrics globally.
//	    When disabled, otelx falls back to no-op providers.
//
//	OTEL_COLLECTOR_ENDPOINT=host:port
//	    The OTLP gRPC endpoint for the OpenTelemetry Collector.
//
//	SERVICE_VERSION=string
//	    The semantic version of the service (set as a Resource attribute).
//
//	ENV=local|dev|prod
//	    Deployment environment.
//
// # Tracing
//
// Call NewTraceProvider() at service startup:
//
//	ctx := context.Background()
//	tp, cleanup := otelx.NewTraceProvider(ctx, "auth-service")
//	defer cleanup()
//
// This sets the global tracer provider and configures:
//
//   - AlwaysSample sampler
//   - BatchSpanProcessor
//   - OTLP gRPC exporter
//   - Composite propagator (W3C TraceContext + Baggage)
//
// Span creation example:
//
//	ctx, span := otelx.StartSpan(ctx)
//	defer span.End()
//
// StartSpan() automatically names spans using the caller function and line
// number, which is useful for debugging without manually naming each span.
//
// # Metrics
//
// Call NewMeterProvider() once during startup:
//
//	shutdownMetrics := otelx.NewMeterProvider(ctx, "auth-service")
//	defer shutdownMetrics()
//
// otelx initializes two standard instruments:
//
//	http_requests_total (Int64Counter)
//	http_request_duration_seconds (Float64Histogram)
//
// These metrics are used across:
//
//   - HTTP middleware
//   - Unary gRPC interceptor
//   - Stream gRPC interceptor
//
// Each metric includes consistent attributes:
//
//	method: HTTP or gRPC method
//	path: HTTP path (HTTP only)
//	status_code: integer response code
//
// # HTTP Instrumentation
//
// otelx includes:
//
//   - MetricsMiddleware: records request count & duration
//   - HTTPClient / DoRequest: propagates trace context and instruments outgoing requests
//
// Usage:
//
//	r := mux.NewRouter()
//	r.Use(otelx.MetricsMiddleware)
//
// Outgoing example:
//
//	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
//	resp, err := otelx.DoRequest(ctx, req)
//
// # gRPC Instrumentation
//
// Unary interceptor:
//
//	grpc.UnaryInterceptor(otelx.UnaryServerMetricsInterceptor())
//
// Streaming interceptor:
//
//	grpc.StreamInterceptor(otelx.StreamServerMetricsInterceptor())
//
// Both interceptors record request count and latency for:
//
//   - Unary RPCs
//   - Client-streaming RPCs
//   - Server-streaming RPCs
//   - Bidirectional streaming RPCs
//
// They attach attributes:
//
//	method: full gRPC method (/package.Service/Method)
//	status_code: gRPC status as int
//
// # Outgoing HTTP Tracing
//
// otelx.HTTPClient() wraps the default transport using otelhttp.NewTransport,
// injecting trace headers automatically:
//
//	client := otelx.HTTPClient(ctx, req)
//	client.Do(req)
//
// # Graceful Shutdown
//
// Both the TracerProvider and MeterProvider support graceful shutdown:
//
//	cleanupTrace()
//	cleanupMetrics()
//
// This ensures all pending spans and metrics are flushed before process exit.
//
// # No-op Mode
//
// If OTEL_ENABLE != "true" or the collector connection fails:
//
//   - Tracing falls back to noop.NewTracerProvider()
//   - Metrics middleware still runs but uses no-op instruments
//
// This guarantees the service never crashes due to telemetry failure.
//
// # Best Practices
//
//   - Initialize tracing & metrics once at startup
//   - Reuse a single HTTP client for outgoing calls
//   - Wrap all HTTP routes with MetricsMiddleware
//   - Register both gRPC interceptors
//   - Prefer StartSpan() in handlers for automatic naming
//
// # Summary
//
// otelx is a simple, production-ready OpenTelemetry helper for Go microservices
// that standardizes telemetry across HTTP, gRPC, and internal service-to-service
// calls. It encourages consistent metrics, reliable trace propagation, and
// minimal boilerplate while maintaining full flexibility and OTEL compliance.
package otelx
