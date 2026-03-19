package chaterror

import (
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

// WithProvider applies an explicit provider hint to a classification.
func WithProvider(classified ClassifiedError, provider string) ClassifiedError {
	return classified.WithProvider(provider)
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
	if e == nil || e.cause == nil {
		return ""
	}
	return e.cause.Error()
}

func (e *classifiedError) Unwrap() error {
	if e == nil {
		return nil
	}
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
	if errors.As(err, &wrapped) && wrapped != nil {
		return normalizeClassification(wrapped.classified)
	}

	message := strings.TrimSpace(err.Error())
	if message == "" {
		return ClassifiedError{}
	}

	signals := collectSignals(err, message)

	switch {
	case signals.canceled || signals.interrupted:
		return normalizeClassification(ClassifiedError{
			Message:    "The request was canceled before it completed.",
			Kind:       KindGeneric,
			Provider:   signals.provider,
			Retryable:  false,
			StatusCode: signals.statusCode,
		})
	case signals.overloaded:
		return normalizeClassification(ClassifiedError{
			Kind:       KindOverloaded,
			Provider:   signals.provider,
			Retryable:  true,
			StatusCode: signals.statusCode,
		})
	case signals.auth:
		return normalizeClassification(ClassifiedError{
			Kind:       KindAuth,
			Provider:   signals.provider,
			Retryable:  false,
			StatusCode: signals.statusCode,
		})
	case signals.rateLimit:
		return normalizeClassification(ClassifiedError{
			Kind:       KindRateLimit,
			Provider:   signals.provider,
			Retryable:  true,
			StatusCode: signals.statusCode,
		})
	case signals.deadline || signals.timeout:
		return normalizeClassification(ClassifiedError{
			Kind:       KindTimeout,
			Provider:   signals.provider,
			Retryable:  !signals.deadline,
			StatusCode: signals.statusCode,
		})
	case signals.config:
		return normalizeClassification(ClassifiedError{
			Kind:       KindConfig,
			Provider:   signals.provider,
			Retryable:  false,
			StatusCode: signals.statusCode,
		})
	case signals.genericRetryable:
		return normalizeClassification(ClassifiedError{
			Kind:       KindGeneric,
			Provider:   signals.provider,
			Retryable:  true,
			StatusCode: signals.statusCode,
		})
	default:
		return normalizeClassification(ClassifiedError{
			Kind:       KindGeneric,
			Provider:   signals.provider,
			Retryable:  false,
			StatusCode: signals.statusCode,
		})
	}
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
