package otelx

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// defaultClient is the shared HTTP client used by DoRequest.
//
// It is declared at package scope (rather than constructed per call) so that
// the underlying transport's connection pool is reused across requests.
// Creating a new *http.Client on every call would discard idle connections
// and force a fresh TCP/TLS handshake each time, which is wasteful under
// any non-trivial load.
//
// Configuration:
//
//   - Timeout: 20 seconds. This is an end-to-end deadline covering connection,
//     any redirects, and reading the response body. Callers that need a
//     different deadline should use context.WithTimeout on the request's
//     context rather than mutating this client.
//   - Transport: http.DefaultTransport wrapped with otelhttp.NewTransport,
//     which records a span and standard HTTP metrics for every request.
//
// *http.Client is safe for concurrent use by multiple goroutines, so this
// single instance can serve the entire process.
var defaultClient = &http.Client{
	Timeout:   20 * time.Second,
	Transport: otelhttp.NewTransport(http.DefaultTransport),
}

// DoRequest executes an HTTP request with OpenTelemetry tracing and
// context propagation.
//
// It performs two steps before delegating to the shared client:
//
//  1. Injects the trace context carried by ctx into req.Header using the
//     globally configured TextMapPropagator. This lets downstream services
//     stitch their spans onto the current trace.
//  2. Sends the request through the package's shared, otelhttp-instrumented
//     client, which records a client span and standard HTTP metrics.
//
// The provided ctx should normally be the same context attached to req via
// http.NewRequestWithContext. Cancelling ctx (e.g. via context.WithTimeout
// or context.WithCancel) cancels the in-flight request.
//
// DoRequest is intended for the common case of a single, one-off HTTP call.
// If you need finer control - custom timeouts per call, retries, a different
// transport, or a long-lived client tied to a specific dependency -
// construct your own *http.Client wrapped with otelhttp.NewTransport and
// inject the propagator yourself.
//
// Example:
//
//	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com", nil)
//	if err != nil {
//	    return err
//	}
//	resp, err := otelx.DoRequest(ctx, req)
//	if err != nil {
//	    log.Println("request failed:", err)
//	    return err
//	}
//	defer resp.Body.Close()
//
//	if resp.StatusCode != http.StatusOK {
//	    log.Printf("unexpected status: %d", resp.StatusCode)
//	}
//
// Returns:
//   - *http.Response: the response from the server. The caller is
//     responsible for closing resp.Body.
//   - error: a non-nil error if the request could not be sent, the context
//     was cancelled, or the configured timeout was exceeded. Note that
//     non-2xx HTTP status codes are *not* returned as errors; callers
//     must inspect resp.StatusCode themselves.
func DoRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	return defaultClient.Do(req)
}
