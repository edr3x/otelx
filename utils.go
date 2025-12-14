package otelx

import "os"

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
