package chaterror

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// ErrProviderTransportReset identifies provider stream cancellations that
// occur while the caller-owned chat context is still alive.
var ErrProviderTransportReset = xerrors.New("provider transport reset")

// ClassifiedError is the normalized, user-facing view of an
// underlying provider or runtime error.
type ClassifiedError struct {
	Message    string
	Detail     string
	Kind       codersdk.ChatErrorKind
	Provider   string
	Retryable  bool
	StatusCode int

	// RetryAfter is a normalized minimum retry delay derived from
	// provider response metadata when available.
	RetryAfter time.Duration
}

// http2PeerResetCause mirrors golang.org/x/net/http2's unexported
// errFromPeer message.
const http2PeerResetCause = "received from peer"

const responsesAPIDiagnosticMessage = "The chat continuation failed due to an " +
	"internal state mismatch. This is not a configuration or billing issue."

type responsesAPIDiagnosticMatch struct {
	pattern string
	detail  string
}

type streamIncompleteMatch struct {
	pattern  string
	provider string
}

// responsesAPIDiagnosticMatches maps provider error fragments to safe
// diagnostics. Details must not include provider item IDs because they are
// returned to clients and used by operators for grepping.
var responsesAPIDiagnosticMatches = []responsesAPIDiagnosticMatch{
	{
		pattern: "no tool output found for function call",
		detail:  "OpenAI Responses API request continuity diagnostic: match=function_call_output_missing.",
	},
	{
		pattern: "was provided without its required 'reasoning' item",
		detail:  "OpenAI Responses API request continuity diagnostic: match=web_search_reasoning_missing.",
	},
}

// streamIncompleteMatches maps provider stream-truncation errors from
// fantasy to clearer user-facing messages before broad EOF handling
// classifies them as generic transport timeouts.
var streamIncompleteMatches = []streamIncompleteMatch{
	{
		pattern:  "anthropic stream closed before message_stop",
		provider: "anthropic",
	},
	{
		pattern:  "openai responses stream closed before terminal event",
		provider: "openai",
	},
}

// WithProvider returns a copy of the classification using an explicit
// provider hint. Explicit provider hints are trusted over provider names
// heuristically parsed from the error text.
func (c ClassifiedError) WithProvider(provider string) ClassifiedError {
	hint := normalizeProvider(provider)
	if hint == "" {
		return normalizeClassification(c)
	}
	if c.Provider == hint && strings.TrimSpace(c.Message) != "" {
		return normalizeClassification(c)
	}
	updated := c
	updated.Provider = hint
	updated.Message = ""
	return normalizeClassification(updated)
}

// WithClassification wraps err so future calls to Classify return
// classified instead of re-deriving it from err.Error().
func WithClassification(err error, classified ClassifiedError) error {
	if err == nil {
		return nil
	}
	return &classifiedError{
		cause:      err,
		classified: normalizeClassification(classified),
	}
}

type classifiedError struct {
	cause      error
	classified ClassifiedError
}

func (e *classifiedError) Error() string {
	return e.cause.Error()
}

func (e *classifiedError) Unwrap() error {
	return e.cause
}

// Classify normalizes err into a stable, user-facing payload used for
// retry handling, streamed terminal errors, and persisted last_error
// values.
func Classify(err error) ClassifiedError {
	if err == nil {
		return ClassifiedError{}
	}

	var wrapped *classifiedError
	if errors.As(err, &wrapped) {
		return normalizeClassification(wrapped.classified)
	}

	structured := extractProviderErrorDetails(err)
	message := strings.TrimSpace(err.Error())
	if message == "" && structured.detail == "" && structured.statusCode == 0 && structured.retryAfter <= 0 {
		return ClassifiedError{}
	}

	lower := strings.ToLower(message)
	statusCode := structured.statusCode
	if statusCode == 0 {
		statusCode = extractStatusCode(lower)
	}
	provider := detectProvider(lower)
	canceled := errors.Is(err, context.Canceled)
	providerTransportReset := errors.Is(err, ErrProviderTransportReset)
	interrupted := containsAny(lower, interruptedPatterns...)
	if interrupted {
		return normalizeClassification(ClassifiedError{
			Message:    "The request was canceled before it completed.",
			Detail:     structured.detail,
			Kind:       codersdk.ChatErrorKindGeneric,
			Provider:   provider,
			StatusCode: statusCode,
			RetryAfter: structured.retryAfter,
		})
	}

	if detail, ok := responsesAPIDiagnostic(lower, structured.detail); ok {
		return normalizeClassification(ClassifiedError{
			Message:    responsesAPIDiagnosticMessage,
			Detail:     detail,
			Kind:       codersdk.ChatErrorKindGeneric,
			Provider:   provider,
			StatusCode: statusCode,
			RetryAfter: structured.retryAfter,
		})
	}

	if classified, ok := streamIncompleteClassification(
		lower,
		provider,
		statusCode,
		structured,
	); ok {
		return classified
	}

	retryableHTTP2StreamReset, hasHTTP2StreamReset := classifyHTTP2StreamReset(err)
	providerDisabledMatch := containsAny(lower, providerDisabledPatterns...)
	deadline := errors.Is(err, context.DeadlineExceeded) || strings.Contains(lower, "context deadline exceeded")
	overloadedMatch := statusCode == 529 || containsAny(lower, overloadedPatterns...)
	// Usage limits do not have a dedicated status code, so provider
	// response bodies can be the only reliable signal. Other classes
	// already have status-code signals or transport wrapper text.
	usageLimitText := lower + "\n" + strings.ToLower(structured.detail)
	usageLimitMatch := containsAny(usageLimitText, usageLimitAnyStatusPatterns...) ||
		(statusCode != 429 && containsAny(usageLimitText, usageLimitPatterns...))
	authStrong := statusCode == 401 || containsAny(lower, authStrongPatterns...)
	configMatch := containsAny(lower, configPatterns...)
	authWeak := statusCode == 403 || containsAny(lower, authWeakPatterns...)
	rateLimitMatch := statusCode == 429 || containsAny(lower, rateLimitPatterns...)
	timeoutPatternMatch := containsAny(lower, timeoutPatterns...)
	if hasHTTP2StreamReset && !retryableHTTP2StreamReset {
		// A typed HTTP/2 stream error gives us the reset code. Trust it
		// over broader string fallbacks so protocol bugs do not retry.
		timeoutPatternMatch = false
	}
	providerTransportResetMatch := providerTransportReset && statusCode == 0
	timeoutMatch := providerTransportResetMatch || deadline ||
		statusCode == 408 || statusCode == 502 || statusCode == 503 ||
		statusCode == 504 || retryableHTTP2StreamReset ||
		timeoutPatternMatch
	genericRetryableMatch := statusCode == 500 || containsAny(lower, genericRetryablePatterns...)

	// Config signals should beat ambiguous wrapper signals so
	// transient-looking errors like "503 invalid model" fail fast.
	// Overloaded stays ahead because 529/overloaded is a dedicated
	// provider saturation signal, not a common transport wrapper.
	// Usage-limit fires before auth so non-429 quota/billing text,
	// plus insufficient_quota at any status, wins over auth signals.
	// Strong auth still stays above config because bad credentials are
	// the root cause when both signals appear.
	// Provider-disabled must precede timeout because disabled providers
	// return 503, which matches the timeout rule.
	rules := []struct {
		match     bool
		kind      codersdk.ChatErrorKind
		retryable bool
	}{
		{
			match:     overloadedMatch,
			kind:      codersdk.ChatErrorKindOverloaded,
			retryable: true,
		},
		{
			match:     usageLimitMatch,
			kind:      codersdk.ChatErrorKindUsageLimit,
			retryable: false,
		},
		{
			match:     authStrong,
			kind:      codersdk.ChatErrorKindAuth,
			retryable: false,
		},
		{
			match:     authWeak && !configMatch,
			kind:      codersdk.ChatErrorKindAuth,
			retryable: false,
		},
		{
			match:     rateLimitMatch && !configMatch,
			kind:      codersdk.ChatErrorKindRateLimit,
			retryable: true,
		},
		{
			match:     providerDisabledMatch,
			kind:      codersdk.ChatErrorKindProviderDisabled,
			retryable: false,
		},
		{
			match:     timeoutMatch && !configMatch,
			kind:      codersdk.ChatErrorKindTimeout,
			retryable: !deadline,
		},
		{
			match:     configMatch,
			kind:      codersdk.ChatErrorKindConfig,
			retryable: false,
		},
		{
			match:     genericRetryableMatch,
			kind:      codersdk.ChatErrorKindGeneric,
			retryable: true,
		},
	}
	for _, rule := range rules {
		if !rule.match {
			continue
		}
		detail := structured.detail
		if rule.kind != codersdk.ChatErrorKindAuth {
			detail = resolveDiagnosticDetail(structured.detail, err)
		}
		return normalizeClassification(ClassifiedError{
			Detail:     detail,
			Kind:       rule.kind,
			Provider:   provider,
			Retryable:  rule.retryable,
			StatusCode: statusCode,
			RetryAfter: structured.retryAfter,
		})
	}

	if canceled {
		return normalizeClassification(ClassifiedError{
			Message:    "The request was canceled before it completed.",
			Detail:     structured.detail,
			Kind:       codersdk.ChatErrorKindGeneric,
			Provider:   provider,
			StatusCode: statusCode,
			RetryAfter: structured.retryAfter,
		})
	}

	return normalizeClassification(ClassifiedError{
		Detail:     resolveDiagnosticDetail(structured.detail, err),
		Kind:       codersdk.ChatErrorKindGeneric,
		Provider:   provider,
		StatusCode: statusCode,
		RetryAfter: structured.retryAfter,
	})
}

func classifyHTTP2StreamReset(err error) (retryable bool, found bool) {
	streamErr, ok := findHTTP2StreamError(err)
	if !ok {
		return false, false
	}
	if !isPeerHTTP2StreamError(streamErr) {
		return false, true
	}
	return isRetryableHTTP2StreamCode(streamErr.Code), true
}

func findHTTP2StreamError(err error) (http2.StreamError, bool) {
	var streamErr http2.StreamError
	if errors.As(err, &streamErr) {
		return streamErr, true
	}
	var streamErrPtr *http2.StreamError
	if errors.As(err, &streamErrPtr) && streamErrPtr != nil {
		return *streamErrPtr, true
	}
	return http2.StreamError{}, false
}

func isPeerHTTP2StreamError(streamErr http2.StreamError) bool {
	return streamErr.Cause != nil && streamErr.Cause.Error() == http2PeerResetCause
}

func isRetryableHTTP2StreamCode(code http2.ErrCode) bool {
	switch code {
	case http2.ErrCodeNo,
		http2.ErrCodeInternal,
		http2.ErrCodeRefusedStream,
		http2.ErrCodeCancel,
		http2.ErrCodeEnhanceYourCalm:
		return true
	default:
		return false
	}
}

func streamIncompleteClassification(
	lowerMessage string,
	provider string,
	statusCode int,
	structured providerErrorDetails,
) (ClassifiedError, bool) {
	for _, match := range streamIncompleteMatches {
		if !strings.Contains(lowerMessage, match.pattern) {
			continue
		}
		if provider == "" {
			provider = match.provider
		}
		return normalizeClassification(ClassifiedError{
			Message:    streamIncompleteMessage(provider),
			Detail:     structured.detail,
			Kind:       codersdk.ChatErrorKindTimeout,
			Provider:   provider,
			Retryable:  true,
			StatusCode: statusCode,
			RetryAfter: structured.retryAfter,
		}), true
	}
	return ClassifiedError{}, false
}

func streamIncompleteMessage(provider string) string {
	return providerSubject(provider) + " stream closed unexpectedly before the response completed."
}

func responsesAPIDiagnostic(lowerMessage, detail string) (string, bool) {
	lowerDetail := strings.ToLower(detail)
	for _, match := range responsesAPIDiagnosticMatches {
		if strings.Contains(lowerMessage, match.pattern) || strings.Contains(lowerDetail, match.pattern) {
			return match.detail, true
		}
	}
	return "", false
}

func normalizeClassification(classified ClassifiedError) ClassifiedError {
	classified.Message = strings.TrimSpace(classified.Message)
	classified.Detail = normalizeClassificationDetail(classified.Detail)
	classified.Kind = codersdk.ChatErrorKind(strings.TrimSpace(string(classified.Kind)))
	classified.Provider = normalizeProvider(classified.Provider)
	if classified.RetryAfter < 0 {
		classified.RetryAfter = 0
	}
	if classified.Kind == "" && classified.Message == "" {
		if classified.Detail == "" && classified.StatusCode == 0 &&
			classified.RetryAfter <= 0 {
			return ClassifiedError{}
		}
		classified.Kind = codersdk.ChatErrorKindGeneric
	}
	if classified.Kind == "" {
		classified.Kind = codersdk.ChatErrorKindGeneric
	}
	if classified.Message == "" {
		classified.Message = terminalMessage(classified)
	}
	return classified
}

const maxClassificationDetailRunes = 500

func normalizeClassificationDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return ""
	}
	runes := []rune(detail)
	if len(runes) <= maxClassificationDetailRunes {
		return detail
	}
	return string(runes[:maxClassificationDetailRunes-1]) + "…"
}
