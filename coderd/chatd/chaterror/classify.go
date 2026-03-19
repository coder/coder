package chaterror

import (
	"context"
	"errors"
	"strings"
)

// ClassifiedError is the normalized, user-facing view of an
// underlying provider or runtime error.
type ClassifiedError struct {
	Message    string
	Kind       string
	Provider   string
	Retryable  bool
	StatusCode int
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

	message := strings.TrimSpace(err.Error())
	if message == "" {
		return ClassifiedError{}
	}

	lower := strings.ToLower(message)
	statusCode := extractStatusCode(lower)
	provider := detectProvider(lower)
	canceled := errors.Is(err, context.Canceled) || strings.Contains(lower, "context canceled")
	interrupted := containsAny(lower, interruptedPatterns...)
	if canceled || interrupted {
		return normalizeClassification(ClassifiedError{
			Message:    "The request was canceled before it completed.",
			Kind:       KindGeneric,
			Provider:   provider,
			StatusCode: statusCode,
		})
	}

	deadline := errors.Is(err, context.DeadlineExceeded) || strings.Contains(lower, "context deadline exceeded")
	rules := []struct {
		match     bool
		kind      string
		retryable bool
	}{
		{
			match:     statusCode == 529 || containsAny(lower, overloadedPatterns...),
			kind:      KindOverloaded,
			retryable: true,
		},
		{
			match:     statusCode == 401 || statusCode == 403 || containsAny(lower, authPatterns...),
			kind:      KindAuth,
			retryable: false,
		},
		{
			match:     statusCode == 429 || containsAny(lower, rateLimitPatterns...),
			kind:      KindRateLimit,
			retryable: true,
		},
		{
			match: deadline || statusCode == 408 || statusCode == 502 ||
				statusCode == 503 || statusCode == 504 ||
				containsAny(lower, timeoutPatterns...),
			kind:      KindTimeout,
			retryable: !deadline,
		},
		{
			match:     containsAny(lower, configPatterns...),
			kind:      KindConfig,
			retryable: false,
		},
		{
			match:     statusCode == 500 || containsAny(lower, genericRetryablePatterns...),
			kind:      KindGeneric,
			retryable: true,
		},
	}
	for _, rule := range rules {
		if !rule.match {
			continue
		}
		return normalizeClassification(ClassifiedError{
			Kind:       rule.kind,
			Provider:   provider,
			Retryable:  rule.retryable,
			StatusCode: statusCode,
		})
	}

	return normalizeClassification(ClassifiedError{
		Kind:       KindGeneric,
		Provider:   provider,
		StatusCode: statusCode,
	})
}

func normalizeClassification(classified ClassifiedError) ClassifiedError {
	classified.Message = strings.TrimSpace(classified.Message)
	classified.Kind = strings.TrimSpace(classified.Kind)
	classified.Provider = normalizeProvider(classified.Provider)
	if classified.Kind == "" && classified.Message == "" {
		return ClassifiedError{}
	}
	if classified.Kind == "" {
		classified.Kind = KindGeneric
	}
	if classified.Message == "" {
		classified.Message = userFacingMessage(classified)
	}
	return classified
}
