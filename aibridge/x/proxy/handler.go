package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	"github.com/coder/coder/v2/aibridge/extractors"
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

// errForwardingAborted marks forwarding that ended without returning, such as
// a panic that aborts the response copy. endRecord replaces it with the real
// cause when one is known.
var errForwardingAborted = xerrors.New("forwarding aborted")

// forwardedRequest tracks single bridged request
type forwardedRequest struct {
	handler *forwardingHandler

	// logger, cred, and record are set by start after the request passes
	// validation and its interception start is recorded. record is nil
	// otherwise.
	logger slog.Logger
	cred   credential.Credential
	record *recorder.InterceptionRecord
	// extractor derives recording facts for the route, set by start. Nil when
	// the route has no extractor yet.
	extractor extractors.Extractor

	// outbound is the request passed to the reverse proxy, set by start once
	// the request body has been read. Nil otherwise.
	outbound *http.Request
	// requestBody captures the outbound request body for replay. Set with
	// outbound.
	requestBody *replaybody.TeeBuffer

	// status is the final upstream response status, set by forward.
	// Zero when no upstream response was accepted.
	status int
	// responseBody captures the final upstream response body, set by forward.
	// Nil when no upstream response was accepted.
	responseBody *replaybody.TeeBuffer
	// credentialHint identifies the last pooled key used, set by forward.
	// Empty when no pooled key was used.
	credentialHint string

	// err is why forwarding failed, including circuit breaker rejection.
	// errForwardingAborted until forwarding returns.
	err error
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
	fr := &forwardedRequest{handler: h}
	fr.serve(w, r)
}

// serve admits, validates, and forwards r, recording the outcome in fr.
func (fr *forwardedRequest) serve(w http.ResponseWriter, r *http.Request) {
	release, ok := fr.handler.inflight.Admit()
	if !ok {
		http.Error(w, "AI Gateway is shutting down", http.StatusServiceUnavailable)
		return
	}
	// Set the sentinel in case forwarding panics and breaker.Execute does not return.
	// fr.endRecord replaces it with the real cause when one is known.
	fr.err = errForwardingAborted

	// Defer ordering: finish, then cleanup, then span.End.
	ctx, span := fr.handler.tracer.Start(r.Context(), "Proxy")
	defer span.End()
	ctx, cleanup := fr.handler.inflight.RequestContext(ctx)
	defer cleanup()
	// finish needs to run before cleanup cancels the ctx.
	defer fr.finish(ctx, release)
	defer func() {
		if fr.requestBody != nil {
			_ = fr.requestBody.Close()
		}
	}()
	r = r.WithContext(ctx)
	if !fr.start(ctx, w, r) {
		return
	}

	route := strings.TrimPrefix(r.URL.Path, "/"+fr.handler.provider.Name())
	fr.err = fr.handler.breaker.Execute(route, "", w, fr.forward)
	if fr.err != nil {
		span.SetStatus(codes.Error, "forwarding failed")
	}
}

// start validates and reads the request, extracts its facts, and records its
// start before forwarding. On rejection it writes the response and reports
// false.
func (fr *forwardedRequest) start(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	logger, record, cred := fr.handler.checkRequest(w, r)
	fr.logger = logger
	if record == nil {
		return false
	}

	if !fr.readRequest(ctx, w, r) {
		return false
	}
	record.ClientSessionID = aibclient.GuessSessionIDFromPayload(aibclient.Type(record.Client), r, fr.requestBody.Bytes())
	// Routes without an extractor yet are forwarded and recorded without the
	// facts it would add.
	if extractor := fr.handler.provider.CreateExtractor(r.URL.Path); extractor != nil {
		// Extraction failures only lose recording facts, the request is still
		// forwarded unchanged.
		if err := extractor.ExtractRequest(fr.requestBody.Bytes(), record); err != nil {
			logger.Warn(ctx, "failed to extract request facts", slog.Error(err))
		}
		fr.extractor = extractor
	}

	// The start record must exist before forwarding or recording its end.
	if err := fr.handler.recorder.RecordInterception(ctx, record); err != nil {
		trace.SpanFromContext(ctx).SetStatus(codes.Error, "failed to record interception")
		logger.Warn(ctx, "failed to record interception", slog.Error(err))
		// TODO: Return this error in the provider's JSON format. https://linear.app/codercom/issue/AIGOV-810
		http.Error(w, "failed to record interception", http.StatusInternalServerError)
		return false
	}
	fr.cred, fr.record = cred, record
	return true
}

// readRequest reads the complete request body through a replay capture, then
// rewinds the outbound request so forwarding sends the same bytes. It sets
// fr.outbound and fr.requestBody. On failure it writes the rejection and
// reports false.
func (fr *forwardedRequest) readRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	outbound, capture := replaybody.NewRequest(r)
	// Closing the capture unblocks a stalled body read, so cancellation,
	// including inflight gate shutdown, releases the request.
	stop := context.AfterFunc(ctx, func() { _ = capture.Close() })
	_, err := io.Copy(io.Discard, outbound.Body)
	stop()
	var replay io.ReadCloser
	if err == nil {
		// Rewind so forwarding sends the captured bytes.
		replay, err = outbound.GetBody()
	}
	if err != nil {
		_ = capture.Close()
		trace.SpanFromContext(ctx).SetStatus(codes.Error, "failed to read request body")
		if ctx.Err() != nil {
			fr.logger.Debug(ctx, "request canceled while reading its body", slog.Error(context.Cause(ctx)))
			// TODO: Return this error in the provider's JSON format. https://linear.app/codercom/issue/AIGOV-810
			http.Error(w, "request canceled", http.StatusServiceUnavailable)
			return false
		}
		//nolint:errcheck // AsType returns the match; only its presence matters.
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			fr.logger.Debug(ctx, "rejecting oversized request body", slog.Error(err))
			routing.WriteRequestBodyTooLarge(ctx, w)
			return false
		}
		fr.logger.Warn(ctx, "failed to read request body", slog.Error(err))
		// TODO: Return this error in the provider's JSON format. https://linear.app/codercom/issue/AIGOV-810
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return false
	}
	_ = outbound.Body.Close()
	outbound.Body = replay
	fr.outbound, fr.requestBody = outbound, capture
	return true
}

// finish records how the interception ended, then releases in-flight
// admission. It must run before cleanup cancels ctx.
func (fr *forwardedRequest) finish(ctx context.Context, release func()) {
	// No start record means the request was not forwarded, do not record an end.
	if fr.record == nil {
		release()
		return
	}
	ended := fr.endRecord(ctx)
	// Record asynchronously so the response finishes without waiting for the recorder.
	go fr.recordEnd(ctx, ended, release)
}

// endRecord describes how forwarding ended.
func (fr *forwardedRequest) endRecord(ctx context.Context) *recorder.InterceptionRecordEnded {
	// fr.err is what breaker.Execute returned, or errForwardingAborted if
	// forwarding panicked. A nil error still needs the response capture and
	// status to tell success from failure.
	err := fr.err

	// Prioritize context cancellation over the errForwardingAborted sentinel.
	if canceled := context.Cause(ctx); canceled != nil && errors.Is(err, errForwardingAborted) {
		err = canceled
	}

	var body []byte
	if fr.responseBody != nil {
		// fr.forward has closed the capture, so it has finished.
		readErr := fr.responseBody.Err()
		if errors.Is(readErr, replaybody.ErrCaptureInProgress) {
			fr.logger.Warn(ctx, "response capture still in progress when recording interception end", slog.F("interception_id", fr.record.ID))
		}
		body = fr.responseBody.Bytes()
		// Errors from the circuit breaker, reverse proxy, or cancellation are left
		// unchanged. A nil err, or the errForwardingAborted sentinel, takes the
		// capture's read error. The sentinel stays only if the capture completed but
		// the copy to the client aborted.
		if err == nil || (errors.Is(err, errForwardingAborted) && readErr != nil) {
			err = readErr
		}
	}

	// Categorize Go errors first, HTTP error responses may have no Go error.
	errType, errMsg := interceptionerror.Categorize(fr.handler.provider, err)
	if err == nil && fr.status >= http.StatusBadRequest {
		// TODO: provide SDK error to Categorize
		// https://linear.app/codercom/issue/AIGOV-811
		errType = recorder.ErrorTypeFromStatus(fr.status)
		errMsg = interceptionerror.TruncateMessage(string(body))
		if errMsg == "" {
			errMsg = http.StatusText(fr.status)
		}
	}

	// Keep the initial hint for BYOK, or use the last attempted pooled key.
	hint := fr.record.CredentialHint
	if fr.credentialHint != "" {
		hint = fr.credentialHint
	}
	return &recorder.InterceptionRecordEnded{
		ID:             fr.record.ID,
		EndedAt:        time.Now().UTC(),
		CredentialHint: hint,
		ErrorType:      errType,
		ErrorMessage:   errMsg,
	}
}

// recordEnd stores the end record, then releases in-flight admission.
func (fr *forwardedRequest) recordEnd(ctx context.Context, ended *recorder.InterceptionRecordEnded, release func()) {
	// Release in-flight gate admission after recording.
	defer release()

	// Do not propagate cancellation to the recorder.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recorder.DefaultAsyncTimeout)
	defer cancel()
	if err := fr.handler.recorder.RecordInterceptionEnded(ctx, ended); err != nil {
		fr.handler.logger.Warn(ctx, "failed to record interception end", slog.F("interception_id", ended.ID), slog.Error(err))
	}
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
		// The route's extractor sets Model, ClientSessionID, and
		// CorrelatingToolCallID, see forwardedRequest.start.
	}

	return logger, ir, cred
}

// forward proxies fr.outbound upstream and writes the final response after key
// failover to w. When no upstream response is accepted, it writes an error
// response and returns the cause.
func (fr *forwardedRequest) forward(w http.ResponseWriter) (err error) {
	defer func() {
		// Only the capture can close the upstream body. Close it to release the connection.
		if fr.responseBody != nil {
			_ = fr.responseBody.Close()
		}
	}()
	failover := fr.handler.failover
	if failover.Pool != nil {
		// TODO: key failover transport should provide the key hint and the key
		// pool error. https://linear.app/codercom/issue/AIGOV-795
		failover.InjectAuthKey = func(header *http.Header, key string) {
			fr.credentialHint = utils.MaskSecret(key)
			fr.handler.failover.InjectAuthKey(header, key)
		}
		failover.BuildKeyPoolResponse = func(keyErr *keypool.Error) *http.Response {
			err = keyErr
			return fr.handler.failover.BuildKeyPoolResponse(keyErr)
		}
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			rewriteForwardingURL(pr, fr.handler.provider.RoutePrefix(), fr.handler.baseURL)
			pr.Out.Header = prepareForwardingHeaders(pr.Out.Header, fr.cred)
		},
		Transport:     keypool.NewKeyFailoverTransport(fr.handler.transport, failover),
		FlushInterval: -1,
		// Only the final response returned by key failover is observed.
		ModifyResponse: func(resp *http.Response) error {
			if resp.StatusCode == http.StatusSwitchingProtocols {
				return xerrors.New("upstream protocol upgrades are not supported")
			}
			fr.status = resp.StatusCode
			fr.responseBody = replaybody.NewResponse(resp)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, proxyErr error) {
			err = proxyErr
			ctx := r.Context()
			fr.logger.Warn(ctx, "upstream proxy error", slog.Error(proxyErr))
			// TODO: Return this error in the provider's JSON format. https://linear.app/codercom/issue/AIGOV-810
			http.Error(w, "upstream proxy error", http.StatusBadGateway)
		},
		ErrorLog: fr.handler.errorLog,
	}
	proxy.ServeHTTP(w, fr.outbound)
	return err
}
