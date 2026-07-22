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
	maxResponseBodyBytes    = 1_048_576
	maxModelContextBytes    = 16_384
	capacityWaitLimit       = 250 * time.Millisecond
	retryBackoff            = 250 * time.Millisecond
	finalizeTimeout         = 2 * time.Second
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

type store interface {
	InsertChatHookDispatch(context.Context, database.InsertChatHookDispatchParams) (database.ChatHookDispatch, error)
	FinalizeChatHookDispatch(context.Context, database.FinalizeChatHookDispatchParams) (database.ChatHookDispatch, error)
}

// Event carries the identities persisted with each delivery attempt.
type Event struct {
	Type         agenthooks.EventType
	ChatID       uuid.UUID
	OwnerID      uuid.UUID
	WorkspaceID  *uuid.UUID
	TurnID       *uuid.UUID
	ParentChatID *uuid.UUID
	RootChatID   *uuid.UUID
	Data         any
}

func (e Event) toolMetadata() (toolUseID, toolName *string) {
	switch e.Type {
	case agenthooks.EventPreToolUse:
		if data, ok := dataValue[agenthooks.PreToolUseData](e.Data); ok {
			return &data.ToolUseID, &data.ToolName
		}
	case agenthooks.EventPostToolUse:
		if data, ok := dataValue[agenthooks.PostToolUseData](e.Data); ok {
			return &data.ToolUseID, &data.ToolName
		}
	}
	return nil, nil
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

// Dispatcher persists and delivers lifecycle hook attempts.
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

// New copies (or creates) the HTTP client and disables redirects for signed
// requests.
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

func (d *Dispatcher) Enabled() bool {
	return d != nil && d.hookURL != ""
}

// Dispatch persists and delivers one event. The returned ID identifies the
// dispatch so effects can bind to a single attempt.
func (d *Dispatcher) Dispatch(ctx context.Context, event Event) (agenthooks.Response, uuid.UUID, error) {
	if !d.Enabled() {
		return agenthooks.Response{}, uuid.Nil, xerrors.New("chat hook dispatcher is not enabled")
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
		response, err := d.finishWithoutPost(ctx, event, dispatchID, startedAt, ResultTimeout, ctx.Err())
		return response, dispatchID, err
	case <-capacityTimer.C:
		response, err := d.finishWithoutPost(ctx, event, dispatchID, startedAt, ResultOverCapacity, context.DeadlineExceeded)
		return response, dispatchID, err
	}

	// Insert with a detached context so a caller canceled right after
	// acquiring capacity still leaves an audit row, matching finishWithoutPost.
	insertCtx, cancelInsert := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	insertErr := d.insert(insertCtx, event, dispatchID, startedAt)
	cancelInsert()
	if insertErr != nil {
		d.metrics.observe(event.Type, ResultInternalError, agenthooks.Response{}, time.Since(startedAt))
		return agenthooks.Response{}, dispatchID, newDispatchError(ResultInternalError, dispatchID, xerrors.Errorf("insert chat hook dispatch: %w", insertErr))
	}

	response, outcome := d.prepareAndPost(ctx, event, dispatchID)
	finalizeErr := d.finalize(ctx, event, dispatchID, outcome)
	d.metrics.observe(event.Type, outcome.result, outcome.response, time.Since(startedAt))
	if finalizeErr != nil {
		d.logger.Error(context.WithoutCancel(ctx), "failed to finalize chat hook dispatch", slog.Error(finalizeErr))
		if outcome.err != nil {
			return agenthooks.Response{}, dispatchID, newDispatchError(outcome.result, dispatchID, errors.Join(outcome.err, finalizeErr))
		}
		return agenthooks.Response{}, dispatchID, newDispatchError(ResultInternalError, dispatchID, finalizeErr)
	}
	return response, dispatchID, newDispatchError(outcome.result, dispatchID, outcome.err)
}

func (d *Dispatcher) finishWithoutPost(
	ctx context.Context,
	event Event,
	dispatchID uuid.UUID,
	startedAt time.Time,
	result DispatchResult,
	dispatchErr error,
) (agenthooks.Response, error) {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer cancel()
	if err := d.insert(persistCtx, event, dispatchID, startedAt); err != nil {
		return agenthooks.Response{}, newDispatchError(result, dispatchID, errors.Join(dispatchErr, xerrors.Errorf("insert chat hook dispatch: %w", err)))
	}
	outcome := dispatchOutcome{result: result, err: dispatchErr}
	finalizeErr := d.finalize(ctx, event, dispatchID, outcome)
	d.metrics.observe(event.Type, result, agenthooks.Response{}, time.Since(startedAt))
	if finalizeErr != nil {
		return agenthooks.Response{}, newDispatchError(result, dispatchID, errors.Join(dispatchErr, finalizeErr))
	}
	return agenthooks.Response{}, newDispatchError(result, dispatchID, dispatchErr)
}

func (d *Dispatcher) insert(ctx context.Context, event Event, dispatchID uuid.UUID, startedAt time.Time) error {
	toolUseID, toolName := event.toolMetadata()
	_, err := d.db.InsertChatHookDispatch(ctx, database.InsertChatHookDispatchParams{
		ID:          dispatchID,
		ChatID:      event.ChatID,
		Event:       string(event.Type),
		TurnID:      nullUUID(event.TurnID),
		ToolUseID:   nullStringPtr(toolUseID),
		ToolName:    nullStringPtr(toolName),
		OwnerID:     event.OwnerID,
		WorkspaceID: nullUUID(event.WorkspaceID),
		StartedAt:   startedAt,
	})
	return err
}

type dispatchOutcome struct {
	result     DispatchResult
	httpStatus sql.NullInt32
	response   agenthooks.Response
	original   pqtype.NullRawMessage
	err        error
}

func (d *Dispatcher) prepareAndPost(ctx context.Context, event Event, dispatchID uuid.UUID) (agenthooks.Response, dispatchOutcome) {
	data, original, err := marshalEventData(event)
	if err != nil {
		return agenthooks.Response{}, dispatchOutcome{result: ResultProtocolError, original: original, err: err}
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
			ParentChatID:  event.ParentChatID,
			RootChatID:    event.RootChatID,
		},
		Data: data,
	}
	body, err := json.Marshal(request)
	if err != nil {
		return agenthooks.Response{}, dispatchOutcome{result: ResultProtocolError, original: original, err: xerrors.Errorf("marshal request: %w", err)}
	}

	digest := sha256.Sum256(body)
	now := time.Now()
	token, err := agenthooks.SignClaims(d.secret, agenthooks.Claims{
		Issuer:   d.deploymentID,
		Subject:  "coder:chat:" + event.ChatID.String(),
		Audience: d.hookURL,
		IssuedAt: now.Unix(),
		// Backdate nbf to tolerate consumer clock skew.
		NotBefore:  now.Add(-clockSkewLeeway).Unix(),
		Expires:    now.Add(d.timeout + clockSkewLeeway).Unix(),
		JTI:        dispatchID,
		Type:       event.Type,
		BodySHA256: hex.EncodeToString(digest[:]),
	})
	if err != nil {
		return agenthooks.Response{}, dispatchOutcome{result: ResultProtocolError, original: original, err: xerrors.Errorf("sign request: %w", err)}
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
) (response agenthooks.Response, status sql.NullInt32, result DispatchResult, err error) {
	for attempt := range 2 {
		attemptCtx, cancel := context.WithTimeout(ctx, d.timeout)
		req, reqErr := http.NewRequestWithContext(attemptCtx, http.MethodPost, d.hookURL, bytes.NewReader(body))
		if reqErr != nil {
			cancel()
			return agenthooks.Response{}, sql.NullInt32{}, ResultProtocolError, xerrors.Errorf("create request: %w", reqErr)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", d.userAgent)

		httpResponse, requestErr := d.client.Do(req)
		if requestErr != nil {
			attemptErr := attemptCtx.Err()
			cancel()
			if isTimeoutError(attemptErr) || isTimeoutError(requestErr) || errors.Is(requestErr, context.Canceled) {
				return agenthooks.Response{}, sql.NullInt32{}, ResultTimeout, xerrors.Errorf("post lifecycle hook: %w", requestErr)
			}
			if !isConnectionError(requestErr) {
				return agenthooks.Response{}, sql.NullInt32{}, ResultProtocolError, xerrors.Errorf("post lifecycle hook: %w", requestErr)
			}
			if attempt == 1 {
				return agenthooks.Response{}, sql.NullInt32{}, ResultConnectionError, xerrors.Errorf("post lifecycle hook: %w", requestErr)
			}
			backoff := time.NewTimer(retryBackoff)
			select {
			case <-ctx.Done():
				backoff.Stop()
				return agenthooks.Response{}, sql.NullInt32{}, ResultTimeout, xerrors.Errorf("post lifecycle hook: %w", ctx.Err())
			case <-backoff.C:
			}
			continue
		}

		statusCode := int64(httpResponse.StatusCode)
		if statusCode < math.MinInt32 || statusCode > math.MaxInt32 {
			_ = httpResponse.Body.Close()
			cancel()
			return agenthooks.Response{}, sql.NullInt32{}, ResultProtocolError, xerrors.Errorf("lifecycle hook returned invalid HTTP status %d", httpResponse.StatusCode)
		}
		status = sql.NullInt32{Int32: int32(statusCode), Valid: true}
		if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
			_ = httpResponse.Body.Close()
			cancel()
			return agenthooks.Response{}, status, ResultHTTPError, xerrors.Errorf("lifecycle hook returned HTTP status %d", httpResponse.StatusCode)
		}

		responseBody, readErr := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBodyBytes+1))
		attemptErr := attemptCtx.Err()
		_ = httpResponse.Body.Close()
		cancel()
		if readErr != nil {
			switch {
			case isTimeoutError(attemptErr), isTimeoutError(readErr), errors.Is(readErr, context.Canceled):
				return agenthooks.Response{}, status, ResultTimeout, xerrors.Errorf("read lifecycle hook response: %w", readErr)
			case isConnectionError(readErr):
				if attempt == 1 {
					return agenthooks.Response{}, status, ResultConnectionError, xerrors.Errorf("read lifecycle hook response: %w", readErr)
				}
			default:
				return agenthooks.Response{}, status, ResultProtocolError, xerrors.Errorf("read lifecycle hook response: %w", readErr)
			}
			// Mid-body connection drops get the same single retry as dial
			// failures, reusing the dispatch ID.
			backoff := time.NewTimer(retryBackoff)
			select {
			case <-ctx.Done():
				backoff.Stop()
				return agenthooks.Response{}, status, ResultTimeout, xerrors.Errorf("read lifecycle hook response: %w", ctx.Err())
			case <-backoff.C:
			}
			continue
		}
		if len(responseBody) > maxResponseBodyBytes {
			return agenthooks.Response{}, status, ResultProtocolError, xerrors.New("lifecycle hook response exceeds 1 MiB")
		}
		trimmed := bytes.TrimSpace(responseBody)
		if len(trimmed) == 0 {
			return agenthooks.Response{}, status, ResultOK, nil
		}
		if bytes.Equal(trimmed, []byte("null")) {
			return agenthooks.Response{}, status, ResultProtocolError, xerrors.New("lifecycle hook response must be a JSON object")
		}
		if err := json.Unmarshal(trimmed, &response); err != nil {
			return agenthooks.Response{}, status, ResultProtocolError, xerrors.Errorf("decode lifecycle hook response: %w", err)
		}
		return response, status, ResultOK, nil
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
		// A persisted deny override would poison decision reuse, which
		// matches future inputs against original_input OR input_override.
		inputOverride := bytes.TrimSpace(response.Permission.InputOverride)
		if len(inputOverride) > 0 && !bytes.Equal(inputOverride, []byte("null")) {
			return xerrors.New("deny decision must not include input_override")
		}
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
		if value.ToolUseID == "" || value.ToolName == "" {
			return nil, original, xerrors.New("pre_tool_use data requires tool_use_id and tool_name")
		}
		// original_input is a jsonb column, so malformed model output
		// cannot be persisted there. Leave it NULL so the dispatch can
		// still finalize as a protocol error instead of staying pending.
		if json.Valid(value.ToolInput) {
			original = pqtype.NullRawMessage{RawMessage: bytes.Clone(value.ToolInput), Valid: true}
		}
	case agenthooks.EventPostToolUse:
		value, ok := dataValue[agenthooks.PostToolUseData](event.Data)
		if !ok {
			return nil, original, xerrors.New("post_tool_use data has the wrong type")
		}
		if value.ToolUseID == "" || value.ToolName == "" {
			return nil, original, xerrors.New("post_tool_use data requires tool_use_id and tool_name")
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

func (d *Dispatcher) finalize(ctx context.Context, event Event, dispatchID uuid.UUID, outcome dispatchOutcome) error {
	response := outcome.response
	if outcome.err != nil {
		d.logger.Warn(context.WithoutCancel(ctx), "chat hook dispatch failed",
			slog.F("dispatch_id", dispatchID),
			slog.F("event", event.Type),
			slog.F("result", outcome.result),
			slog.Error(outcome.err),
		)
	}
	// Only accepted outcomes may persist reusable decisions.
	accepted := outcome.result == ResultOK || outcome.result == ResultDenied
	var decision sql.NullString
	var inputOverride pqtype.NullRawMessage
	if response.Permission != nil && accepted {
		decision = sql.NullString{String: string(response.Permission.Decision), Valid: true}
		if response.Permission.InputOverride != nil {
			inputOverride = pqtype.NullRawMessage{RawMessage: bytes.Clone(response.Permission.InputOverride), Valid: true}
		}
	} else if event.Type == agenthooks.EventPreToolUse && outcome.result == ResultOK {
		decision = sql.NullString{String: string(agenthooks.PermissionAllow), Valid: true}
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
		FinishedAt:     time.Now(),
		Result:         string(outcome.result),
		HttpStatus:     outcome.httpStatus,
		Decision:       decision,
		DecisionReason: nullPermissionReason(response.Permission),
		InputOverride:  inputOverride,
		OriginalInput:  outcome.original,
		ModelContext:   nullString(response.ModelContext),
		UserMessage:    nullString(response.UserMessage),
		AllowedTools:   allowedTools,
		EndChat:        sql.NullBool{Bool: response.EndChat, Valid: response.EndChat},
		Error:          nullError(outcome.err),
		ID:             dispatchID,
		ChatID:         event.ChatID,
		OwnerID:        event.OwnerID,
	})
	if err != nil {
		return xerrors.Errorf("finalize chat hook dispatch: %w", err)
	}
	return nil
}

func nullPermissionReason(permission *agenthooks.Permission) sql.NullString {
	if permission == nil || permission.Reason == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: permission.Reason, Valid: true}
}

func nullUUID(value *uuid.UUID) uuid.NullUUID {
	if value == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func nullStringPtr(value *string) sql.NullString {
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
	case agenthooks.PermissionAllow, agenthooks.PermissionDeny, agenthooks.PermissionAsk:
		m.decisions.WithLabelValues(event, string(response.Permission.Decision)).Inc()
	}
	if response.Permission.Decision == agenthooks.PermissionAllow && response.Permission.InputOverride != nil {
		m.inputOverrides.WithLabelValues(event).Inc()
	}
}
