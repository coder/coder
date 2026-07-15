// Package chathooks dispatches chat lifecycle events to an external webhook.
package chathooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk/agenthooks"
)

const (
	maxConcurrentDispatches = 256
	maxResponseBodyBytes    = 1 << 20
	maxModelContextBytes    = 16 << 10
	capacityWaitLimit       = 250 * time.Millisecond
	retryBackoff            = 250 * time.Millisecond
	finalizeTimeout         = 2 * time.Second
)

// Dispatches begin as pending, then finalize as ok, denied, http_error,
// protocol_error, timeout, connection_error, or over_capacity.
const (
	resultOK              = "ok"
	resultDenied          = "denied"
	resultHTTPError       = "http_error"
	resultProtocolError   = "protocol_error"
	resultTimeout         = "timeout"
	resultConnectionError = "connection_error"
	resultOverCapacity    = "over_capacity"
	resultInternalError   = "internal_error"
)

type store interface {
	InsertChatHookDispatch(context.Context, database.InsertChatHookDispatchParams) (database.ChatHookDispatch, error)
	FinalizeChatHookDispatch(context.Context, database.FinalizeChatHookDispatchParams) (database.ChatHookDispatch, error)
}

// Event contains a lifecycle event and its dispatch metadata.
type Event struct {
	Type        agenthooks.EventType
	ChatID      uuid.UUID
	OwnerID     uuid.UUID
	WorkspaceID *uuid.UUID
	TurnID      *uuid.UUID
	ToolUseID   *string
	Data        any
}

// DispatchError identifies a failed lifecycle hook dispatch.
type DispatchError struct {
	Class      string
	DispatchID uuid.UUID
	Err        error
}

func (e *DispatchError) Error() string {
	return e.Err.Error()
}

func (e *DispatchError) Unwrap() error {
	return e.Err
}

func newDispatchError(class string, dispatchID uuid.UUID, err error) error {
	if err == nil {
		return nil
	}
	return &DispatchError{Class: class, DispatchID: dispatchID, Err: err}
}

// Dispatcher posts lifecycle events to the configured webhook.
type Dispatcher struct {
	logger       slog.Logger
	db           store
	client       *http.Client
	hookURL      string
	secret       []byte
	timeout      time.Duration
	deploymentID string
	userAgent    string
	semaphore    chan struct{}
	metrics      *metrics
}

// New creates a lifecycle hook dispatcher.
func New(
	logger slog.Logger,
	db database.Store,
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
		db:           db,
		client:       client,
		hookURL:      hookURL,
		secret:       []byte(secret),
		timeout:      timeout,
		deploymentID: deploymentID,
		userAgent:    "coderd/" + coderVersion,
		semaphore:    make(chan struct{}, maxConcurrentDispatches),
		metrics:      newMetrics(reg),
	}
}

// Enabled reports whether the dispatcher is configured to send events.
func (d *Dispatcher) Enabled() bool {
	return d != nil && d.hookURL != ""
}

// Dispatch sends an event and returns the consumer response.
func (d *Dispatcher) Dispatch(ctx context.Context, event Event) (agenthooks.Response, error) {
	if !d.Enabled() {
		return agenthooks.Response{}, xerrors.New("chat hook dispatcher is not enabled")
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
		return d.finishWithoutPost(ctx, event, dispatchID, startedAt, resultTimeout, ctx.Err())
	case <-capacityTimer.C:
		return d.finishWithoutPost(ctx, event, dispatchID, startedAt, resultOverCapacity, context.DeadlineExceeded)
	}

	if err := d.insert(ctx, event, dispatchID, startedAt); err != nil {
		return agenthooks.Response{}, newDispatchError(resultInternalError, dispatchID, xerrors.Errorf("insert chat hook dispatch: %w", err))
	}

	response, outcome := d.prepareAndPost(ctx, event, dispatchID)
	finalizeErr := d.finalize(ctx, dispatchID, outcome)
	d.metrics.observe(event.Type, outcome.result, outcome.response, time.Since(startedAt))
	if finalizeErr != nil {
		d.logger.Error(context.WithoutCancel(ctx), "failed to finalize chat hook dispatch", slog.Error(finalizeErr))
		if outcome.err != nil {
			return agenthooks.Response{}, newDispatchError(outcome.result, dispatchID, errors.Join(outcome.err, finalizeErr))
		}
		return agenthooks.Response{}, newDispatchError(resultInternalError, dispatchID, finalizeErr)
	}
	return response, newDispatchError(outcome.result, dispatchID, outcome.err)
}

func (d *Dispatcher) finishWithoutPost(
	ctx context.Context,
	event Event,
	dispatchID uuid.UUID,
	startedAt time.Time,
	result string,
	dispatchErr error,
) (agenthooks.Response, error) {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer cancel()
	if err := d.insert(persistCtx, event, dispatchID, startedAt); err != nil {
		return agenthooks.Response{}, newDispatchError(result, dispatchID, errors.Join(dispatchErr, xerrors.Errorf("insert chat hook dispatch: %w", err)))
	}
	outcome := dispatchOutcome{result: result, err: dispatchErr}
	finalizeErr := d.finalize(ctx, dispatchID, outcome)
	d.metrics.observe(event.Type, result, agenthooks.Response{}, time.Since(startedAt))
	if finalizeErr != nil {
		return agenthooks.Response{}, newDispatchError(result, dispatchID, errors.Join(dispatchErr, finalizeErr))
	}
	return agenthooks.Response{}, newDispatchError(result, dispatchID, dispatchErr)
}

func (d *Dispatcher) insert(ctx context.Context, event Event, dispatchID uuid.UUID, startedAt time.Time) error {
	_, err := d.db.InsertChatHookDispatch(ctx, database.InsertChatHookDispatchParams{
		ID:          dispatchID,
		ChatID:      event.ChatID,
		Event:       string(event.Type),
		TurnID:      nullUUID(event.TurnID),
		ToolUseID:   nullToolUseID(event.ToolUseID),
		OwnerID:     event.OwnerID,
		WorkspaceID: nullUUID(event.WorkspaceID),
		StartedAt:   startedAt,
	})
	return err
}

type dispatchOutcome struct {
	result     string
	httpStatus sql.NullInt32
	response   agenthooks.Response
	original   pqtype.NullRawMessage
	err        error
}

func (d *Dispatcher) prepareAndPost(ctx context.Context, event Event, dispatchID uuid.UUID) (agenthooks.Response, dispatchOutcome) {
	data, original, err := marshalEventData(event)
	if err != nil {
		return agenthooks.Response{}, dispatchOutcome{result: resultProtocolError, original: original, err: err}
	}
	request := agenthooks.Request{
		Type: event.Type,
		Meta: agenthooks.Meta{
			DispatchID:    dispatchID,
			SchemaVersion: agenthooks.SchemaVersion,
			ChatID:        event.ChatID,
			OwnerID:       event.OwnerID,
			WorkspaceID:   event.WorkspaceID,
			TurnID:        event.TurnID,
		},
		Data: data,
	}
	body, err := json.Marshal(request)
	if err != nil {
		return agenthooks.Response{}, dispatchOutcome{result: resultProtocolError, original: original, err: xerrors.Errorf("marshal request: %w", err)}
	}

	digest := sha256.Sum256(body)
	now := time.Now()
	token, err := agenthooks.SignClaims(d.secret, agenthooks.Claims{
		Issuer:     d.deploymentID,
		Subject:    "coder:chat:" + event.ChatID.String(),
		Audience:   d.hookURL,
		IssuedAt:   now.Unix(),
		NotBefore:  now.Unix(),
		Expires:    now.Add(d.timeout + 30*time.Second).Unix(),
		JTI:        dispatchID,
		Type:       event.Type,
		BodySHA256: hex.EncodeToString(digest[:]),
	})
	if err != nil {
		return agenthooks.Response{}, dispatchOutcome{result: resultProtocolError, original: original, err: xerrors.Errorf("sign request: %w", err)}
	}

	response, status, result, err := d.post(ctx, body, token)
	outcome := dispatchOutcome{
		result:     result,
		httpStatus: status,
		response:   response,
		original:   original,
		err:        err,
	}
	if err != nil {
		return agenthooks.Response{}, outcome
	}
	if err := validateResponse(event.Type, response); err != nil {
		outcome.result = resultProtocolError
		outcome.err = err
		return agenthooks.Response{}, outcome
	}
	if response.Permission != nil && response.Permission.Decision == agenthooks.PermissionDeny {
		outcome.result = resultDenied
	}
	return response, outcome
}

func (d *Dispatcher) post(
	ctx context.Context,
	body []byte,
	token string,
) (response agenthooks.Response, status sql.NullInt32, result string, err error) {
	for attempt := range 2 {
		attemptCtx, cancel := context.WithTimeout(ctx, d.timeout)
		req, reqErr := http.NewRequestWithContext(attemptCtx, http.MethodPost, d.hookURL, bytes.NewReader(body))
		if reqErr != nil {
			cancel()
			return agenthooks.Response{}, sql.NullInt32{}, resultProtocolError, xerrors.Errorf("create request: %w", reqErr)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", d.userAgent)

		httpResponse, requestErr := d.client.Do(req)
		if requestErr != nil {
			attemptErr := attemptCtx.Err()
			cancel()
			if isTimeoutError(attemptErr) || isTimeoutError(requestErr) || errors.Is(requestErr, context.Canceled) {
				return agenthooks.Response{}, sql.NullInt32{}, resultTimeout, xerrors.Errorf("post lifecycle hook: %w", requestErr)
			}
			if !isConnectionError(requestErr) {
				return agenthooks.Response{}, sql.NullInt32{}, resultConnectionError, xerrors.Errorf("post lifecycle hook: %w", requestErr)
			}
			if attempt == 1 {
				return agenthooks.Response{}, sql.NullInt32{}, resultConnectionError, xerrors.Errorf("post lifecycle hook: %w", requestErr)
			}
			backoff := time.NewTimer(retryBackoff)
			select {
			case <-ctx.Done():
				backoff.Stop()
				return agenthooks.Response{}, sql.NullInt32{}, resultTimeout, xerrors.Errorf("post lifecycle hook: %w", ctx.Err())
			case <-backoff.C:
			}
			continue
		}

		statusCode := int64(httpResponse.StatusCode)
		if statusCode < math.MinInt32 || statusCode > math.MaxInt32 {
			_ = httpResponse.Body.Close()
			cancel()
			return agenthooks.Response{}, sql.NullInt32{}, resultProtocolError, xerrors.Errorf("lifecycle hook returned invalid HTTP status %d", httpResponse.StatusCode)
		}
		status = sql.NullInt32{Int32: int32(statusCode), Valid: true}
		if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
			_ = httpResponse.Body.Close()
			cancel()
			return agenthooks.Response{}, status, resultHTTPError, xerrors.Errorf("lifecycle hook returned HTTP status %d", httpResponse.StatusCode)
		}

		responseBody, readErr := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBodyBytes+1))
		attemptErr := attemptCtx.Err()
		_ = httpResponse.Body.Close()
		cancel()
		if readErr != nil {
			switch {
			case isTimeoutError(attemptErr), isTimeoutError(readErr), errors.Is(readErr, context.Canceled):
				return agenthooks.Response{}, status, resultTimeout, xerrors.Errorf("read lifecycle hook response: %w", readErr)
			case isConnectionError(readErr):
				return agenthooks.Response{}, status, resultConnectionError, xerrors.Errorf("read lifecycle hook response: %w", readErr)
			default:
				return agenthooks.Response{}, status, resultProtocolError, xerrors.Errorf("read lifecycle hook response: %w", readErr)
			}
		}
		if len(responseBody) > maxResponseBodyBytes {
			return agenthooks.Response{}, status, resultProtocolError, xerrors.New("lifecycle hook response exceeds 1 MiB")
		}
		trimmed := bytes.TrimSpace(responseBody)
		if len(trimmed) == 0 {
			return agenthooks.Response{}, status, resultOK, nil
		}
		if bytes.Equal(trimmed, []byte("null")) {
			return agenthooks.Response{}, status, resultProtocolError, xerrors.New("lifecycle hook response must be a JSON object")
		}
		if err := json.Unmarshal(trimmed, &response); err != nil {
			return agenthooks.Response{}, status, resultProtocolError, xerrors.Errorf("decode lifecycle hook response: %w", err)
		}
		return response, status, resultOK, nil
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
		return nil
	case agenthooks.PermissionAsk:
		return xerrors.New("ask decision is not supported")
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

func marshalEventData(event Event) (data json.RawMessage, original pqtype.NullRawMessage, err error) {
	switch event.Type {
	case agenthooks.EventSessionStart:
		if !isData[agenthooks.SessionStartData](event.Data) {
			return nil, original, xerrors.New("session_start data has the wrong type")
		}
	case agenthooks.EventUserPromptSubmit:
		value, ok := dataValue[agenthooks.UserPromptSubmitData](event.Data)
		if !ok {
			return nil, original, xerrors.New("user_prompt_submit data has the wrong type")
		}
		encoded, marshalErr := json.Marshal(value.Prompt)
		if marshalErr != nil {
			return nil, original, xerrors.Errorf("marshal original prompt: %w", marshalErr)
		}
		original = pqtype.NullRawMessage{RawMessage: encoded, Valid: true}
	case agenthooks.EventPreToolUse:
		value, ok := dataValue[agenthooks.PreToolUseData](event.Data)
		if !ok {
			return nil, original, xerrors.New("pre_tool_use data has the wrong type")
		}
		original = pqtype.NullRawMessage{RawMessage: bytes.Clone(value.ToolInput), Valid: true}
	case agenthooks.EventPostToolUse:
		if !isData[agenthooks.PostToolUseData](event.Data) {
			return nil, original, xerrors.New("post_tool_use data has the wrong type")
		}
	case agenthooks.EventPreCompact:
		if !isData[agenthooks.PreCompactData](event.Data) {
			return nil, original, xerrors.New("pre_compact data has the wrong type")
		}
	case agenthooks.EventPostCompact:
		if !isData[agenthooks.PostCompactData](event.Data) {
			return nil, original, xerrors.New("post_compact data has the wrong type")
		}
	case agenthooks.EventStop:
		if !isData[agenthooks.StopData](event.Data) {
			return nil, original, xerrors.New("stop data has the wrong type")
		}
	default:
		return nil, original, xerrors.Errorf("unknown event type %q", event.Type)
	}

	encoded, marshalErr := json.Marshal(event.Data)
	if marshalErr != nil {
		return nil, original, xerrors.Errorf("marshal event data: %w", marshalErr)
	}
	return encoded, original, nil
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

func (d *Dispatcher) finalize(ctx context.Context, dispatchID uuid.UUID, outcome dispatchOutcome) error {
	response := outcome.response
	var decision sql.NullString
	var inputOverride pqtype.NullRawMessage
	if response.Permission != nil {
		decision = sql.NullString{String: string(response.Permission.Decision), Valid: true}
		if response.Permission.InputOverride != nil {
			inputOverride = pqtype.NullRawMessage{RawMessage: bytes.Clone(response.Permission.InputOverride), Valid: true}
		}
	}
	allowedTools := pqtype.NullRawMessage{}
	if response.AllowedTools != nil {
		encoded, err := json.Marshal(response.AllowedTools)
		if err != nil {
			return xerrors.Errorf("marshal allowed tools: %w", err)
		}
		allowedTools = pqtype.NullRawMessage{RawMessage: encoded, Valid: true}
	}

	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer cancel()
	_, err := d.db.FinalizeChatHookDispatch(finalizeCtx, database.FinalizeChatHookDispatchParams{
		FinishedAt:    time.Now(),
		Result:        outcome.result,
		HttpStatus:    outcome.httpStatus,
		Decision:      decision,
		InputOverride: inputOverride,
		OriginalInput: outcome.original,
		ModelContext:  nullString(response.ModelContext),
		UserMessage:   nullString(response.UserMessage),
		AllowedTools:  allowedTools,
		EndChat:       sql.NullBool{Bool: response.EndChat, Valid: response.EndChat},
		Error:         nullError(outcome.err),
		ID:            dispatchID,
	})
	if err != nil {
		return xerrors.Errorf("finalize chat hook dispatch: %w", err)
	}
	return nil
}

func nullUUID(value *uuid.UUID) uuid.NullUUID {
	if value == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func nullToolUseID(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func nullError(err error) sql.NullString {
	if err == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: err.Error(), Valid: true}
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

func (m *metrics) observe(eventType agenthooks.EventType, result string, response agenthooks.Response, duration time.Duration) {
	event := string(eventType)
	m.dispatches.WithLabelValues(event, result).Inc()
	m.duration.WithLabelValues(event).Observe(duration.Seconds())
	if response.ModelContext != "" {
		m.contextSize.WithLabelValues(event).Observe(float64(len(response.ModelContext)))
	}
	if response.Permission == nil {
		return
	}
	switch response.Permission.Decision {
	case agenthooks.PermissionAllow, agenthooks.PermissionDeny, agenthooks.PermissionAsk:
		m.decisions.WithLabelValues(event, string(response.Permission.Decision)).Inc()
	}
	if response.Permission.Decision == agenthooks.PermissionAllow && response.Permission.InputOverride != nil {
		m.inputOverrides.WithLabelValues(event).Inc()
	}
}
