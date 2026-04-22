package aibridge

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/quartz"
)

// newPassthroughRouter returns a simple reverse-proxy implementation which will be used when a route is not handled specifically
// by a [intercept.Provider].
// A single reverse proxy is created per provider and reused across all requests.
func newPassthroughRouter(prov provider.Provider, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer) http.HandlerFunc {
	provBaseURL, err := url.Parse(prov.BaseURL())
	if err != nil {
		return newInvalidBaseURLHandler(prov, logger, m, tracer, err)
	}
	if _, err := url.JoinPath(provBaseURL.Path, "/"); err != nil {
		return newInvalidBaseURLHandler(prov, logger, m, tracer, err)
	}

	// Transport tuned for streaming (no response header timeout).
	t := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Build a reverse proxy to the upstream, reused across all requests for this provider.
	// All request modifications happen in Rewrite.
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			rewritePassthroughRequest(pr, provBaseURL, prov)
		},
		Transport: apidump.NewPassthroughMiddleware(t, prov.APIDumpDir(), prov.Name(), logger, quartz.NewReal()),
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, e error) {
			logger.Warn(req.Context(), "reverse proxy error", slog.Error(e), slog.F("path", req.URL.Path))
			http.Error(rw, "upstream proxy error", http.StatusBadGateway)
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if m != nil {
			m.PassthroughCount.WithLabelValues(prov.Name(), r.URL.Path, r.Method).Add(1)
		}

		ctx, span := startSpan(r, tracer)
		defer span.End()

		proxy.ServeHTTP(w, r.WithContext(ctx))
	}
}

// rewritePassthroughRequest configures the outbound request for the upstream and
// applies proxy headers and provider auth.
func rewritePassthroughRequest(pr *httputil.ProxyRequest, provBaseURL *url.URL, prov provider.Provider) {
	pr.SetURL(provBaseURL)

	// Rewrite sets "X-Forwarded-For" to just last hop (clients IP address).
	// To preserve old Director behavior pr.In "X-Forwarded-For" header
	// values need to be copied manually.
	// https://pkg.go.dev/net/http/httputil#ProxyRequest.SetXForwarded
	if prior, ok := pr.In.Header["X-Forwarded-For"]; ok {
		pr.Out.Header["X-Forwarded-For"] = append([]string(nil), prior...)
	}
	pr.SetXForwarded()

	span := trace.SpanFromContext(pr.Out.Context())
	span.SetAttributes(attribute.String(tracing.PassthroughUpstreamURL, pr.Out.URL.String()))

	// Avoid default Go user-agent if none provided.
	if _, ok := pr.Out.Header["User-Agent"]; !ok {
		pr.Out.Header.Set("User-Agent", "aibridge") // TODO: use build tag.
	}

	// Inject provider auth.
	prov.InjectAuthHeader(&pr.Out.Header)
}

// newInvalidBaseURLHandler returns a handler that always returns 502
// when the provider's base URL is invalid.
func newInvalidBaseURLHandler(prov provider.Provider, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer, baseURLErr error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := startSpan(r, tracer)
		defer span.End()

		if m != nil {
			m.PassthroughCount.WithLabelValues(prov.Name(), r.URL.Path, r.Method).Add(1)
		}

		logger.Warn(ctx, "invalid provider base URL", slog.Error(baseURLErr))
		http.Error(w, "invalid provider base URL", http.StatusBadGateway)
		span.SetStatus(codes.Error, "invalid provider base URL: "+baseURLErr.Error())
	}
}

func startSpan(r *http.Request, tracer trace.Tracer) (context.Context, trace.Span) {
	return tracer.Start(r.Context(), "Passthrough", trace.WithAttributes(
		attribute.String(tracing.PassthroughURL, r.URL.String()),
		attribute.String(tracing.PassthroughMethod, r.Method),
	))
}
