// Package dispatch delivers chat lifecycle events to an external webhook.
package dispatch

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
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/util/xnet"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
)

const (
	maxConcurrentDispatches = 256
	// Keep capacity reachable by work that a chat already admitted.
	maxAdmissionDispatches = 192
	maxResponseBodyBytes   = 1_048_576
	maxModelContextBytes   = 16_384
	capacityWaitLimit      = 250 * time.Millisecond
	retryBackoff           = 250 * time.Millisecond
	clockSkewLeeway        = 30 * time.Second
)

// CapacityClass selects which share of dispatch capacity an event draws from.
type CapacityClass int

const (
	CapacityClassUnset CapacityClass = iota
	CapacityClassAdmission
	CapacityClassGeneration
)

// Result classifies the terminal outcome of a dispatch attempt.
type Result string

const (
	ResultOK              Result = "ok"
	ResultDenied          Result = "denied"
	ResultHTTPError       Result = "http_error"
	ResultProtocolError   Result = "protocol_error"
	ResultTimeout         Result = "timeout"
	ResultConnectionError Result = "connection_error"
	ResultOverCapacity    Result = "over_capacity"
	ResultInternalError   Result = "internal_error"
)

// Event carries the identities delivered with each dispatch attempt.
type Event struct {
	Type agenthooks.EventType
	agenthooks.ChatRef
	Data     any
	Capacity CapacityClass
}

// Error preserves the attempt ID and failure class.
type Error struct {
	Class      Result
	DispatchID uuid.UUID
	Err        error
}

func (e *Error) Error() string {
	return e.Err.Error()
}

func (e *Error) Unwrap() error {
	return e.Err
}

func newError(class Result, dispatchID uuid.UUID, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Class: class, DispatchID: dispatchID, Err: err}
}

// Dispatcher delivers lifecycle hook attempts. It keeps no delivery or
// decision state, so delivery is best-effort and consumers own both.
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
	admission    chan struct{}
	metrics      *metrics
}

// validateHookURL requires HTTPS because hook traffic carries sensitive data
// and authorization tokens, and responses can control execution. Loopback HTTP
// is allowed by default; allowInsecure permits HTTP for any host.
//
//nolint:revive // allowInsecure is operator configuration, not caller control coupling.
func validateHookURL(raw string, allowInsecure bool) error {
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return xerrors.Errorf("parse hook URL: %w", err)
	}
	// The raw URL is signed verbatim as the JWT audience, and neither
	// component is ever transmitted, so a consumer configured with the URL
	// it actually serves would never match.
	if parsed.Fragment != "" {
		return xerrors.New("chat hook URL must not contain a fragment")
	}
	if parsed.User != nil {
		return xerrors.New("chat hook URL must not contain userinfo")
	}
	switch parsed.Scheme {
	case "https":
		if parsed.Hostname() == "" {
			return xerrors.New("chat hook URL must include a host")
		}
		return nil
	case "http":
		host := parsed.Hostname()
		if host == "" {
			return xerrors.New("chat hook URL must include a host")
		}
		if allowInsecure {
			return nil
		}
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
	allowInsecureURL bool,
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
		hookURLErr:   validateHookURL(hookURL, allowInsecureURL),
		secret:       []byte(secret),
		timeout:      timeout,
		deploymentID: deploymentID,
		userAgent:    "coderd-agenthooks/" + coderVersion,
		semaphore:    make(chan struct{}, maxConcurrentDispatches),
		admission:    make(chan struct{}, maxAdmissionDispatches),
		metrics:      newMetrics(reg),
	}
}

// Enabled reports whether a hook URL is configured. A nil dispatcher reads
// as disabled so callers can hold one unconditionally.
func (d *Dispatcher) Enabled() bool {
	return d != nil && d.hookURL != ""
}

// Dispatch delivers one event. The returned ID correlates the attempt in logs
// and Error values; the dispatcher does not persist delivery state.
func (d *Dispatcher) Dispatch(ctx context.Context, event Event) (agenthooks.Response, uuid.UUID, error) {
	if !d.Enabled() {
		return agenthooks.Response{}, uuid.Nil, xerrors.New("chat hook dispatcher is not enabled")
	}
	if d.hookURLErr != nil {
		return agenthooks.Response{}, uuid.Nil, xerrors.Errorf("chat hook URL rejected: %w", d.hookURLErr)
	}

	switch event.Capacity {
	case CapacityClassAdmission, CapacityClassGeneration:
	default:
		return agenthooks.Response{}, uuid.Nil, xerrors.Errorf("dispatch event %q has no capacity class", event.Type)
	}

	startedAt := time.Now()
	dispatchID := uuid.New()
	capacityDeadline := startedAt.Add(max(min(d.timeout, capacityWaitLimit), 0))

	release, refused, ok := d.acquireCapacity(ctx, event.Capacity, capacityDeadline)
	if !ok {
		return agenthooks.Response{}, dispatchID, d.finish(ctx, event, dispatchID, startedAt, refused)
	}
	defer release()

	// Both post attempts share whatever remains of the configured timeout so
	// that waiting for capacity cannot extend the dispatch past it.
	ctx, cancel := context.WithTimeout(ctx, d.timeout-time.Since(startedAt))
	defer cancel()

	outcome := d.prepareAndPost(ctx, event, dispatchID)
	if err := d.finish(ctx, event, dispatchID, startedAt, outcome); err != nil {
		return agenthooks.Response{}, dispatchID, err
	}
	return outcome.response, dispatchID, nil
}

// Admission acquires its gate before the shared pool so queued admission
// dispatches cannot occupy the reserved capacity.
func (d *Dispatcher) acquireCapacity(
	ctx context.Context,
	capacity CapacityClass,
	deadline time.Time,
) (release func(), outcome dispatchOutcome, ok bool) {
	if capacity == CapacityClassAdmission {
		releaseAdmission, refused, admitted := acquire(ctx, d.admission, deadline)
		if !admitted {
			return nil, refused, false
		}
		releaseShared, refused, acquired := acquire(ctx, d.semaphore, deadline)
		if !acquired {
			releaseAdmission()
			return nil, refused, false
		}
		return func() {
			releaseShared()
			releaseAdmission()
		}, dispatchOutcome{}, true
	}
	return acquire(ctx, d.semaphore, deadline)
}

// Check the deadline before select because select randomly chooses among ready
// cases, including a free slot and an expired timer.
func acquire(
	ctx context.Context,
	pool chan struct{},
	deadline time.Time,
) (release func(), outcome dispatchOutcome, ok bool) {
	overCapacity := dispatchOutcome{result: ResultOverCapacity, err: context.DeadlineExceeded}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return nil, overCapacity, false
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case pool <- struct{}{}:
		return func() { <-pool }, dispatchOutcome{}, true
	case <-ctx.Done():
		return nil, dispatchOutcome{result: ResultTimeout, err: ctx.Err()}, false
	case <-timer.C:
		return nil, overCapacity, false
	}
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
			slog.F("error", outcome.err.Error()),
		)
	} else {
		d.logger.Debug(context.WithoutCancel(ctx), "chat hook dispatched",
			slog.F("dispatch_id", dispatchID),
			slog.F("event", event.Type),
			slog.F("duration", time.Since(startedAt)),
		)
	}
	d.metrics.observe(event.Type, outcome.result, outcome.response, time.Since(startedAt))
	return newError(outcome.result, dispatchID, outcome.err)
}

type dispatchOutcome struct {
	result   Result
	response agenthooks.Response
	err      error
}

func (d *Dispatcher) prepareAndPost(ctx context.Context, event Event, dispatchID uuid.UUID) dispatchOutcome {
	data, err := marshalEventData(event)
	if err != nil {
		return dispatchOutcome{result: ResultProtocolError, err: err}
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
		return dispatchOutcome{result: ResultProtocolError, err: xerrors.Errorf("marshal request: %w", err)}
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
		return dispatchOutcome{result: ResultProtocolError, err: xerrors.Errorf("sign request: %w", err)}
	}

	response, result, err := d.post(ctx, body, token)
	outcome := dispatchOutcome{
		result:   result,
		response: response,
		err:      err,
	}
	if err != nil {
		return outcome
	}
	if err := validateResponse(event.Type, response); err != nil {
		// Drop the rejected response so its decision, override, and context
		// values are not observed as if they had been applied.
		outcome.response = agenthooks.Response{}
		outcome.result = ResultProtocolError
		outcome.err = err
		return outcome
	}
	if response.Permission != nil && response.Permission.Decision == agenthooks.PermissionDeny {
		outcome.result = ResultDenied
	}
	return outcome
}

func (d *Dispatcher) post(
	ctx context.Context,
	body []byte,
	token string,
) (response agenthooks.Response, result Result, err error) {
	// The deadline set in Dispatch bounds both attempts so a retry cannot
	// extend the dispatch past the configured timeout or the JWT lifetime.
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
			if xnet.IsTimeoutError(attemptErr) || xnet.IsTimeoutError(requestErr) || errors.Is(requestErr, context.Canceled) {
				return agenthooks.Response{}, ResultTimeout, xerrors.Errorf("post lifecycle hook: %w", requestErr)
			}
			if !xnet.IsConnectionError(requestErr) {
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
			case xnet.IsTimeoutError(attemptErr), xnet.IsTimeoutError(readErr), errors.Is(readErr, context.Canceled):
				return agenthooks.Response{}, ResultTimeout, xerrors.Errorf("read lifecycle hook response: %w", readErr)
			case xnet.IsConnectionError(readErr):
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
		if err := decodeResponse(trimmed, &response); err != nil {
			return agenthooks.Response{}, ResultProtocolError, xerrors.Errorf("decode lifecycle hook response: %w", err)
		}
		return response, ResultOK, nil
	}
	panic("unreachable")
}

// decodeResponse rejects response bodies that plain unmarshaling would
// silently misread as allow: unknown fields (misspelled keys), duplicate
// object keys (Go keeps the last value), and trailing JSON values.
func decodeResponse(trimmed []byte, response *agenthooks.Response) error {
	if err := rejectDuplicateKeys(json.NewDecoder(bytes.NewReader(trimmed)), 0, keyMatchingFolded); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(response); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return xerrors.New("response must contain one JSON object")
	}
	return nil
}

const maxResponseJSONDepth = 128

// foldJSONName canonicalizes an object key the way encoding/json matches
// struct fields: ASCII is case-folded, and other runes collapse to the
// smallest member of their Unicode simple-fold orbit. strings.ToLower is
// not equivalent, so "permi\u017f\u017fion" would otherwise slip past the
// duplicate check and overwrite "permission".
func foldJSONName(name string) string {
	folded := make([]byte, 0, len(name))
	for _, r := range name {
		if r < utf8.RuneSelf {
			if 'a' <= r && r <= 'z' {
				r -= 'a' - 'A'
			}
			folded = append(folded, byte(r))
			continue
		}
		for {
			next := unicode.SimpleFold(r)
			if next <= r {
				r = next
				break
			}
			r = next
		}
		folded = utf8.AppendRune(folded, r)
	}
	return string(folded)
}

// foldedInputOverride is the canonical form of the one envelope field whose
// contents are opaque pass-through data.
var foldedInputOverride = foldJSONName("input_override")

// keyMatching selects how object keys are compared for duplicates.
type keyMatching int

const (
	// keyMatchingFolded mirrors encoding/json struct-field matching, used
	// for the typed response envelope.
	keyMatchingFolded keyMatching = iota
	// keyMatchingExact is used inside input_override, which is opaque data
	// for a case-sensitive tool schema.
	keyMatchingExact
)

// rejectDuplicateKeys consumes one JSON value and fails on duplicate object
// keys at any depth, including inside input_override, because a duplicated
// key such as {"permission":{"decision":"deny"},"permission":null} would
// otherwise drop the decision the consumer intended. Envelope keys also
// collide when they differ only by folding, matching how encoding/json
// resolves struct fields, so "Permission" cannot override "permission".
//
// CLEANUP: json/v2 removes the need for this. It matches names case-sensitively
// and RejectDuplicateNames rejects exact duplicates.
func rejectDuplicateKeys(decoder *json.Decoder, depth int, matching keyMatching) error {
	if depth > maxResponseJSONDepth {
		return xerrors.New("response JSON exceeds supported nesting depth")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, _ := keyToken.(string)
			canonical := key
			if matching == keyMatchingFolded {
				canonical = foldJSONName(key)
			}
			if _, dup := seen[canonical]; dup {
				return xerrors.Errorf("duplicate key %q", key)
			}
			seen[canonical] = struct{}{}
			nested := matching
			// The decoder binds this field case-insensitively, so the
			// boundary must be detected the same way.
			if matching == keyMatchingFolded && canonical == foldedInputOverride {
				nested = keyMatchingExact
			}
			if err := rejectDuplicateKeys(decoder, depth+1, nested); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := rejectDuplicateKeys(decoder, depth+1, matching); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return nil
	}
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
		// Denied input does not proceed, so reject overrides to surface consumer bugs.
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
	// This override is decoded into a struct rather than passed through, so
	// it needs the same folded duplicate check as the response envelope.
	if err := rejectDuplicateKeys(json.NewDecoder(bytes.NewReader(input)), 0, keyMatchingFolded); err != nil {
		return xerrors.Errorf("user_prompt_submit input_override: %w", err)
	}
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

func (m *metrics) observe(eventType agenthooks.EventType, result Result, response agenthooks.Response, duration time.Duration) {
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
