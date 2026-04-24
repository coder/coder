package chaterror

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ClassifiedError is the normalized, user-facing view of an
// underlying provider or runtime error.
type ClassifiedError struct {
	Message    string
	Detail     string
	Kind       string
	Provider   string
	Retryable  bool
	StatusCode int

	// RetryAfter is a normalized minimum retry delay derived from
	// provider response metadata when available.
	RetryAfter time.Duration
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
			Kind:       KindGeneric,
			Provider:   provider,
			StatusCode: statusCode,
			RetryAfter: structured.retryAfter,
		})
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
		kind      string
		retryable bool
	}{
		{
			match:     overloadedMatch,
			kind:      KindOverloaded,
			retryable: true,
		},
		{
			match:     authStrong,
			kind:      KindAuth,
			retryable: false,
		},
		{
			match:     authWeak && !configMatch,
			kind:      KindAuth,
			retryable: false,
		},
		{
			match:     rateLimitMatch && !configMatch,
			kind:      KindRateLimit,
			retryable: true,
		},
		{
			match:     timeoutMatch && !configMatch,
			kind:      KindTimeout,
			retryable: !deadline,
		},
		{
			match:     configMatch,
			kind:      KindConfig,
			retryable: false,
		},
		{
			match:     genericRetryableMatch,
			kind:      KindGeneric,
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
		Kind:       KindGeneric,
		Provider:   provider,
		StatusCode: statusCode,
		RetryAfter: structured.retryAfter,
	})
}

func normalizeClassification(classified ClassifiedError) ClassifiedError {
	classified.Message = strings.TrimSpace(classified.Message)
	classified.Detail = normalizeClassificationDetail(classified.Detail)
	classified.Kind = strings.TrimSpace(classified.Kind)
	classified.Provider = normalizeProvider(classified.Provider)
	if classified.RetryAfter < 0 {
		classified.RetryAfter = 0
	}
	if classified.Kind == "" && classified.Message == "" {
		if classified.Detail == "" && classified.StatusCode == 0 &&
			classified.RetryAfter <= 0 {
			return ClassifiedError{}
		}
		classified.Kind = KindGeneric
	}
	if classified.Kind == "" {
		classified.Kind = KindGeneric
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
