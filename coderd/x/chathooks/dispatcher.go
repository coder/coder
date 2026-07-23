// Package chathooks dispatches chat lifecycle events to an external webhook.
package chathooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk/agenthooks"
)

const (
	maxConcurrentDispatches = 256
	maxResponseBodyBytes    = 1_048_576
	maxModelContextBytes    = 16_384
	capacityWaitLimit       = 250 * time.Millisecond
	retryBackoff            = 250 * time.Millisecond
	// clockSkewLeeway tolerates small clock differences with hook consumers.
	clockSkewLeeway = 30 * time.Second
)

// DispatchResult classifies the terminal outcome of a dispatch attempt.
type DispatchResult string

const (
	ResultOK              DispatchResult = "ok"
	ResultDenied          DispatchResult = "denied"
	ResultHTTPError       DispatchResult = "http_error"
	ResultProtocolError   DispatchResult = "protocol_error"
	ResultTimeout         DispatchResult = "timeout"
	ResultConnectionError DispatchResult = "connection_error"
	ResultOverCapacity    DispatchResult = "over_capacity"
	ResultInternalError   DispatchResult = "internal_error"
)

// Event carries the identities delivered with each dispatch attempt.
type Event struct {
	Type agenthooks.EventType
	agenthooks.ChatRef
	Data any
}

// DispatchError preserves the attempt ID and failure class.
type DispatchError struct {
	Class      DispatchResult
	DispatchID uuid.UUID
	Err        error
}

func (e *DispatchError) Error() string {
	return e.Err.Error()
}

func (e *DispatchError) Unwrap() error {
	return e.Err
}

func newDispatchError(class DispatchResult, dispatchID uuid.UUID, err error) error {
	if err == nil {
		return nil
	}
	return &DispatchError{Class: class, DispatchID: dispatchID, Err: err}
}

// Dispatcher delivers lifecycle hook attempts. It keeps no delivery decision
// state; delivery is at-least-once and consumers own decision state.
type Dispatcher struct {
	logger       slog.Logger
	client       *http.Client
	hookURL      string
	hookURLErr   error
	secret       []byte
	timeout      time.Duration
	deploymentID string
	userAgent    string
	semaphore    chan struct{}
	metrics      *metrics
}

// validateHookURL fails closed on cleartext hook URLs: the request
// carries prompt and tool data plus a bearer token, and the response
// controls allow/deny, so both need a confidential, authenticated
// channel. Plain HTTP is allowed only for loopback development
// consumers.
func validateHookURL(raw string) error {
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return xerrors.Errorf("parse hook URL: %w", err)
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		host := parsed.Hostname()
		if host == "localhost" {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
		return xerrors.New("chat hook URL must use HTTPS; plain HTTP is allowed only for loopback addresses")
	default:
		return xerrors.Errorf("chat hook URL scheme %q is not supported", parsed.Scheme)
	}
}

// New copies (or creates) the HTTP client and disables redirects for signed
// requests.
func New(
	logger slog.Logger,
	client *http.Client,
	hookURL string,
	secret string,
	timeout time.Duration,
	deploymentID string,
	coderVersion string,
	reg prometheus.Registerer,
) *Dispatcher {
	if client == nil {
		client = &http.Client{}
	} else {
		clientCopy := *client
		client = &clientCopy
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Dispatcher{
		logger:       logger.Named("chat_hook_dispatcher"),
		client:       client,
		hookURL:      hookURL,
		hookURLErr:   validateHookURL(hookURL),
		secret:       []byte(secret),
		timeout:      timeout,
		deploymentID: deploymentID,
		userAgent:    "coderd/" + coderVersion,
		semaphore:    make(chan struct{}, maxConcurrentDispatches),
		metrics:      newMetrics(reg),
	}
}

func (d *Dispatcher) Enabled() bool {
	return d != nil && d.hookURL != ""
}

// Dispatch delivers one event. The returned ID correlates the attempt in
// logs and DispatchError values; nothing is persisted.
func (d *Dispatcher) Dispatch(ctx context.Context, event Event) (agenthooks.Response, uuid.UUID, error) {
	if !d.Enabled() {
		return agenthooks.Response{}, uuid.Nil, xerrors.New("chat hook dispatcher is not enabled")
	}
	if d.hookURLErr != nil {
		return agenthooks.Response{}, uuid.Nil, xerrors.Errorf("chat hook URL rejected: %w", d.hookURLErr)
	}

	startedAt := time.Now()
	dispatchID := uuid.New()
	wait := min(d.timeout, capacityWaitLimit)
	if wait < 0 {
		wait = 0
	}
	capacityTimer := time.NewTimer(wait)
	defer capacityTimer.Stop()

	select {
	case d.semaphore <- struct{}{}:
		defer func() { <-d.semaphore }()
	case <-ctx.Done():
		outcome := dispatchOutcome{result: ResultTimeout, err: ctx.Err()}
		return agenthooks.Response{}, dispatchID, d.finish(ctx, event, dispatchID, startedAt, outcome)
	case <-capacityTimer.C:
		outcome := dispatchOutcome{result: ResultOverCapacity, err: context.DeadlineExceeded}
		return agenthooks.Response{}, dispatchID, d.finish(ctx, event, dispatchID, startedAt, outcome)
	}

	response, outcome := d.prepareAndPost(ctx, event, dispatchID)
	if err := d.finish(ctx, event, dispatchID, startedAt, outcome); err != nil {
		return agenthooks.Response{}, dispatchID, err
	}
	return response, dispatchID, nil
}

func (d *Dispatcher) finish(
	ctx context.Context,
	event Event,
	dispatchID uuid.UUID,
	startedAt time.Time,
	outcome dispatchOutcome,
) error {
	if outcome.err != nil {
		d.logger.Warn(context.WithoutCancel(ctx), "chat hook dispatch failed",
			slog.F("dispatch_id", dispatchID),
			slog.F("event", event.Type),
			slog.F("result", outcome.result),
			slog.Error(outcome.err),
		)
	} else {
		d.logger.Debug(context.WithoutCancel(ctx), "chat hook dispatched",
			slog.F("dispatch_id", dispatchID),
			slog.F("event", event.Type),
			slog.F("duration", time.Since(startedAt)),
		)
	}
	d.metrics.observe(event.Type, outcome.result, outcome.response, time.Since(startedAt))
	return newDispatchError(outcome.result, dispatchID, outcome.err)
}

type dispatchOutcome struct {
	result   DispatchResult
	response agenthooks.Response
	err      error
}

func (d *Dispatcher) prepareAndPost(ctx context.Context, event Event, dispatchID uuid.UUID) (agenthooks.Response, dispatchOutcome) {
	data, err := marshalEventData(event)
	if err != nil {
		return agenthooks.Response{}, dispatchOutcome{result: ResultProtocolError, err: err}
	}
	request := agenthooks.Request{
		Type: event.Type,
		Meta: agenthooks.Meta{
			DispatchID:    dispatchID,
			SchemaVersion: agenthooks.SchemaVersion,
			ChatRef:       event.ChatRef,
		},
		Data: data,
	}
	body, err := json.Marshal(request)
	if err != nil {
		return agenthooks.Response{}, dispatchOutcome{result: ResultProtocolError, err: xerrors.Errorf("marshal request: %w", err)}
	}

	digest := sha256.Sum256(body)
	now := time.Now()
	token, err := agenthooks.SignClaims(d.secret, agenthooks.Claims{
		Issuer:     d.deploymentID,
		Subject:    "coder:chat:" + event.ChatID.String(),
		Audience:   d.hookURL,
		IssuedAt:   now.Unix(),
		NotBefore:  now.Add(-clockSkewLeeway).Unix(),
		Expires:    now.Add(d.timeout + clockSkewLeeway).Unix(),
		JTI:        dispatchID,
		Type:       event.Type,
		BodySHA256: hex.EncodeToString(digest[:]),
	})
	if err != nil {
		return agenthooks.Response{}, dispatchOutcome{result: ResultProtocolError, err: xerrors.Errorf("sign request: %w", err)}
	}

	response, result, err := d.post(ctx, body, token)
	outcome := dispatchOutcome{
		result:   result,
		response: response,
		err:      err,
	}
	if err != nil {
		return agenthooks.Response{}, outcome
	}
	if err := validateResponse(event.Type, response); err != nil {
		outcome.result = ResultProtocolError
		outcome.err = err
		return agenthooks.Response{}, outcome
	}
	if response.Permission != nil && response.Permission.Decision == agenthooks.PermissionDeny {
		outcome.result = ResultDenied
	}
	return response, outcome
}

func (d *Dispatcher) post(
	ctx context.Context,
	body []byte,
	token string,
) (response agenthooks.Response, result DispatchResult, err error) {
	// One deadline bounds both attempts so a retry cannot extend the
	// dispatch past the configured timeout or the JWT lifetime.
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	for attempt := range 2 {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, d.hookURL, bytes.NewReader(body))
		if reqErr != nil {
			return agenthooks.Response{}, ResultProtocolError, xerrors.Errorf("create request: %w", reqErr)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", d.userAgent)

		httpResponse, requestErr := d.client.Do(req)
		if requestErr != nil {
			attemptErr := ctx.Err()
			if isTimeoutError(attemptErr) || isTimeoutError(requestErr) || errors.Is(requestErr, context.Canceled) {
				return agenthooks.Response{}, ResultTimeout, xerrors.Errorf("post lifecycle hook: %w", requestErr)
			}
			if !isConnectionError(requestErr) {
				return agenthooks.Response{}, ResultProtocolError, xerrors.Errorf("post lifecycle hook: %w", requestErr)
			}
			if attempt == 1 {
				return agenthooks.Response{}, ResultConnectionError, xerrors.Errorf("post lifecycle hook: %w", requestErr)
			}
			backoff := time.NewTimer(retryBackoff)
			select {
			case <-ctx.Done():
				backoff.Stop()
				return agenthooks.Response{}, ResultTimeout, xerrors.Errorf("post lifecycle hook: %w", ctx.Err())
			case <-backoff.C:
			}
			continue
		}

		if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
			_ = httpResponse.Body.Close()
			return agenthooks.Response{}, ResultHTTPError, xerrors.Errorf("lifecycle hook returned HTTP status %d", httpResponse.StatusCode)
		}

		responseBody, readErr := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBodyBytes+1))
		attemptErr := ctx.Err()
		_ = httpResponse.Body.Close()
		if readErr != nil {
			switch {
			case isTimeoutError(attemptErr), isTimeoutError(readErr), errors.Is(readErr, context.Canceled):
				return agenthooks.Response{}, ResultTimeout, xerrors.Errorf("read lifecycle hook response: %w", readErr)
			case isConnectionError(readErr):
				if attempt == 1 {
					return agenthooks.Response{}, ResultConnectionError, xerrors.Errorf("read lifecycle hook response: %w", readErr)
				}
			default:
				return agenthooks.Response{}, ResultProtocolError, xerrors.Errorf("read lifecycle hook response: %w", readErr)
			}
			// Mid-body connection drops get the same single retry as dial
			// failures, reusing the dispatch ID.
			backoff := time.NewTimer(retryBackoff)
			select {
			case <-ctx.Done():
				backoff.Stop()
				return agenthooks.Response{}, ResultTimeout, xerrors.Errorf("read lifecycle hook response: %w", ctx.Err())
			case <-backoff.C:
			}
			continue
		}
		if len(responseBody) > maxResponseBodyBytes {
			return agenthooks.Response{}, ResultProtocolError, xerrors.New("lifecycle hook response exceeds 1 MiB")
		}
		trimmed := bytes.TrimSpace(responseBody)
		if len(trimmed) == 0 {
			return agenthooks.Response{}, ResultOK, nil
		}
		if bytes.Equal(trimmed, []byte("null")) {
			return agenthooks.Response{}, ResultProtocolError, xerrors.New("lifecycle hook response must be a JSON object")
		}
		if err := json.Unmarshal(trimmed, &response); err != nil {
			return agenthooks.Response{}, ResultProtocolError, xerrors.Errorf("decode lifecycle hook response: %w", err)
		}
		return response, ResultOK, nil
	}
	panic("unreachable")
}

func validateResponse(eventType agenthooks.EventType, response agenthooks.Response) error {
	if len(response.ModelContext) > maxModelContextBytes {
		return xerrors.New("model_context exceeds 16 KiB")
	}
	if response.Permission == nil {
		return nil
	}
	if eventType != agenthooks.EventUserPromptSubmit && eventType != agenthooks.EventPreToolUse {
		return xerrors.Errorf("permission is not valid for event %q", eventType)
	}

	switch response.Permission.Decision {
	case agenthooks.PermissionAllow:
		inputOverride := bytes.TrimSpace(response.Permission.InputOverride)
		if len(inputOverride) == 0 || bytes.Equal(inputOverride, []byte("null")) {
			return xerrors.New("allow decision requires input_override")
		}
		if eventType == agenthooks.EventUserPromptSubmit {
			if err := validateUserPromptSubmitOverride(inputOverride); err != nil {
				return err
			}
		}
	case agenthooks.PermissionDeny:
		// A denied call never executes, so an override is meaningless;
		// reject it to surface consumer bugs.
		inputOverride := bytes.TrimSpace(response.Permission.InputOverride)
		if len(inputOverride) > 0 && !bytes.Equal(inputOverride, []byte("null")) {
			return xerrors.New("deny decision must not include input_override")
		}
	default:
		return xerrors.Errorf("invalid permission decision %q", response.Permission.Decision)
	}
	return nil
}

func validateUserPromptSubmitOverride(input json.RawMessage) error {
	var override struct {
		Prompt *string `json:"prompt"`
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&override); err != nil {
		return xerrors.Errorf("user_prompt_submit input_override must be {\"prompt\": string}: %w", err)
	}
	if override.Prompt == nil {
		return xerrors.New("user_prompt_submit input_override must be {\"prompt\": string}")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return xerrors.New("user_prompt_submit input_override must contain one JSON object")
	}
	return nil
}

func marshalEventData(event Event) (json.RawMessage, error) {
	switch event.Type {
	case agenthooks.EventSessionStart:
		if !isData[agenthooks.SessionStartData](event.Data) {
			return nil, xerrors.New("session_start data has the wrong type")
		}
	case agenthooks.EventUserPromptSubmit:
		if !isData[agenthooks.UserPromptSubmitData](event.Data) {
			return nil, xerrors.New("user_prompt_submit data has the wrong type")
		}
	case agenthooks.EventPreToolUse:
		value, ok := dataValue[agenthooks.PreToolUseData](event.Data)
		if !ok {
			return nil, xerrors.New("pre_tool_use data has the wrong type")
		}
		if value.ToolUseID == "" || value.ToolName == "" {
			return nil, xerrors.New("pre_tool_use data requires tool_use_id and tool_name")
		}
	case agenthooks.EventPostToolUse:
		value, ok := dataValue[agenthooks.PostToolUseData](event.Data)
		if !ok {
			return nil, xerrors.New("post_tool_use data has the wrong type")
		}
		if value.ToolUseID == "" || value.ToolName == "" {
			return nil, xerrors.New("post_tool_use data requires tool_use_id and tool_name")
		}
	case agenthooks.EventPreCompact:
		if !isData[agenthooks.PreCompactData](event.Data) {
			return nil, xerrors.New("pre_compact data has the wrong type")
		}
	case agenthooks.EventPostCompact:
		if !isData[agenthooks.PostCompactData](event.Data) {
			return nil, xerrors.New("post_compact data has the wrong type")
		}
	case agenthooks.EventStop:
		if !isData[agenthooks.StopData](event.Data) {
			return nil, xerrors.New("stop data has the wrong type")
		}
	default:
		return nil, xerrors.Errorf("unknown event type %q", event.Type)
	}

	encoded, marshalErr := json.Marshal(event.Data)
	if marshalErr != nil {
		return nil, xerrors.Errorf("marshal event data: %w", marshalErr)
	}
	return encoded, nil
}

func isData[T any](value any) bool {
	_, ok := dataValue[T](value)
	return ok
}

func dataValue[T any](value any) (T, bool) {
	if typed, ok := value.(T); ok {
		return typed, true
	}
	if typed, ok := value.(*T); ok && typed != nil {
		return *typed, true
	}
	var zero T
	return zero, false
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func isConnectionError(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr)
}

type metrics struct {
	dispatches     *prometheus.CounterVec
	duration       *prometheus.HistogramVec
	decisions      *prometheus.CounterVec
	contextSize    *prometheus.HistogramVec
	inputOverrides *prometheus.CounterVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	if reg == nil {
		reg = prometheus.NewRegistry()
	}
	factory := promauto.With(reg)
	return &metrics{
		dispatches: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "chatd",
			Name:      "hook_dispatches_total",
			Help:      "Total lifecycle hook dispatches by event and result.",
		}, []string{"event", "result"}),
		duration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "chatd",
			Name:      "hook_dispatch_seconds",
			Help:      "Lifecycle hook dispatch duration in seconds.",
		}, []string{"event"}),
		decisions: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "chatd",
			Name:      "hook_decisions_total",
			Help:      "Total lifecycle hook permission decisions by event and decision.",
		}, []string{"event", "decision"}),
		contextSize: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "chatd",
			Name:      "hook_context_size_bytes",
			Help:      "Lifecycle hook model context response size in bytes.",
			Buckets:   prometheus.ExponentialBuckets(64, 2, 10),
		}, []string{"event"}),
		inputOverrides: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "chatd",
			Name:      "hook_input_overrides_total",
			Help:      "Total lifecycle hook input overrides by event.",
		}, []string{"event"}),
	}
}

func (m *metrics) observe(eventType agenthooks.EventType, result DispatchResult, response agenthooks.Response, duration time.Duration) {
	event := string(eventType)
	m.dispatches.WithLabelValues(event, string(result)).Inc()
	m.duration.WithLabelValues(event).Observe(duration.Seconds())
	if response.ModelContext != "" {
		m.contextSize.WithLabelValues(event).Observe(float64(len(response.ModelContext)))
	}
	if response.Permission == nil {
		return
	}
	switch response.Permission.Decision {
	case agenthooks.PermissionAllow, agenthooks.PermissionDeny:
		m.decisions.WithLabelValues(event, string(response.Permission.Decision)).Inc()
	}
	if response.Permission.Decision == agenthooks.PermissionAllow && response.Permission.InputOverride != nil {
		m.inputOverrides.WithLabelValues(event).Inc()
	}
}
