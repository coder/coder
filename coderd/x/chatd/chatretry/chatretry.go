// Package chatretry provides retry logic for transient LLM provider
// errors. It classifies errors as retryable or permanent and
// implements exponential backoff matching the behavior of coder/mux.
package chatretry

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

const (
	// InitialDelay is the backoff duration for the first retry
	// attempt.
	InitialDelay = 1 * time.Second

	// MaxDelay is the upper bound for the exponential backoff
	// duration. Matches the cap used in coder/mux.
	MaxDelay = 60 * time.Second

	// MaxAttempts is the upper bound on retry attempts before
	// giving up. With a 60s max backoff this allows roughly
	// 25 minutes of retries, which is reasonable for transient
	// LLM provider issues.
	MaxAttempts = 25
)

const (
	errorKindOverloaded = "overloaded"
	errorKindRateLimit  = "rate_limit"
	errorKindTimeout    = "timeout"
	errorKindAuth       = "auth"
	errorKindConfig     = "config"
	errorKindGeneric    = "generic"
)

var (
	statusCodePattern       = regexp.MustCompile(`(?i)(?:status(?:\s+code)?|http)\s*[:=]?\s*(\d{3})`)
	standaloneStatusPattern = regexp.MustCompile(`\b(?:401|403|408|429|500|502|503|504|529)\b`)
)

var overloadedPatterns = []string{
	"overloaded",
	"overloaded_error",
}

var rateLimitPatterns = []string{
	"rate limit",
	"rate_limit",
	"rate limited",
	"rate-limited",
	"too many requests",
}

var timeoutPatterns = []string{
	"timeout",
	"timed out",
	"service unavailable",
	"unavailable",
	"connection reset",
	"connection refused",
	"eof",
	"broken pipe",
	"bad gateway",
	"gateway timeout",
}

var authPatterns = []string{
	"authentication",
	"unauthorized",
	"forbidden",
	"invalid api key",
	"invalid_api_key",
	"quota",
	"billing",
	"insufficient_quota",
	"payment required",
}

var configPatterns = []string{
	"invalid model",
	"model not found",
	"model_not_found",
	"unsupported model",
	"context length exceeded",
	"context_exceeded",
	"maximum context length",
	"malformed config",
	"malformed configuration",
}

var genericRetryablePatterns = []string{
	"server error",
	"internal server error",
}

// ClassifiedError is the normalized, user-facing view of an
// underlying provider or runtime error.
type ClassifiedError struct {
	Message    string
	Kind       string
	Provider   string
	Retryable  bool
	StatusCode int
}

// WithProvider returns a copy of the classification with Provider
// filled when the classifier could not infer one from the error text.
func (c ClassifiedError) WithProvider(provider string) ClassifiedError {
	if strings.TrimSpace(c.Provider) != "" {
		return c
	}
	c.Provider = normalizeProvider(provider)
	return normalizeClassification(c)
}

// WithClassification wraps err so future calls to ClassifyError return
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

// ClassifyError normalizes err into a stable, user-facing payload used
// for retry handling, streamed terminal errors, and persisted last_error
// values.
func ClassifyError(err error) ClassifiedError {
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

	lower := strings.ToLower(message)
	statusCode := extractStatusCode(lower)
	provider := extractProvider(lower, statusCode)

	if errors.Is(err, context.Canceled) || strings.Contains(lower, "context canceled") {
		return normalizeClassification(ClassifiedError{
			Message:    "The request was canceled before it completed.",
			Kind:       errorKindGeneric,
			Provider:   provider,
			Retryable:  false,
			StatusCode: statusCode,
		})
	}

	if errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(lower, "context deadline exceeded") {
		return normalizeClassification(ClassifiedError{
			Kind:       errorKindTimeout,
			Provider:   provider,
			Retryable:  false,
			StatusCode: statusCode,
		})
	}

	switch {
	case isOverloaded(lower, statusCode):
		return normalizeClassification(ClassifiedError{
			Kind:       errorKindOverloaded,
			Provider:   provider,
			Retryable:  true,
			StatusCode: statusCode,
		})
	case isConfig(lower):
		return normalizeClassification(ClassifiedError{
			Kind:       errorKindConfig,
			Provider:   provider,
			Retryable:  false,
			StatusCode: statusCode,
		})
	case isAuth(lower, statusCode):
		return normalizeClassification(ClassifiedError{
			Kind:       errorKindAuth,
			Provider:   provider,
			Retryable:  false,
			StatusCode: statusCode,
		})
	case isRateLimit(lower, statusCode):
		return normalizeClassification(ClassifiedError{
			Kind:       errorKindRateLimit,
			Provider:   provider,
			Retryable:  true,
			StatusCode: statusCode,
		})
	case isTimeout(lower, statusCode):
		return normalizeClassification(ClassifiedError{
			Kind:       errorKindTimeout,
			Provider:   provider,
			Retryable:  true,
			StatusCode: statusCode,
		})
	case isGenericRetryable(lower, statusCode):
		return normalizeClassification(ClassifiedError{
			Kind:       errorKindGeneric,
			Provider:   provider,
			Retryable:  true,
			StatusCode: statusCode,
		})
	default:
		return normalizeClassification(ClassifiedError{
			Kind:       errorKindGeneric,
			Provider:   provider,
			Retryable:  false,
			StatusCode: statusCode,
		})
	}
}

// IsRetryable determines whether an error from an LLM provider is
// transient and worth retrying.
func IsRetryable(err error) bool {
	return ClassifyError(err).Retryable
}

// StatusCodeRetryable returns true for HTTP status codes that
// indicate a transient failure worth retrying.
func StatusCodeRetryable(code int) bool {
	switch code {
	case 408, 429, 500, 502, 503, 504, 529:
		return true
	default:
		return false
	}
}

// Delay returns the backoff duration for the given 0-indexed attempt.
// Uses exponential backoff: min(InitialDelay * 2^attempt, MaxDelay).
// Matches the backoff curve used in coder/mux.
func Delay(attempt int) time.Duration {
	d := InitialDelay
	for range attempt {
		d *= 2
		if d >= MaxDelay {
			return MaxDelay
		}
	}
	return d
}

// RetryFn is the function to retry. It receives a context and returns
// an error. The context may be a child of the original with adjusted
// deadlines for individual attempts.
type RetryFn func(ctx context.Context) error

// OnRetryFn is called before each retry attempt with the attempt
// number (1-indexed), the raw error that triggered the retry, the
// normalized error payload, and the delay before the next attempt.
type OnRetryFn func(attempt int, err error, classified ClassifiedError, delay time.Duration)

// Retry calls fn repeatedly until it succeeds, returns a
// non-retryable error, ctx is canceled, or MaxAttempts is reached.
// Retries use exponential backoff capped at MaxDelay.
//
// The onRetry callback (if non-nil) is called before each retry
// attempt, giving the caller a chance to reset state, log, or
// publish status events.
func Retry(ctx context.Context, fn RetryFn, onRetry OnRetryFn) error {
	var attempt int
	for {
		err := fn(ctx)
		if err == nil {
			return nil
		}

		classified := ClassifyError(err)
		if !classified.Retryable {
			return WithClassification(err, classified)
		}

		// If the caller's context is already done, return the
		// context error so cancellation propagates cleanly.
		if ctx.Err() != nil {
			return ctx.Err()
		}

		attempt++
		if attempt >= MaxAttempts {
			return WithClassification(
				xerrors.Errorf("max retry attempts (%d) exceeded: %w", MaxAttempts, err),
				classified,
			)
		}

		delay := Delay(attempt - 1)

		if onRetry != nil {
			onRetry(attempt, err, classified, delay)
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
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
		classified.Kind = errorKindGeneric
	}
	if classified.Message == "" {
		classified.Message = userFacingMessage(classified)
	}
	return classified
}

func userFacingMessage(classified ClassifiedError) string {
	switch classified.Kind {
	case errorKindOverloaded:
		return overloadedMessage(classified.Provider, classified.StatusCode)
	case errorKindRateLimit:
		return rateLimitMessage(classified.Provider, classified.StatusCode)
	case errorKindTimeout:
		if classified.StatusCode > 0 {
			return timeoutStatusMessage(classified.Provider, classified.StatusCode)
		}
		if classified.Retryable {
			return retryableTimeoutMessage(classified.Provider)
		}
		return timeoutMessage()
	case errorKindAuth:
		return authMessage(classified.Provider)
	case errorKindConfig:
		return configMessage(classified.Provider)
	default:
		if classified.StatusCode > 0 {
			if classified.Retryable {
				return retryableGenericStatusMessage(
					classified.Provider,
					classified.StatusCode,
				)
			}
			return genericStatusMessage(classified.Provider, classified.StatusCode)
		}
		if classified.Retryable {
			return retryableGenericMessage(classified.Provider)
		}
		return genericMessage()
	}
}

func overloadedMessage(provider string, statusCode int) string {
	subject := providerSubject(provider)
	if statusCode > 0 {
		return fmt.Sprintf(
			"%s is temporarily overloaded (HTTP %d). Please try again later.",
			subject,
			statusCode,
		)
	}
	return fmt.Sprintf("%s is temporarily overloaded. Please try again later.", subject)
}

func rateLimitMessage(provider string, statusCode int) string {
	subject := providerSubject(provider)
	if statusCode > 0 {
		return fmt.Sprintf(
			"%s is rate limiting requests (HTTP %d). Please try again later.",
			subject,
			statusCode,
		)
	}
	return fmt.Sprintf("%s is rate limiting requests. Please try again later.", subject)
}

func timeoutStatusMessage(provider string, statusCode int) string {
	subject := providerSubject(provider)
	return fmt.Sprintf(
		"%s is temporarily unavailable (HTTP %d). Please try again later.",
		subject,
		statusCode,
	)
}

func retryableTimeoutMessage(provider string) string {
	subject := providerSubject(provider)
	return fmt.Sprintf("%s did not respond in time. Please try again.", subject)
}

func timeoutMessage() string {
	return "The request timed out before it completed. Please try again."
}

func authMessage(provider string) string {
	if displayName := providerDisplayName(provider); displayName != "" {
		return fmt.Sprintf(
			"Authentication with %s failed. Check the API key, permissions, and billing settings.",
			displayName,
		)
	}
	return "Authentication with the AI provider failed. Check the API key, permissions, and billing settings."
}

func configMessage(provider string) string {
	subject := providerSubject(provider)
	return fmt.Sprintf(
		"%s rejected the model configuration. Check the selected model and provider settings.",
		subject,
	)
}

func retryableGenericStatusMessage(provider string, statusCode int) string {
	subject := providerSubject(provider)
	return fmt.Sprintf(
		"%s returned an unexpected error (HTTP %d). Please try again later.",
		subject,
		statusCode,
	)
}

func genericStatusMessage(provider string, statusCode int) string {
	subject := providerSubject(provider)
	return fmt.Sprintf(
		"%s returned an unexpected error (HTTP %d). Please try again.",
		subject,
		statusCode,
	)
}

func retryableGenericMessage(provider string) string {
	subject := providerSubject(provider)
	return fmt.Sprintf("%s returned an unexpected error. Please try again later.", subject)
}

func genericMessage() string {
	return "The chat request failed unexpectedly. Please try again."
}

func providerSubject(provider string) string {
	if displayName := providerDisplayName(provider); displayName != "" {
		return displayName
	}
	return "The AI provider"
}

func providerDisplayName(provider string) string {
	switch normalizeProvider(provider) {
	case "anthropic":
		return "Anthropic"
	case "azure":
		return "Azure OpenAI"
	case "bedrock":
		return "AWS Bedrock"
	case "google":
		return "Google"
	case "openai":
		return "OpenAI"
	case "openai-compat":
		return "OpenAI Compatible"
	case "openrouter":
		return "OpenRouter"
	case "vercel":
		return "Vercel AI Gateway"
	default:
		return ""
	}
}

func normalizeProvider(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	switch normalized {
	case "azure openai", "azure-openai":
		return "azure"
	case "openai compat", "openai compatible", "openai_compat":
		return "openai-compat"
	default:
		return normalized
	}
}

func extractStatusCode(lower string) int {
	if matches := statusCodePattern.FindStringSubmatch(lower); len(matches) == 2 {
		return atoi(matches[1])
	}
	if match := standaloneStatusPattern.FindString(lower); match != "" {
		return atoi(match)
	}
	switch {
	case containsAny(lower, "too many requests"):
		return 429
	case containsAny(lower, "unauthorized", "invalid api key", "invalid_api_key"):
		return 401
	case containsAny(lower, "forbidden"):
		return 403
	case containsAny(lower, "gateway timeout"):
		return 504
	case containsAny(lower, "bad gateway"):
		return 502
	case containsAny(lower, "service unavailable"):
		return 503
	case containsAny(lower, "overloaded"):
		return 529
	default:
		return 0
	}
}

func extractProvider(lower string, statusCode int) string {
	switch {
	case containsAny(lower, "openai-compat", "openai compatible"):
		return "openai-compat"
	case containsAny(lower, "azure openai", "azure-openai"):
		return "azure"
	case containsAny(lower, "openrouter"):
		return "openrouter"
	case containsAny(lower, "aws bedrock", "bedrock"):
		return "bedrock"
	case containsAny(lower, "vercel ai gateway", "vercel"):
		return "vercel"
	case containsAny(lower, "anthropic", "claude"):
		return "anthropic"
	case containsAny(lower, "google", "gemini", "vertex"):
		return "google"
	case containsAny(lower, "openai"):
		return "openai"
	case statusCode == 529 || containsAny(lower, "overloaded", "overloaded_error"):
		return "anthropic"
	default:
		return ""
	}
}

func isOverloaded(lower string, statusCode int) bool {
	return statusCode == 529 || containsAny(lower, overloadedPatterns...)
}

func isRateLimit(lower string, statusCode int) bool {
	if containsAny(lower, "quota", "billing", "insufficient_quota") {
		return false
	}
	return statusCode == 429 || containsAny(lower, rateLimitPatterns...)
}

func isTimeout(lower string, statusCode int) bool {
	switch statusCode {
	case 408, 502, 503, 504:
		return true
	default:
		return containsAny(lower, timeoutPatterns...)
	}
}

func isAuth(lower string, statusCode int) bool {
	switch statusCode {
	case 401, 403:
		return true
	default:
		return containsAny(lower, authPatterns...)
	}
}

func isConfig(lower string) bool {
	return containsAny(lower, configPatterns...)
}

func isGenericRetryable(lower string, statusCode int) bool {
	return statusCode == 500 || containsAny(lower, genericRetryablePatterns...)
}

func containsAny(lower string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func atoi(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}
