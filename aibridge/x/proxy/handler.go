package proxy

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/clientmeta"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/quartz"
)

// Proxy mode does not parse protocol payloads, so the model remains unknown
// until extractor integration.
const unknownModel = ""

type forwardingHandler struct {
	provider    provider.Provider
	baseURL     *url.URL
	transport   http.RoundTripper
	breaker     *circuitbreaker.ProviderCircuitBreakers
	failover    keypool.KeyFailoverConfig
	logger      slog.Logger
	metrics     *metrics.Metrics
	tracer      trace.Tracer
	metricRoute string
}

// newForwardingHandler constructs the complete non-recording forwarding path.
// The returned transport is owned by the eventual router and must be closed when
// that provider snapshot is retired.
func newForwardingHandler(prov provider.Provider, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer, metricRoute string) (*forwardingHandler, *http.Transport, error) {
	baseURL, err := url.Parse(prov.BaseURL())
	if err != nil {
		return nil, nil, xerrors.Errorf("configure provider %q base URL: %w", prov.Name(), err)
	}
	if (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
		return nil, nil, xerrors.Errorf("configure provider %q base URL: absolute HTTP or HTTPS URL with host required", prov.Name())
	}
	if _, err := url.JoinPath(baseURL.Path, "/"); err != nil {
		return nil, nil, xerrors.Errorf("configure provider %q base URL path: %w", prov.Name(), err)
	}

	failover := prov.KeyFailoverConfig(logger)
	if failover.Pool != nil {
		switch {
		case failover.IsBYOK == nil:
			return nil, nil, xerrors.Errorf("configure provider %q key failover: IsBYOK callback is required with a key pool", prov.Name())
		case failover.InjectAuthKey == nil:
			return nil, nil, xerrors.Errorf("configure provider %q key failover: InjectAuthKey callback is required with a key pool", prov.Name())
		case failover.BuildKeyPoolResponse == nil:
			return nil, nil, xerrors.Errorf("configure provider %q key failover: BuildKeyPoolResponse callback is required with a key pool", prov.Name())
		}
	}
	if tracer == nil {
		tracer = noop.NewTracerProvider().Tracer("proxy")
	}

	transport := utils.NewStreamingTransport()
	// Response observation must see the same encoded bytes copied to the client.
	transport.DisableCompression = true
	dumpTransport := apidump.NewPassthroughMiddleware(
		transport, prov.APIDumpDir(), prov.Name(), logger, quartz.NewReal(),
	)
	return &forwardingHandler{
		provider:    prov,
		baseURL:     baseURL,
		transport:   dumpTransport,
		failover:    failover,
		logger:      logger,
		metrics:     m,
		tracer:      tracer,
		metricRoute: metricRoute,
	}, transport, nil
}

func (h *forwardingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "Proxy")
	defer span.End()
	r = r.WithContext(ctx)

	client := clientmeta.GuessClient(r)
	actorID := aibcontext.ActorIDFromContext(ctx)
	log := h.logger.With(
		slog.F("provider", h.provider.Name()),
		slog.F("path", r.URL.Path),
		slog.F("method", r.Method),
		slog.F("actor_id", actorID),
		slog.F("client", string(client)),
	)

	if clientmeta.HasConnectionUpgrade(r) {
		log.Debug(ctx, "rejecting unsupported HTTP upgrade")
		http.Error(w, "HTTP upgrades are not supported", http.StatusNotImplemented)
		return
	}
	if _, _, err := clientmeta.ExtractAgentFirewallHeaders(r); err != nil {
		log.Warn(ctx, "rejecting request with invalid agent firewall headers", slog.Error(err))
		http.Error(w, "invalid agent firewall headers", http.StatusBadRequest)
		return
	}
	if err := routing.ValidateForwardPath(r.URL); err != nil {
		log.Warn(ctx, "rejecting unsafe upstream path", slog.Error(err))
		http.Error(w, "invalid request path", http.StatusBadRequest)
		return
	}
	if r.ContentLength > routing.MaxRequestBodyBytes {
		if r.Body != nil {
			_ = r.Body.Close()
		}
		log.Warn(ctx, "rejecting oversized request body")
		routing.WriteRequestBodyTooLarge(ctx, w)
		return
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, routing.MaxRequestBodyBytes)
	}

	state := forwardingState{}
	defer func() {
		panicValue := recover()
		aborted, _ := terminalError(r, &state, panicValue)
		if panicValue != nil {
			panic(panicValue)
		}
		if aborted {
			panic(http.ErrAbortHandler)
		}
	}()

	if h.metrics != nil {
		h.metrics.PassthroughCount.WithLabelValues(
			h.provider.Name(), h.metricRoute, routing.MetricMethod(r.Method),
		).Inc()
	}

	requestProxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			rewriteRequest(pr, h.provider.RoutePrefix(), h.baseURL)
		},
		Transport:     keypool.NewKeyFailoverTransport(h.transport, h.failover),
		FlushInterval: -1,
		ModifyResponse: func(resp *http.Response) error {
			utils.StripSensitiveResponseHeaders(resp.Header)
			utils.DropResponseTrailers(resp)
			state.status = resp.StatusCode
			if resp.StatusCode == http.StatusSwitchingProtocols {
				return xerrors.New("upstream protocol upgrades are not supported")
			}
			state.body = newObservedBody(resp.Body, nil)
			resp.Body = state.body
			return nil
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
			state.transportErr = proxyErr
			if _, ok := errors.AsType[*http.MaxBytesError](proxyErr); ok {
				if r.Body != nil {
					_ = r.Body.Close()
				}
				log.Warn(req.Context(), "rejecting oversized request body", slog.Error(proxyErr))
				routing.WriteRequestBodyTooLarge(req.Context(), rw)
				return
			}
			requestEnded := req.Context().Err() != nil &&
				(errors.Is(proxyErr, context.Canceled) || errors.Is(proxyErr, context.DeadlineExceeded))
			if requestEnded {
				log.Debug(req.Context(), "reverse proxy request ended", slog.Error(proxyErr))
			} else {
				log.Warn(req.Context(), "reverse proxy error", slog.Error(proxyErr))
			}
			span.SetStatus(codes.Error, "upstream proxy error")
			http.Error(rw, "upstream proxy error", http.StatusBadGateway)
		},
		ErrorLog: slog.Stdlib(ctx, log, slog.LevelWarn),
	}

	state.execErr = h.breaker.Execute(h.metricRoute, unknownModel, w, func(rw http.ResponseWriter) error {
		writer := &errorCapturingResponseWriter{ResponseWriter: rw}
		defer func() { state.writeErr = writer.err }()
		requestProxy.ServeHTTP(writer, r)
		return nil
	})
}

func rewriteRequest(pr *httputil.ProxyRequest, routePrefix string, baseURL *url.URL) {
	pr.Out.URL.Path = strings.TrimPrefix(pr.In.URL.Path, routePrefix)
	if pr.In.URL.RawPath != "" {
		pr.Out.URL.RawPath = strings.TrimPrefix(pr.In.URL.RawPath, routePrefix)
	}
	pr.Out.URL.RawQuery = pr.In.URL.RawQuery
	pr.SetURL(baseURL)
	trace.SpanFromContext(pr.Out.Context()).SetAttributes(
		attribute.String(tracing.PassthroughUpstreamURL, pr.Out.URL.String()),
	)
	// Proxy mode never injects configured actor headers because it does not build
	// provider SDK requests. Client-supplied actor headers are always untrusted.
	utils.StripSensitiveRequestHeaders(pr.Out.Header)
	utils.DropRequestTrailers(pr.Out)
}

type forwardingState struct {
	// ReverseProxy and its transports update this state synchronously on the
	// serving goroutine. The stdlib flush timer never accesses these fields.
	status       int
	transportErr error
	execErr      error
	writeErr     error
	body         *observedBody
}

// terminalError reports whether the response stream must be aborted and the
// terminal forwarding error. Breaker errors retain precedence over copy aborts.
func terminalError(r *http.Request, state *forwardingState, panicValue any) (aborted bool, terminal error) {
	var abortErr error
	switch {
	case r.Context().Err() != nil && (state.body == nil || !state.body.eof):
		abortErr = r.Context().Err()
	case state.body != nil && state.body.readErr != nil:
		abortErr = state.body.readErr
	case state.writeErr != nil:
		abortErr = state.writeErr
	case panicValue != nil:
		abortErr = xerrors.Errorf("proxy stream aborted: %v", panicValue)
	case state.body != nil && !state.body.eof && state.transportErr == nil:
		abortErr = xerrors.New("proxy stream aborted before EOF")
	}

	switch {
	case state.execErr != nil:
		return abortErr != nil, state.execErr
	case abortErr != nil:
		return true, abortErr
	case state.transportErr != nil:
		return false, state.transportErr
	default:
		return false, nil
	}
}

type errorCapturingResponseWriter struct {
	http.ResponseWriter
	err error
}

func (w *errorCapturingResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if err != nil && w.err == nil {
		w.err = err
	}
	return n, err
}

func (w *errorCapturingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *errorCapturingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
}

func (w *errorCapturingResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
