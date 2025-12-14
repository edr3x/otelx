package otelx

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	metrics        Metrics
	tracer         trace.Tracer
	grpcConnection *grpc.ClientConn
)

// Metrics holds pre-initialized OpenTelemetry instruments for recording
// HTTP request metrics.
//
// These instruments are created when NewMeterProvider() is called.
// Typical usage:
//
//	metrics.RequestCounter.Add(ctx, 1, attribute.String("route", "/login"))
//	metrics.RequestHistogram.Record(ctx, duration.Seconds())
//
// All services share the same meter provider unless explicitly overridden.
type Metrics struct {
	RequestCounter   api.Int64Counter
	RequestHistogram api.Float64Histogram
}

// initCollector establishes a shared gRPC connection to the OpenTelemetry
// Collector. It is called internally by both NewTraceProvider and
// NewMeterProvider.
//
// Behavior:
//   - Respects OTEL_ENABLE=false (returns a known error instead of connecting)
//   - Requires OTEL_COLLECTOR_ENDPOINT to be set (host:port)
//   - Returns the existing cached connection if already initialized
//
// This function should not be used directly by applications.
func initCollector() (*grpc.ClientConn, error) {
	if grpcConnection != nil {
		return grpcConnection, nil
	}

	// Tracing disabled via environment flag.
	if strings.ToLower(os.Getenv("OTEL_ENABLE")) != "true" {
		return nil, errors.New("tracing disabled via OTEL_ENABLE=false")
	}

	otlpEndpoint := os.Getenv("OTEL_COLLECTOR_ENDPOINT")
	if otlpEndpoint == "" {
		return nil, errors.New("OTEL_COLLECTOR_ENDPOINT not set")
	}

	// It connects the OpenTelemetry Collector through local gRPC connection.
	conn, err := grpc.NewClient(otlpEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection to collector: %w", err)
	}

	fmt.Println("grpc connection success")

	grpcConnection = conn
	return conn, nil
}

// newResource builds a standard OpenTelemetry Resource describing the service.
// It collects:
//
//   - service.name      (explicit argument)
//   - service.version   from $SERVICE_VERSION
//   - deployment.environment from $ENV
//   - host.*            automatically via resource.WithHost()
//
// These attributes help Tempo/Jaeger/Grafana correctly group and filter spans.
//
// This function is used internally by NewTraceProvider() and NewMeterProvider().
func newResource(ctx context.Context, service string) (*resource.Resource, error) {
	return resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(service),
			semconv.ServiceVersionKey.String(os.Getenv("SERVICE_VERSION")),
			semconv.DeploymentEnvironmentKey.String(os.Getenv("ENV")),
		),
		resource.WithHost(), // automatically adds host.id, host.name
	)
}

// NewTraceProvider configures and registers a global OpenTelemetry
// TracerProvider for the calling service.
//
// It automatically:
//
//  1. Connects to the OTEL Collector via gRPC
//  2. Creates an OTLP trace exporter
//  3. Attaches a BatchSpanProcessor for efficient export
//  4. Builds a Resource containing service metadata
//  5. Sets W3C TraceContext + Baggage as global propagators
//  6. Exposes a package-level tracer used by StartSpan()
//
// If OTEL_ENABLE=false or the connection fails, a No-Op tracer is installed
// so the application continues functioning without telemetry.
//
// Return values:
//   - *sdktrace.TracerProvider  The initialized provider (or nil on failure)
//   - cleanup()                 Flushes spans and shuts down the provider
//
// Example:
//
//	tp, cleanup := otelx.NewTraceProvider(ctx, "auth-service")
//	defer cleanup()
//
// Traces created through StartSpan() or otel.Tracer() will automatically be sent
// to the collector if telemetry is enabled.
func NewTraceProvider(ctx context.Context, service string) (*sdktrace.TracerProvider, func()) {
	clean := func() {}
	conn, err := initCollector()
	if err != nil {
		log.Printf("failed to grpc connection: %v\n", err)
		tp := noop.NewTracerProvider()
		tracer = tp.Tracer("noop")
		return nil, clean
	}

	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		log.Printf("failed to create exporter: %v\n", err)
		return nil, clean
	}

	// Define standard resource attributes used by all traces.
	res, err := newResource(ctx, service)
	if err != nil {
		log.Printf("failed to create resource: %v\n", err)
		return nil, clean
	}

	bsm := sdktrace.NewBatchSpanProcessor(traceExporter)

	// Create the tracer provider with batching exporter and resource.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsm),
	)

	// Register as global provider.
	otel.SetTracerProvider(tp)

	// Set global propagators: TraceContext + Baggage.
	// Ensures correct trace propagation across microservices.
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.Baggage{},
			propagation.TraceContext{},
		),
	)

	tracer = tp.Tracer(service)

	cleanup := func() {
		// Graceful shutdown ensures pending spans are flushed.
		if err := tp.Shutdown(ctx); err != nil {
			log.Printf("error shutting down tracer provider: %v", err)
		}
	}

	return tp, cleanup
}

// NewMeterProvider initializes the global OpenTelemetry MeterProvider and
// registers metrics instruments used across the service.
//
// Initialization steps:
//   - Connect to OTEL Collector (same rules as tracing)
//   - Create an OTLP metric exporter
//   - Attach a PeriodicReader for automatic metric export
//   - Create service-level Resource metadata
//   - Build a meter and populate the global Metrics struct
//
// The following instruments are created by default:
//
//   - http_requests_total                (counter)
//   - http_request_duration_seconds      (histogram)
//
// These match common Prometheus naming conventions.
//
// Returns a cleanup function that flushes and shuts down the provider.
func NewMeterProvider(ctx context.Context, service string) func() {
	emptyCleanup := func() {}
	conn, err := initCollector()
	if err != nil {
		log.Printf("failed to grpc connection: %v\n", err)
		return emptyCleanup
	}

	metricExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn))
	if err != nil {
		log.Printf("failed to grpc connection: %v\n", err)
		return emptyCleanup
	}

	// Define standard resource attributes used by all traces.
	res, err := newResource(ctx, service)
	if err != nil {
		log.Printf("failed to create resource: %v\n", err)
		return emptyCleanup
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	meter := mp.Meter(service)

	// Initialize metrics
	counter, err := meter.Int64Counter(
		"http_requests_total",
		api.WithDescription("Total number of HTTP requests"),
	)
	if err != nil {
		log.Printf("failed to create counter: %s\n", err.Error())
		return emptyCleanup
	}

	histogram, err := meter.Float64Histogram(
		"http_request_duration_seconds",
		api.WithDescription("HTTP request duration in seconds"),
		api.WithExplicitBucketBoundaries(
			0.1, 0.2, 0.3, 0.4, 0.5,
			0.6, 0.7, 0.8, 0.9, 1.0,
			2.0, 5.0,
		),
	)
	if err != nil {
		log.Printf("failed to create histogram: %s\n", err.Error())
		return emptyCleanup
	}

	metrics = Metrics{
		RequestCounter:   counter,
		RequestHistogram: histogram,
	}

	shutdown := func() {
		if err := mp.Shutdown(ctx); err != nil {
			log.Printf("error shutting down meter provider: %v", err)
		}
	}

	return shutdown
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
