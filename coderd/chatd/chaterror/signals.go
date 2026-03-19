package chaterror

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
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

type rawSignals struct {
	statusCode       int
	provider         string
	overloaded       bool
	rateLimit        bool
	deadline         bool
	timeout          bool
	auth             bool
	config           bool
	genericRetryable bool
	canceled         bool
	interrupted      bool
}

func collectSignals(err error, message string) rawSignals {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return rawSignals{}
	}

	statusCode := extractStatusCode(lower)

	return rawSignals{
		statusCode:       statusCode,
		provider:         extractProvider(lower),
		overloaded:       isOverloaded(lower, statusCode),
		rateLimit:        isRateLimit(lower, statusCode),
		deadline:         errors.Is(err, context.DeadlineExceeded) || strings.Contains(lower, "context deadline exceeded"),
		timeout:          isTimeout(lower, statusCode),
		auth:             isAuth(lower, statusCode),
		config:           isConfig(lower),
		genericRetryable: isGenericRetryable(lower, statusCode),
		canceled:         errors.Is(err, context.Canceled) || strings.Contains(lower, "context canceled"),
		interrupted:      isInterrupted(lower),
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

func extractProvider(lower string) string {
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

func isInterrupted(lower string) bool {
	return containsAny(lower,
		"chat interrupted",
		"request interrupted",
		"operation interrupted",
	)
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
