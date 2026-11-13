package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	aibclient "github.com/coder/coder/v2/aibridge/client"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/credential"
	"github.com/coder/coder/v2/aibridge/headers"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/interceptionerror"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/coder/v2/aibridge/x/proxy/replaybody"
	"github.com/coder/quartz"
)

type forwardingHandler struct {
	provider provider.Provider
	baseURL  *url.URL
	logger   slog.Logger
	tracer   trace.Tracer
	errorLog *log.Logger
	// transport is shared by the per-request proxies so they reuse upstream
	// connections.
	transport http.RoundTripper
	failover  keypool.KeyFailoverConfig
	inflight  *aibridge.InflightGate
	recorder  recorder.Recorder
	breaker   *circuitbreaker.ProviderCircuitBreakers
}

// forwardResult describes how forwarding one request ended.
type forwardResult struct {
	// status is the final upstream status. Zero when no upstream response was
	// accepted.
	status int
	// response captures the final upstream response body. Nil when no upstream
	// response was accepted.
	response *replaybody.TeeBuffer
	// credentialHint identifies the last pooled key used. Empty when no pooled
	// key was used.
	credentialHint string
}

var _ http.Handler = (*forwardingHandler)(nil)

// newForwardingHandler constructs one handler per provider snapshot.
func newForwardingHandler(prov provider.Provider, logger slog.Logger, m *metrics.Metrics, tracer trace.Tracer, inflight *aibridge.InflightGate, rec recorder.Recorder) (*forwardingHandler, error) {
	if rec == nil {
		return nil, xerrors.New("recorder is required")
	}
	baseURL, err := url.Parse(prov.BaseURL())
	if err != nil {
		return nil, xerrors.Errorf("configure provider %q base URL: %w", prov.Name(), err)
	}
	transport := apidump.NewPassthroughMiddleware(utils.NewStreamingTransport(), prov.APIDumpDir(), prov.Name(), logger, quartz.NewReal())
	// Providers that adjust upstream requests themselves, such as Claude
	// Platform, wrap the transport beneath key failover.
	if wrapper, ok := prov.(provider.PassthroughTransportWrapper); ok {
		transport = wrapper.WrapPassthroughTransport(transport)
	}
	h := &forwardingHandler{
		provider:  prov,
		baseURL:   baseURL,
		logger:    logger.With(slog.F("provider", prov.Name())),
		tracer:    tracer,
		transport: transport,
		failover:  prov.KeyFailoverConfig(logger),
		inflight:  inflight,
		recorder:  rec,
		breaker:   circuitbreaker.NewProviderCircuitBreakers(prov.Name(), prov.CircuitBreakerConfig(), logger, m),
	}
	h.errorLog = slog.Stdlib(context.Background(), h.logger, slog.LevelWarn)
	return h, nil
}

func rewriteForwardingURL(pr *httputil.ProxyRequest, routePrefix string, baseURL *url.URL) {
	pr.Out.URL.Path = strings.TrimPrefix(pr.In.URL.Path, routePrefix)
	if pr.In.URL.RawPath != "" {
		pr.Out.URL.RawPath = strings.TrimPrefix(pr.In.URL.RawPath, routePrefix)
	}
	// Keep the client's query bytes. ReverseProxy rewrites queries it cannot parse.
	pr.Out.URL.RawQuery = pr.In.URL.RawQuery
	pr.SetURL(baseURL)
}

func prepareForwardingHeaders(clientHeaders http.Header, cred credential.Credential) http.Header {
	prepared := headers.PrepareClientHeaders(clientHeaders)
	if byok, ok := credential.AsBYOK(cred); ok {
		value := byok.Secret
		if byok.Header == headers.AuthHeaderAuthorization {
			value = "Bearer " + value
		}
		prepared.Set(byok.Header, value)
	}
	return prepared
}

func (h *forwardingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	release, ok := h.inflight.Admit()
	if !ok {
		http.Error(w, "AI Gateway is shutting down", http.StatusServiceUnavailable)
		return
	}
	ctx := r.Context()
	var res forwardResult
	var record *recorder.InterceptionRecord
	err := xerrors.New("forwarding did not complete")
	defer func() { h.finishForwarding(ctx, record, res, err, release) }()

	ctx, cleanup := h.inflight.RequestContext(ctx)
	defer cleanup()
	ctx, span := h.tracer.Start(ctx, "Proxy")
	defer span.End()
	r = r.WithContext(ctx)
	logger, record, cred := h.startForwarding(ctx, w, r)
	if record == nil {
		return
	}

	route := strings.TrimPrefix(r.URL.Path, "/"+h.provider.Name())
	err = h.breaker.Execute(route, "", w, func(rw http.ResponseWriter) error {
		outbound, body := replaybody.NewRequest(r)
		defer func() { _ = body.Close() }()
		return h.forward(rw, outbound, cred, &res, logger)
	})
	if err != nil {
		span.SetStatus(codes.Error, "forwarding failed")
	}
}

// startForwarding validates the request and records its start before forwarding.
func (h *forwardingHandler) startForwarding(ctx context.Context, w http.ResponseWriter, r *http.Request) (slog.Logger, *recorder.InterceptionRecord, credential.Credential) {
	logger, record, cred := h.checkRequest(w, r)
	if record == nil {
		return logger, nil, nil
	}
	// Forwarding captures the request body as it streams upstream. Without full
	// duplex, the HTTP/1 server drains the original body past the capture before
	// writing an early reply. Writers that cannot enable it keep the default drain.
	if err := http.NewResponseController(w).EnableFullDuplex(); err != nil {
		logger.Debug(ctx, "full duplex unavailable, early replies drain the request body first", slog.Error(err))
	}

	// The start record must exist before forwarding or recording its end.
	if err := h.recorder.RecordInterception(ctx, record); err != nil {
		trace.SpanFromContext(ctx).SetStatus(codes.Error, "failed to record interception")
		logger.Warn(ctx, "failed to record interception", slog.Error(err))
		// TODO: Return this error in the provider's JSON format.
		http.Error(w, "failed to record interception", http.StatusInternalServerError)
		return logger, nil, nil
	}
	return logger, record, cred
}

// finishForwarding asynchronously records interception end and after releases in-flight admission.
func (h *forwardingHandler) finishForwarding(ctx context.Context, record *recorder.InterceptionRecord, res forwardResult, err error, release func()) {
	// No start record means the request was not forwarded, do not record an end.
	if record == nil {
		release()
		return
	}
	endedAt := time.Now().UTC()
	// Run in go-routine to let the response finish without waiting for recording.
	go func() {
		defer release() // Release in-flight gate admission after recording.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recorder.DefaultAsyncTimeout)
		defer cancel()

		// Preserve errors from the circuit breaker or reverse proxy.
		// Otherwise, check for an upstream body read error captured by the tee.
		if err == nil && res.response != nil {
			_, err = res.response.Wait(ctx)
		}

		// Categorize Go errors first, HTTP error responses may have no Go error.
		errType, errMsg := interceptionerror.Categorize(h.provider, err)
		if err == nil && res.status >= http.StatusBadRequest {
			errType = recorder.ErrorTypeFromStatus(res.status)
			errMsg = http.StatusText(res.status)
		}

		// Keep the initial hint for BYOK, or use the last attempted pooled key.
		hint := record.CredentialHint
		if res.credentialHint != "" {
			hint = res.credentialHint
		}
		if err := h.recorder.RecordInterceptionEnded(ctx, &recorder.InterceptionRecordEnded{
			ID:             record.ID,
			EndedAt:        endedAt,
			CredentialHint: hint,
			ErrorType:      errType,
			ErrorMessage:   errMsg,
		}); err != nil {
			h.logger.Warn(ctx, "failed to record interception end", slog.F("interception_id", record.ID), slog.Error(err))
		}
	}()
}

// checkRequest validates a bridged request without reading its body. On rejection
// it writes the response and returns a nil record and credential. Otherwise it
// returns the request logger, initial interception record, and credential
// without writing a response.
func (h *forwardingHandler) checkRequest(w http.ResponseWriter, r *http.Request) (slog.Logger, *recorder.InterceptionRecord, credential.Credential) {
	ctx := r.Context()
	client := aibclient.GuessClient(r)

	logger := h.logger.With(
		slog.F("path", r.URL.Path),
		slog.F("user_agent", r.UserAgent()),
		slog.F("client", string(client)),
	)

	if headers.IsWebSocketUpgrade(r) {
		logger.Debug(ctx, "rejecting unsupported WebSocket upgrade")
		http.Error(w, "WebSocket transport is not supported, use HTTP", http.StatusNotImplemented)
		return logger, nil, nil
	}

	if r.ContentLength > routing.MaxRequestBodyBytes {
		logger.Debug(ctx, "rejecting oversized request body", slog.F("content_length", r.ContentLength))
		routing.WriteRequestBodyTooLarge(ctx, w)
		return logger, nil, nil
	}

	firewallID, firewallSequence, err := headers.ExtractAgentFirewallHeaders(r)
	if err != nil {
		logger.Warn(ctx, "rejecting request with invalid agent firewall headers", slog.Error(err))
		http.Error(w, "invalid agent firewall headers", http.StatusBadRequest)
		return logger, nil, nil
	}

	actor := aibcontext.ActorFromContext(ctx)
	if actor == nil {
		logger.Warn(ctx, "rejecting request without an actor")
		http.Error(w, "no actor found", http.StatusBadRequest)
		return logger, nil, nil
	}
	logger = logger.With(slog.F("initiator_id", actor.ID.String()))

	cred, err := h.provider.ResolveCredential(r)
	if err != nil {
		trace.SpanFromContext(ctx).SetStatus(codes.Error, "failed to resolve credential")
		logger.Warn(ctx, "failed to resolve credential", slog.Error(err))
		if errors.Is(err, provider.ErrNoCredential) {
			http.Error(w, "upstream authentication unavailable: no provider credentials supplied or configured", http.StatusForbidden)
		} else {
			http.Error(w, "upstream authentication unavailable", http.StatusInternalServerError)
		}
		return logger, nil, nil
	}

	switch cred.(type) {
	case credential.BYOK, *credential.CentralizedPool:
	default:
		// The type name identifies the credential kind; the value may hold secrets.
		// TODO: https://linear.app/codercom/issue/AIGOV-628
		logger.Warn(ctx, "rejecting unsupported upstream credential", slog.F("credential_type", fmt.Sprintf("%T", cred)))
		http.Error(w, "upstream authentication is not supported in proxy mode", http.StatusNotImplemented)
		return logger, nil, nil
	}
	var metadata recorder.Metadata
	if actor.Username != "" {
		metadata = recorder.Metadata{"Username": actor.Username}
	}
	ir := &recorder.InterceptionRecord{
		StartedAt:                   time.Now().UTC(),
		ID:                          uuid.NewString(),
		InitiatorID:                 actor.ID.String(),
		Metadata:                    metadata,
		Provider:                    h.provider.Type(),
		ProviderName:                h.provider.Name(),
		UserAgent:                   r.UserAgent(),
		Client:                      string(client),
		AgentFirewallSessionID:      firewallID,
		AgentFirewallSequenceNumber: firewallSequence,
		CredentialKind:              cred.Kind(),
		CredentialHint:              cred.Hint(),
		// Model:                    TODO, depends on extractor
		// ClientSessionID:          TODO, depends on request buffering
		// CorrelatingToolCallID:    TODO, depends on extractor
		// TODO https://linear.app/codercom/issue/AIGOV-613
	}

	return logger, ir, cred
}

// forward proxies r upstream and writes the final response after key failover
// to w. When no upstream response is accepted, it writes an error response and
// returns the cause.
func (h *forwardingHandler) forward(w http.ResponseWriter, r *http.Request, cred credential.Credential, res *forwardResult, logger slog.Logger) (err error) {
	defer func() {
		// The capture keeps the upstream body open until it is closed.
		if res.response != nil {
			_ = res.response.Close()
		}
	}()
	failover := h.failover
	if failover.Pool != nil {
		// TODO: key failover transport should provide the key hint and the key
		// pool error. https://linear.app/codercom/issue/AIGOV-795
		failover.InjectAuthKey = func(header *http.Header, key string) {
			res.credentialHint = utils.MaskSecret(key)
			h.failover.InjectAuthKey(header, key)
		}
		failover.BuildKeyPoolResponse = func(keyErr *keypool.Error) *http.Response {
			err = keyErr
			return h.failover.BuildKeyPoolResponse(keyErr)
		}
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			rewriteForwardingURL(pr, h.provider.RoutePrefix(), h.baseURL)
			pr.Out.Header = prepareForwardingHeaders(pr.Out.Header, cred)
		},
		Transport:     keypool.NewKeyFailoverTransport(h.transport, failover),
		FlushInterval: -1,
		// Only the final response returned by key failover is observed.
		ModifyResponse: func(resp *http.Response) error {
			if resp.StatusCode == http.StatusSwitchingProtocols {
				return xerrors.New("upstream protocol upgrades are not supported")
			}
			res.status = resp.StatusCode
			res.response = replaybody.NewResponse(resp)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, proxyErr error) {
			err = proxyErr
			ctx := r.Context()
			//nolint:errcheck // AsType returns the match; only its presence matters.
			if _, ok := errors.AsType[*http.MaxBytesError](proxyErr); ok {
				routing.WriteRequestBodyTooLarge(ctx, w)
				return
			}
			logger.Warn(ctx, "upstream proxy error", slog.Error(proxyErr))
			// TODO: Return this error in the provider's JSON format.
			http.Error(w, "upstream proxy error", http.StatusBadGateway)
		},
		ErrorLog: h.errorLog,
	}
	proxy.ServeHTTP(w, r)
	return err
}
