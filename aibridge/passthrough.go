package aibridge

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/keypool"
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

	// Build the passthrough proxy, reused across all requests for this provider.
	// Rewrite sets proxy headers. For centralized requests, KeyFailoverTransport
	// handles auth and failover. BYOK requests pass through.
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			rewritePassthroughRequest(pr, provBaseURL)
		},
		ModifyResponse: func(resp *http.Response) error {
			modelsPath := path.Join("/", provBaseURL.Path, "models")
			if prov.Type() != config.ProviderCopilot || resp.Request.Method != http.MethodGet || strings.TrimSuffix(resp.Request.URL.Path, "/") != modelsPath || resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return nil
			}
			return filterCopilotModelsResponse(resp)
		},
		Transport: keypool.NewKeyFailoverTransport(
			apidump.NewPassthroughMiddleware(t, prov.APIDumpDir(), prov.Name(), logger, quartz.NewReal()),
			prov.KeyFailoverConfig(logger),
		),
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, e error) {
			if _, ok := errors.AsType[*http.MaxBytesError](e); ok {
				writeRequestBodyTooLarge(req.Context(), rw)
			} else {
				logger.Warn(req.Context(), "reverse proxy error", slog.Error(e), slog.F("path", req.URL.Path))
				http.Error(rw, "upstream proxy error", http.StatusBadGateway)
			}
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

func filterCopilotModelsResponse(resp *http.Response) error {
	wireBody, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return xerrors.Errorf("read Copilot models response: %w", err)
	}
	_ = resp.Body.Close()

	body := wireBody
	contentEncoding := resp.Header.Get("Content-Encoding")
	if contentEncoding == "gzip" {
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			resp.Body = io.NopCloser(bytes.NewReader(body))
			return nil
		}
		decoded, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			resp.Body = io.NopCloser(bytes.NewReader(body))
			return nil
		}
		body = decoded
	} else if contentEncoding != "" {
		resp.Body = io.NopCloser(bytes.NewReader(wireBody))
		return nil
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		resp.Body = io.NopCloser(bytes.NewReader(wireBody))
		return nil
	}

	var models []json.RawMessage
	if err := json.Unmarshal(payload["data"], &models); err != nil {
		resp.Body = io.NopCloser(bytes.NewReader(wireBody))
		return nil
	}

	modified := false
	for i, rawModel := range models {
		var model map[string]json.RawMessage
		if err := json.Unmarshal(rawModel, &model); err != nil {
			continue
		}

		rawEndpoints, ok := model["supported_endpoints"]
		if !ok {
			continue
		}
		var endpoints []string
		if err := json.Unmarshal(rawEndpoints, &endpoints); err != nil {
			continue
		}

		filtered := endpoints[:0]
		for _, endpoint := range endpoints {
			if endpoint != "ws:/responses" {
				filtered = append(filtered, endpoint)
			}
		}
		if len(filtered) == len(endpoints) {
			continue
		}

		model["supported_endpoints"], err = json.Marshal(filtered)
		if err != nil {
			resp.Body = io.NopCloser(bytes.NewReader(wireBody))
			return nil
		}
		models[i], err = json.Marshal(model)
		if err != nil {
			resp.Body = io.NopCloser(bytes.NewReader(wireBody))
			return nil
		}
		modified = true
	}
	if !modified {
		resp.Body = io.NopCloser(bytes.NewReader(wireBody))
		return nil
	}

	payload["data"], err = json.Marshal(models)
	if err != nil {
		resp.Body = io.NopCloser(bytes.NewReader(wireBody))
		return nil
	}
	body, err = json.Marshal(payload)
	if err != nil {
		resp.Body = io.NopCloser(bytes.NewReader(wireBody))
		return nil
	}

	if contentEncoding == "gzip" {
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		if _, err := writer.Write(body); err != nil {
			resp.Body = io.NopCloser(bytes.NewReader(wireBody))
			return nil
		}
		if err := writer.Close(); err != nil {
			resp.Body = io.NopCloser(bytes.NewReader(wireBody))
			return nil
		}
		body = compressed.Bytes()
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	resp.Header.Del("Content-MD5")
	resp.Header.Del("ETag")
	return nil
}

// rewritePassthroughRequest configures the outbound request for the upstream and
// applies proxy headers.
func rewritePassthroughRequest(pr *httputil.ProxyRequest, provBaseURL *url.URL) {
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
