package chaterror

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/coder/coder/v2/aibridge"
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
		// "client conn" covers all of the stdlib http2 ClientConn errors:
		// "client conn is closed", "client conn not usable",
		// "client conn could not be established",
		// "client connection force closed via ClientConn.Close",
		// and "client connection lost".
		"client conn",
		// Transport-layer failures (HTTP/2 force-closed streams,
		// GOAWAY, closed network connections) so we retry.
		"goaway",
		"http2: stream closed",
		"use of closed network connection",
		// Stringified HTTP/2 RST_STREAM errors. Classify uses
		// typed http2.StreamError values when they survive wrapping;
		// these patterns cover bridge layers that flatten errors.
		"internal_error; received from peer",
		"refused_stream; received from peer",
		"cancel; received from peer",
		"enhance_your_calm; received from peer",
		"no_error; received from peer",
	}
	authStrongPatterns = []string{
		"authentication",
		"unauthorized",
		"invalid api key",
		"invalid_api_key",
	}
	authWeakPatterns   = []string{"forbidden"}
	usageLimitPatterns = []string{
		"quota",
		"billing",
		"payment required",
	}
	// Hard usage exhaustion codes that fire at any HTTP status,
	// including 429.
	usageLimitAnyStatusPatterns = []string{"insufficient_quota"}
	configPatterns              = []string{
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
	providerDisabledPatterns = []string{aibridge.ErrorCodeProviderDisabled}
)

func extractStatusCode(lower string) int {
	if matches := statusCodePattern.FindStringSubmatch(lower); len(matches) == 2 {
		if code, err := strconv.Atoi(matches[1]); err == nil {
			return code
		}
		return 0
	}
	for _, loc := range standaloneStatusPattern.FindAllStringIndex(lower, -1) {
		if shouldSkipStandaloneStatusMatch(lower, loc[0]) {
			continue
		}
		if code, err := strconv.Atoi(lower[loc[0]:loc[1]]); err == nil {
			return code
		}
		return 0
	}
	return 0
}

func shouldSkipStandaloneStatusMatch(lower string, start int) bool {
	// Skip values in host:port text. A later standalone status code in the
	// same message may still be valid, so keep scanning.
	if start > 0 && lower[start-1] == ':' {
		return true
	}

	// Go's HTTP/2 stream reset errors include "stream ID N". Those IDs are
	// not HTTP status codes, even when they happen to equal 401, 429, or 503.
	prefix := strings.TrimRight(lower[:start], " \t\r\n")
	prefix = strings.TrimRight(prefix, ":=")
	prefix = strings.TrimRight(prefix, " \t\r\n")
	return strings.HasSuffix(prefix, "stream id")
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
