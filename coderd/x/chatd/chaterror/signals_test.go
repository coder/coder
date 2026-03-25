package chaterror_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
)

func TestExtractStatusCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  int
	}{
		{name: "Status", input: "received status 429 from upstream", want: 429},
		{name: "StatusCode", input: "status code: 503", want: 503},
		{name: "HTTP", input: "http 502 bad gateway", want: 502},
		{name: "Standalone", input: "got 504 from upstream", want: 504},
		{name: "MultipleStandaloneCodesReturnFirstMatch", input: "retrying 503 after 429", want: 503},
		{name: "MixedCaseViaCallerLowering", input: "HTTP 503 bad gateway", want: 503},
		{name: "PortNumberIPIsNotStatus", input: "dial tcp 10.0.0.1:503: connection refused", want: 0},
		{name: "PortNumberHostIsNotStatus", input: "proxy.internal:502 unreachable", want: 0},
		{name: "PortNumberDialIsNotStatus", input: "dial tcp 172.16.0.5:429: refused", want: 0},
		{name: "PortThenRealStatusReturnsRealStatus", input: "proxy at 10.0.0.1:500 returned 503", want: 503},
		{name: "NoFabricatedOverloadStatus", input: "anthropic overloaded_error", want: 0},
		{name: "NoFabricatedRateLimitStatus", input: "too many requests", want: 0},
		{name: "NoFabricatedBadGatewayStatus", input: "bad gateway", want: 0},
		{name: "NoFabricatedServiceUnavailableStatus", input: "service unavailable", want: 0},
		{name: "NoStatus", input: "boom", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chaterror.ExtractStatusCodeForTest(strings.ToLower(tt.input)))
		})
	}
}

func TestDetectProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "OpenAICompatBeatsOpenAI", input: "openai-compat upstream error", want: "openai-compat"},
		{name: "OpenAICompatibleAlias", input: "openai compatible proxy", want: "openai-compat"},
		{name: "AzureOpenAI", input: "azure openai rate limited", want: "azure"},
		{name: "OpenAI", input: "openai rate limited", want: "openai"},
		{name: "Anthropic", input: "anthropic overloaded", want: "anthropic"},
		{name: "GoogleGemini", input: "gemini timeout", want: "google"},
		{name: "Vercel", input: "vercel ai gateway 503", want: "vercel"},
		{name: "Unknown", input: "local provider error", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chaterror.DetectProviderForTest(strings.ToLower(tt.input)))
		})
	}
}
