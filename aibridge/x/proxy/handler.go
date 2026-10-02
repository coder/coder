package proxy

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog/v3"
	aibclient "github.com/coder/coder/v2/aibridge/client"
	"github.com/coder/coder/v2/aibridge/headers"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/routing"
)

// newForwardingHandler prepares requests before provider-specific handling.
func newForwardingHandler(prov provider.Provider, logger slog.Logger, tracer trace.Tracer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "Proxy")
		defer span.End()

		client := aibclient.GuessClient(r)
		if headers.IsWebSocketUpgrade(r) {
			logger.Debug(ctx, "rejecting unsupported WebSocket upgrade",
				slog.F("provider", prov.Name()),
				slog.F("route", strings.TrimPrefix(r.URL.Path, prov.RoutePrefix())),
				slog.F("client", string(client)),
			)
			http.Error(w, "WebSocket transport is not supported, use HTTP", http.StatusNotImplemented)
			return
		}
		if _, _, err := headers.ExtractAgentFirewallHeaders(r); err != nil {
			logger.Warn(ctx, "rejecting request with invalid agent firewall headers", slog.Error(err))
			http.Error(w, "invalid agent firewall headers", http.StatusBadRequest)
			return
		}

		if r.ContentLength > routing.MaxRequestBodyBytes {
			if r.Body != nil {
				_ = r.Body.Close()
			}
			logger.Debug(ctx, "rejecting oversized request body",
				slog.F("provider", prov.Name()),
				slog.F("route", strings.TrimPrefix(r.URL.Path, prov.RoutePrefix())),
				slog.F("client", string(client)),
				slog.F("content_length", r.ContentLength),
			)
			routing.WriteRequestBodyTooLarge(ctx, w)
			return
		}

		// TODO: forward to provider + provider specific preparations.
		http.NotFound(w, r)
	}
}
