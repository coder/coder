package chaterror

import (
	"regexp"
	"strconv"
	"strings"
)

type providerHint struct {
	provider string
	patterns []string
}

var (
	statusCodePattern       = regexp.MustCompile(`(?:status(?:\s+code)?|http)\s*[:=]?\s*(\d{3})`)
	standaloneStatusPattern = regexp.MustCompile(`\b(?:401|403|408|429|500|502|503|504|529)\b`)
	providerHints           = []providerHint{
		{provider: "openai-compat", patterns: []string{"openai-compat", "openai compatible"}},
		{provider: "azure", patterns: []string{"azure openai", "azure-openai"}},
		{provider: "openrouter", patterns: []string{"openrouter"}},
		{provider: "bedrock", patterns: []string{"aws bedrock", "bedrock"}},
		{provider: "vercel", patterns: []string{"vercel ai gateway", "vercel"}},
		{provider: "anthropic", patterns: []string{"anthropic", "claude"}},
		{provider: "google", patterns: []string{"google", "gemini", "vertex"}},
		{provider: "openai", patterns: []string{"openai"}},
	}
	overloadedPatterns = []string{"overloaded"}
	rateLimitPatterns  = []string{"rate limit", "rate_limit", "rate limited", "rate-limited", "too many requests"}
	timeoutPatterns    = []string{
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
	authStrongPatterns = []string{
		"authentication",
		"unauthorized",
		"invalid api key",
		"invalid_api_key",
		"quota",
		"billing",
		"insufficient_quota",
		"payment required",
	}
	authWeakPatterns = []string{"forbidden"}
	configPatterns   = []string{
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
	genericRetryablePatterns = []string{"server error", "internal server error"}
	interruptedPatterns      = []string{"chat interrupted", "request interrupted", "operation interrupted"}
)

func extractStatusCode(lower string) int {
	if matches := statusCodePattern.FindStringSubmatch(lower); len(matches) == 2 {
		if code, err := strconv.Atoi(matches[1]); err == nil {
			return code
		}
		return 0
	}
	for _, loc := range standaloneStatusPattern.FindAllStringIndex(lower, -1) {
		// Skip values in host:port text. A later standalone status code in the
		// same message may still be valid, so keep scanning.
		if loc[0] > 0 && lower[loc[0]-1] == ':' {
			continue
		}
		if code, err := strconv.Atoi(lower[loc[0]:loc[1]]); err == nil {
			return code
		}
		return 0
	}
	return 0
}

func detectProvider(lower string) string {
	for _, hint := range providerHints {
		if containsAny(lower, hint.patterns...) {
			return hint.provider
		}
	}
	return ""
}

func containsAny(lower string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
