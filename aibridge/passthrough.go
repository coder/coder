package aibridge

import (
	"context"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/coder/v2/aibridge/utils"
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

	// The shared transport is tuned for streaming and deliberately omits a
	// response header timeout.
	t := utils.NewStreamingTransport()

	// Build the passthrough proxy, reused across all requests for this provider.
	// Rewrite sets proxy headers. For centralized requests, KeyFailoverTransport
	// handles auth and failover. BYOK requests pass through.
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			rewritePassthroughRequest(pr, provBaseURL)
		},
		Transport: keypool.NewKeyFailoverTransport(
			apidump.NewPassthroughMiddleware(t, prov.APIDumpDir(), prov.Name(), logger, quartz.NewReal()),
			prov.KeyFailoverConfig(logger),
		),
		ModifyResponse: func(resp *http.Response) error {
			utils.StripSensitiveResponseHeaders(resp.Header)
			if resp.StatusCode != http.StatusSwitchingProtocols {
				utils.DropResponseTrailers(resp)
			}
			return nil
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, e error) {
			if _, ok := errors.AsType[*http.MaxBytesError](e); ok {
				routing.WriteRequestBodyTooLarge(req.Context(), rw)
			} else {
				logger.Warn(req.Context(), "reverse proxy error", slog.Error(e), slog.F("path", req.URL.Path))
				http.Error(rw, "upstream proxy error", http.StatusBadGateway)
			}
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if m != nil {
			m.PassthroughCount.WithLabelValues(prov.Name(), passthroughMetricRoute(prov, r), routing.MetricMethod(r.Method)).Add(1)
		}

		ctx, span := startSpan(r, tracer)
		defer span.End()

		if err := routing.ValidateForwardPath(r.URL); err != nil {
			logger.Warn(ctx, "rejecting unsafe upstream path", slog.Error(err), slog.F("path", r.URL.Path))
			http.Error(w, "invalid request path", http.StatusBadRequest)
			return
		}
		requestProxy := *proxy
		requestProxy.ErrorLog = slog.Stdlib(ctx, logger, slog.LevelWarn)
		requestProxy.ServeHTTP(w, r.WithContext(ctx))
	}
}

// rewritePassthroughRequest configures the outbound request for the upstream and
// applies proxy headers.
func rewritePassthroughRequest(pr *httputil.ProxyRequest, provBaseURL *url.URL) {
	pr.SetURL(provBaseURL)
	utils.StripSensitiveRequestHeaders(pr.Out.Header)
	utils.DropRequestTrailers(pr.Out)

	// SetXForwarded synthesizes a new trusted proxy chain from the request peer.
	// Client-supplied Forwarded and X-Forwarded-* values were removed above.
	pr.SetXForwarded()

	span := trace.SpanFromContext(pr.Out.Context())
	span.SetAttributes(attribute.String(tracing.PassthroughUpstreamURL, pr.Out.URL.String()))

	// Avoid default Go user-agent if none provided.
	if _, ok := pr.Out.Header["User-Agent"]; !ok {
		pr.Out.Header.Set("User-Agent", "aibridge") // TODO: use build tag.
	}
}

// newInvalidBaseURLHandler returns a handler that always returns 502
// when the provider's base URL is invalid.
func newInvalidBaseURLHandler(prov provider.Provider, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer, baseURLErr error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := startSpan(r, tracer)
		defer span.End()

		if m != nil {
			m.PassthroughCount.WithLabelValues(prov.Name(), passthroughMetricRoute(prov, r), routing.MetricMethod(r.Method)).Add(1)
		}

		logger.Warn(ctx, "invalid provider base URL", slog.Error(baseURLErr))
		http.Error(w, "invalid provider base URL", http.StatusBadGateway)
		span.SetStatus(codes.Error, "invalid provider base URL: "+baseURLErr.Error())
	}
}

func passthroughMetricRoute(prov provider.Provider, r *http.Request) string {
	if route, ok := strings.CutPrefix(r.Pattern, prov.RoutePrefix()); ok && route != "" {
		return route
	}
	return "/"
}

func startSpan(r *http.Request, tracer trace.Tracer) (context.Context, trace.Span) {
	return tracer.Start(r.Context(), "Passthrough", trace.WithAttributes(
		attribute.String(tracing.PassthroughURL, r.URL.String()),
		attribute.String(tracing.PassthroughMethod, r.Method),
	))
}
