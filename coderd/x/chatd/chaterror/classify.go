package chaterror

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/coder/coder/v2/codersdk"
)

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

	// ChainBroken is true when the provider reported that the
	// previous_response_id (or analogous chain anchor) is no longer
	// retrievable. The chatloop retry path uses this signal to exit
	// chain mode and replay full history before the next attempt.
	// This is an internal signal; it is not surfaced as a separate
	// codersdk.ChatErrorKind so the user-visible kind set stays
	// stable.
	ChainBroken bool
}

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

type chainBrokenMatch struct {
	// pattern is a lowercase substring required in the error message.
	pattern string
	// requiredAdditional is a second lowercase substring that must
	// also be present. Empty when a single substring is unambiguous.
	requiredAdditional string
	// provider is the provider hint applied when none was detected
	// from the wrapped error.
	provider string
}

// chainBrokenMatches maps provider error fragments that indicate the
// chain anchor (OpenAI previous_response_id, or analogous future
// signals) is no longer retrievable. Recovery is to clear the chain
// state and retry with full history.
//
// Patterns must be specific enough to not catch unrelated 404s; we
// require a co-occurring substring where a single fragment would be
// ambiguous.
var chainBrokenMatches = []chainBrokenMatch{
	{
		pattern:            "previous response with id",
		requiredAdditional: "not found",
		provider:           "openai",
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
	canceled := errors.Is(err, context.Canceled) || strings.Contains(lower, "context canceled")
	interrupted := containsAny(lower, interruptedPatterns...)
	if canceled || interrupted {
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

	// Chain-broken detection runs before the generic rule table so a
	// 404 carrying a chain anchor failure is not classified as a
	// generic non-retryable error. The chatloop retry callback uses
	// the ChainBroken flag to exit chain mode and replay full
	// history.
	if classified, ok := chainBrokenClassification(
		lower,
		provider,
		statusCode,
		structured,
	); ok {
		return classified
	}

	deadline := errors.Is(err, context.DeadlineExceeded) || strings.Contains(lower, "context deadline exceeded")
	overloadedMatch := statusCode == 529 || containsAny(lower, overloadedPatterns...)
	authStrong := statusCode == 401 || containsAny(lower, authStrongPatterns...)
	configMatch := containsAny(lower, configPatterns...)
	authWeak := statusCode == 403 || containsAny(lower, authWeakPatterns...)
	rateLimitMatch := statusCode == 429 || containsAny(lower, rateLimitPatterns...)
	timeoutMatch := deadline || statusCode == 408 || statusCode == 502 ||
		statusCode == 503 || statusCode == 504 ||
		containsAny(lower, timeoutPatterns...)
	genericRetryableMatch := statusCode == 500 || containsAny(lower, genericRetryablePatterns...)

	// Config signals should beat ambiguous wrapper signals so
	// transient-looking errors like "503 invalid model" fail fast.
	// Overloaded stays ahead because 529/overloaded is a dedicated
	// provider saturation signal, not a common transport wrapper.
	// Strong auth still stays above config because bad credentials are
	// the root cause when both signals appear.
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
		return normalizeClassification(ClassifiedError{
			Detail:     structured.detail,
			Kind:       rule.kind,
			Provider:   provider,
			Retryable:  rule.retryable,
			StatusCode: statusCode,
			RetryAfter: structured.retryAfter,
		})
	}

	return normalizeClassification(ClassifiedError{
		Detail:     structured.detail,
		Kind:       codersdk.ChatErrorKindGeneric,
		Provider:   provider,
		StatusCode: statusCode,
		RetryAfter: structured.retryAfter,
	})
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

func chainBrokenClassification(
	lowerMessage string,
	provider string,
	statusCode int,
	structured providerErrorDetails,
) (ClassifiedError, bool) {
	for _, match := range chainBrokenMatches {
		if !strings.Contains(lowerMessage, match.pattern) {
			continue
		}
		if match.requiredAdditional != "" &&
			!strings.Contains(lowerMessage, match.requiredAdditional) {
			continue
		}
		if provider == "" {
			provider = match.provider
		}
		return normalizeClassification(ClassifiedError{
			Detail:      structured.detail,
			Kind:        codersdk.ChatErrorKindGeneric,
			Provider:    provider,
			Retryable:   true,
			StatusCode:  statusCode,
			RetryAfter:  structured.retryAfter,
			ChainBroken: true,
		}), true
	}
	return ClassifiedError{}, false
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
