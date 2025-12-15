package otelx

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// HTTPClient returns a new *http.Client configured with OpenTelemetry tracing support.
//
// It performs two key tasks:
//  1. Injects the current trace context into the given HTTP request headers,
//     allowing distributed tracing across service boundaries.
//  2. Wraps the default HTTP transport with otelhttp.NewTransport to automatically
//     record metrics and spans for outgoing HTTP requests.
//
// The client uses a 20-second timeout by default.
//
// Example:
//
//	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.example.com", body)
//	client := HTTPClient(ctx, req)
//	resp, err := client.Do(req)
//	if err != nil {
//		log.Println("Request failed:", err)
//	}
//	defer resp.Body.Close()
//
// Note: You must call this function *before* sending the request to ensure
// trace propagation headers are properly included.
func HTTPClient(ctx context.Context, req *http.Request) *http.Client {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
}

// DoRequest executes an HTTP request with OpenTelemetry tracing and context propagation.
//
// It is a convenience wrapper that automatically:
//  1. Injects the trace context from the provided context into the request headers.
//  2. Creates an instrumented *http.Client* using HTTPClient for distributed tracing.
//  3. Executes the HTTP request and returns the response.
//
// This function is part of a flexible utility layer — developers can either:
//   - Use DoRequest() for simple, one-off HTTP calls.
//   - Use HTTPClient() directly when more control is needed (e.g., reusing the same client,
//     customizing timeouts, or applying retries).
//
// Example:
//
//	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com", nil)
//	resp, err := otelx.DoRequest(ctx, req)
//	if err != nil {
//		log.Println("Request failed:", err)
//		return
//	}
//	defer resp.Body.Close()
//
//	if resp.StatusCode != http.StatusOK {
//		log.Printf("Unexpected status: %d", resp.StatusCode)
//	}
//
// Return values:
//   - *http.Response: the HTTP response returned by the server
//   - error: if the request fails or the context is canceled
func DoRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	client := HTTPClient(ctx, req)
	return client.Do(req)
}
