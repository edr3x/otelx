package otelx

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// UnaryServerMetricsInterceptor returns a gRPC unary server interceptor that
// records OpenTelemetry metrics for each unary RPC call.
//
// The interceptor measures two metrics for every RPC:
//
//  1. http_requests_total (counter)
//     - method       : gRPC method name (e.g. /package.Service/Method)
//     - status_code  : gRPC status code as int
//
//  2. http_request_duration_seconds (histogram)
//     - method       : same as above
//     - status_code  : same as above
//
// This allows Prometheus, Tempo, and Grafana dashboards to provide detailed
// RPC performance metrics across all services.
//
// The interceptor must be registered after calling NewMeterProvider(), e.g.:
//
//	grpc.NewServer(
//	    grpc.UnaryInterceptor(otelx.UnaryServerMetricsInterceptor()),
//	)
//
// This interceptor is lightweight and does not affect tracing; tracing must be
// enabled separately via the OTEL trace provider.
//
// Example:
//
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(otelx.UnaryServerMetricsInterceptor()),
//	)
func UnaryServerMetricsInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()

		resp, err := handler(ctx, req) // call the actual RPC

		duration := time.Since(start).Seconds()

		code := status.Code(err)

		// Record metrics
		metrics.RequestCounter.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("method", info.FullMethod),
				attribute.Int("status_code", int(code)),
			),
		)

		metrics.RequestHistogram.Record(ctx, duration,
			metric.WithAttributes(
				attribute.String("method", info.FullMethod),
				attribute.Int("status_code", int(code)),
			),
		)

		return resp, err
	}
}

// StreamServerMetricsInterceptor returns a gRPC stream server interceptor that
// records OpenTelemetry metrics for streaming RPC calls (client-streaming,
// server-streaming, and bidirectional streams).
//
// It records the same metrics as the unary interceptor:
//
//  1. http_requests_total (counter)
//  2. http_request_duration_seconds (histogram)
//
// The attributes added are:
//   - method       : gRPC method full name (/pkg.Service/Method)
//   - status_code  : gRPC status code
//
// Stream RPCs are measured from the time the handler starts until the handler
// returns, which provides total session duration for the stream.
//
// Usage:
//
//	grpc.NewServer(
//	    grpc.StreamInterceptor(otelx.StreamServerMetricsInterceptor()),
//	)
//
// This interceptor is designed to be used with NewMeterProvider() and is fully
// OTEL-compliant.
func StreamServerMetricsInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()

		err := handler(srv, ss) // call the actual stream handler

		duration := time.Since(start).Seconds()
		code := status.Code(err)

		metrics.RequestCounter.Add(ss.Context(), 1,
			metric.WithAttributes(
				attribute.String("method", info.FullMethod),
				attribute.Int("status_code", int(code)),
			),
		)
		metrics.RequestHistogram.Record(ss.Context(), duration,
			metric.WithAttributes(
				attribute.String("method", info.FullMethod),
				attribute.Int("status_code", int(code)),
			),
		)

		return err
	}
}
